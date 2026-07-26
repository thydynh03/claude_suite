package claims

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/gorilla/websocket"
)

// Client is how an agent on any machine takes part. It exists so that an IDE
// agent — which can run a shell command but cannot speak websockets — has
// something to call.
type Client struct {
	HostURL   string // ws://host:port or wss://host:port
	SessionID string
	Token     string
	Author    string
	Provider  string

	// OutDir receives verdict.json and transcript.md. The agent reads those
	// files; that is the whole interface back into an IDE.
	OutDir string

	ws *websocket.Conn
	// pending holds the chat history from the watch acknowledgement, so
	// Listen can print what was said before it connected.
	pending []ChatMessage
}

// Connect joins the session.
func (c *Client) Connect() error {
	return c.attach(MsgJoin)
}

// Watch attaches as an observer: token-authenticated, receiving state and
// chat, but never a participant. This is the mode for looking or speaking
// without taking a seat — joining is single-shot per author, so a --ping or a
// --say from an agent that already joined used to fail with "has already
// joined", and adjudication never waits for a watcher.
func (c *Client) Watch() error {
	return c.attach(MsgWatch)
}

func (c *Client) attach(mode string) error {
	if c.HostURL == "" || c.SessionID == "" || c.Token == "" {
		return fmt.Errorf("host, session and token are all required")
	}
	if c.Author == "" {
		return fmt.Errorf("author is required so the outcome can name who claimed what")
	}

	endpoint := fmt.Sprintf("%s/claims/%s?token=%s",
		c.HostURL, url.PathEscape(c.SessionID), url.QueryEscape(c.Token))

	ws, resp, err := websocket.DefaultDialer.Dial(endpoint, nil)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("connect: %s (%d)", err, resp.StatusCode)
		}
		return fmt.Errorf("connect: %w", err)
	}
	c.ws = ws

	if err := ws.WriteJSON(Message{Type: mode, Author: c.Author, Provider: c.Provider}); err != nil {
		return fmt.Errorf("%s: %w", mode, err)
	}
	state, err := c.readUntil(MsgState, 30*time.Second)
	if err != nil {
		return err
	}
	// Surface the roster warning immediately. A same-provider roster still runs,
	// but whoever reads the result should know the agreement in it is not
	// independent confirmation.
	if state.Note != "" {
		fmt.Fprintln(os.Stderr, "note: "+state.Note)
	}
	c.pending = state.Chat
	return nil
}

// Submit sends one claim.
func (c *Client) Submit(subject, assertion, falsifier string) error {
	claim := &Claim{Subject: subject, Assertion: assertion, Falsifier: falsifier}
	if err := c.ws.WriteJSON(Message{Type: MsgClaim, Claim: claim}); err != nil {
		return fmt.Errorf("submit: %w", err)
	}
	_, err := c.readUntil(MsgState, 30*time.Second)
	return err
}

// Say sends a chat message to everyone in the session. It is discussion, not
// evidence: only a falsifier settles a claim.
func (c *Client) Say(text string) error {
	return c.ws.WriteJSON(Message{Type: MsgChat, Text: text})
}

// Listen streams the session's chat to w — one JSON object per line, oldest
// first, each carrying the seq an agent needs to know what it has answered —
// until the session closes or the timeout passes. Attach with Watch first.
//
// JSONL on stdout is the whole interface: an IDE agent runs the command in
// the background and reads lines as the other side speaks.
func (c *Client) Listen(w io.Writer, timeout time.Duration) error {
	enc := json.NewEncoder(w)
	lastSeq := 0
	emit := func(msgs []ChatMessage) error {
		for _, m := range msgs {
			// Every broadcast carries the full transcript; seq is what keeps
			// this from printing the same message once per broadcast.
			if m.Seq > lastSeq {
				lastSeq = m.Seq
				if err := enc.Encode(m); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := emit(c.pending); err != nil {
		return err
	}
	c.pending = nil

	deadline := time.Now().Add(timeout)
	_ = c.ws.SetReadDeadline(deadline)
	for {
		var m Message
		if err := c.ws.ReadJSON(&m); err != nil {
			if time.Now().After(deadline) {
				return nil // listened for the agreed time; a quiet end, not a failure
			}
			return fmt.Errorf("listening: %w", err)
		}
		switch m.Type {
		case MsgState:
			if err := emit(m.Chat); err != nil {
				return err
			}
		case MsgOutcome:
			// A closing line, so a reader knows silence means "over", not "quiet".
			return enc.Encode(map[string]any{"session_closed": true, "session_id": c.SessionID})
		}
	}
}

// Done reports that this agent has nothing further, which lets the host close
// the collect window once everyone has said so.
func (c *Client) Done() error {
	return c.ws.WriteJSON(Message{Type: MsgDone})
}

// Await blocks until the session produces its outcome, then writes it where the
// IDE agent can read it.
func (c *Client) Await(timeout time.Duration) (*Outcome, error) {
	msg, err := c.readUntil(MsgOutcome, timeout)
	if err != nil {
		return nil, err
	}
	if msg.Outcome == nil {
		return nil, fmt.Errorf("host sent an outcome message with no outcome")
	}
	if c.OutDir != "" {
		if err := WriteOutcome(c.OutDir, msg.Outcome); err != nil {
			return msg.Outcome, err
		}
	}
	return msg.Outcome, nil
}

// Close hangs up.
func (c *Client) Close() {
	if c.ws != nil {
		_ = c.ws.Close()
	}
}

func (c *Client) readUntil(want string, timeout time.Duration) (Message, error) {
	deadline := time.Now().Add(timeout)
	_ = c.ws.SetReadDeadline(deadline)
	for {
		var m Message
		if err := c.ws.ReadJSON(&m); err != nil {
			return Message{}, fmt.Errorf("waiting for %s: %w", want, err)
		}
		if m.Type == MsgError {
			return Message{}, fmt.Errorf("host: %s", m.Error)
		}
		if m.Type == want {
			return m, nil
		}
		if time.Now().After(deadline) {
			return Message{}, fmt.Errorf("timed out waiting for %s", want)
		}
	}
}

// WriteOutcome saves the result under dir/session-<id>/.
//
// Files, not a socket: an IDE agent cannot hold a connection, but every one of
// them can read a file it was told about.
func WriteOutcome(dir string, o *Outcome) error {
	sessionDir := filepath.Join(dir, "session-"+o.SessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", sessionDir, err)
	}

	data, err := MarshalOutcome(o)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "verdict.json"), data, 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(sessionDir, "transcript.md"), []byte(o.Summary()), 0o600)
}
