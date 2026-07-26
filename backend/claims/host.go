package claims

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Message is the wire format in both directions.
type Message struct {
	Type string `json:"type"`

	// join
	Author   string `json:"author,omitempty"`
	Provider string `json:"provider,omitempty"`

	// claim / remark
	Claim *Claim `json:"claim,omitempty"`
	Text  string `json:"text,omitempty"`

	// server → client
	Phase    Phase         `json:"phase,omitempty"`
	Claims   []*Claim      `json:"claims,omitempty"`
	Outcome  *Outcome      `json:"outcome,omitempty"`
	Warnings []string      `json:"warnings,omitempty"`
	Chat     []ChatMessage `json:"chat,omitempty"`
	Error    string        `json:"error,omitempty"`
	Note     string        `json:"note,omitempty"`
}

const (
	MsgJoin   = "join"
	MsgClaim  = "claim"
	MsgRemark = "remark"
	// MsgChat is free-form talk between the agents in a session, not tied to a
	// claim. Remarks require a claim id and are only accepted during debate; a
	// teammate's agent that simply wants to say something had nowhere to put it.
	MsgChat    = "chat"
	MsgDone    = "done" // "I have nothing further to submit"
	MsgState   = "state"
	MsgOutcome = "outcome"
	MsgError   = "error"
)

// Timeouts. The collect window matters: a session cannot wait forever for an
// agent whose IDE was closed, but closing it early loses that agent's claims.
const (
	CollectWindow  = 5 * time.Minute
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingInterval   = (pongWait * 9) / 10
	maxMessageSize = 1 << 20
)

type connection struct {
	author string
	ws     *websocket.Conn
	mu     sync.Mutex // serialises writes; gorilla forbids concurrent writers
}

func (c *connection) send(m Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.ws.SetWriteDeadline(time.Now().Add(writeWait))
	return c.ws.WriteJSON(m)
}

type hosted struct {
	session *Session
	token   string
	expires time.Time

	mu        sync.Mutex
	conns     map[string]*connection
	submitted map[string]bool // authors who said they are finished
	// remote holds authors who joined over the MCP endpoint, by last-seen
	// time. They have no connection to drop and no way to be pushed to — they
	// poll. Adjudication must wait for their finish_reporting exactly as it
	// waits for a socket's done message, or a session would close while an
	// MCP agent is mid-review. The timestamp is the counterpart of a socket
	// disconnect: an MCP agent silent for a whole CollectWindow is treated as
	// departed, so a closed laptop cannot hold the session open forever.
	remote map[string]time.Time
	// remoteKeys maps each MCP author to a per-participant secret issued at
	// join. The session token is shared by the whole roster, so on its own it
	// authenticates the session, not the caller — without this key one
	// participant could submit claims or finish the collect phase as another.
	remoteKeys map[string]string
	// outcome is kept after Finish for the pollers: the websocket agents get
	// it broadcast, but an MCP agent that asks a second later would otherwise
	// find nothing.
	outcome *Outcome
	closed  bool
}

// Host accepts agents from other machines into adjudication sessions.
//
// It is a state machine with a socket attached, not a chat room. Agents cannot
// speak freely: every message is checked against the phase, which is how the
// blind collect window and the debate cap are enforced rather than merely asked
// for.
type Host struct {
	mu       sync.Mutex
	sessions map[string]*hosted

	Runner   *Runner
	Upgrader websocket.Upgrader

	// OnEvent, if set, receives human-readable progress for the UI.
	OnEvent func(sessionID, message string)
}

// NewHost builds a host bound to a workspace's check catalogue.
func NewHost(runner *Runner) *Host {
	return &Host{
		sessions: map[string]*hosted{},
		Runner:   runner,
		Upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			// Only reachable over a private network or loopback by design; see
			// the deployment notes. Do not widen this without adding an origin
			// allowlist.
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}
}

func randomToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Open starts a session and returns its id and join token.
//
// The token is per-session and expires. A long-lived shared secret would turn
// one leak into permanent access to every future session.
func (h *Host) Open(subject string, ttl time.Duration) (sessionID, token string, err error) {
	id, err := randomToken()
	if err != nil {
		return "", "", err
	}
	id = id[:12]
	token, err = randomToken()
	if err != nil {
		return "", "", err
	}
	if ttl <= 0 {
		ttl = time.Hour
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.sessions[id] = &hosted{
		session:    NewSession(id, subject),
		token:      token,
		expires:    time.Now().Add(ttl),
		conns:      map[string]*connection{},
		submitted:  map[string]bool{},
		remote:     map[string]time.Time{},
		remoteKeys: map[string]string{},
	}
	return id, token, nil
}

// Session exposes a hosted session for inspection.
func (h *Host) Session(id string) (*Session, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.sessions[id]
	if !ok {
		return nil, false
	}
	return s.session, true
}

func (h *Host) lookup(id, token string) (*hosted, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	s, ok := h.sessions[id]
	if !ok {
		return nil, fmt.Errorf("no session %s", id)
	}
	if time.Now().After(s.expires) {
		return nil, fmt.Errorf("session %s has expired", id)
	}
	// Constant time: a token check that returns early leaks the token one byte
	// at a time to anyone willing to measure.
	if subtle.ConstantTimeCompare([]byte(s.token), []byte(token)) != 1 {
		return nil, fmt.Errorf("invalid join token")
	}
	return s, nil
}

func (h *Host) emit(sessionID, msg string) {
	if h.OnEvent != nil {
		h.OnEvent(sessionID, msg)
	}
}

// Handler serves the websocket endpoint: /claims/{sessionID}?token=...
func (h *Host) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/claims/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/claims/")
		token := r.URL.Query().Get("token")

		hs, err := h.lookup(id, token)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		ws, err := h.Upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		h.serve(hs, ws)
	})
	// The zero-download door: any MCP-capable agent joins with `claude mcp add`
	// and no binary at all. See mcp.go.
	mux.HandleFunc("/mcp/", h.serveMCP)
	return mux
}

func (h *Host) serve(hs *hosted, ws *websocket.Conn) {
	defer ws.Close()

	ws.SetReadLimit(maxMessageSize)
	_ = ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(pongWait))
	})

	conn := &connection{ws: ws}
	stop := make(chan struct{})
	defer close(stop)
	go h.keepAlive(conn, stop)

	for {
		var msg Message
		if err := ws.ReadJSON(&msg); err != nil {
			h.dropConnection(hs, conn)
			return
		}
		if err := h.handle(hs, conn, msg); err != nil {
			_ = conn.send(Message{Type: MsgError, Error: err.Error()})
		}
	}
}

func (h *Host) keepAlive(c *connection, stop <-chan struct{}) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			c.mu.Lock()
			_ = c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			err := c.ws.WriteMessage(websocket.PingMessage, nil)
			c.mu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

// dropConnection removes a disconnected agent. Its already-submitted claims stay:
// an IDE closing is not a retraction.
func (h *Host) dropConnection(hs *hosted, c *connection) {
	hs.mu.Lock()
	if c.author != "" && hs.conns[c.author] == c {
		delete(hs.conns, c.author)
	}
	hs.mu.Unlock()
	if c.author != "" {
		h.emit(hs.session.ID, c.author+" disconnected")
		h.maybeAdjudicate(hs)
	}
}

func (h *Host) handle(hs *hosted, c *connection, msg Message) error {
	switch msg.Type {
	case MsgJoin:
		return h.handleJoin(hs, c, msg)
	case MsgClaim:
		return h.handleClaim(hs, c, msg)
	case MsgDone:
		return h.handleDone(hs, c)
	case MsgRemark:
		if c.author == "" {
			return fmt.Errorf("join before speaking")
		}
		// A remark with no claim attached used to dereference a nil pointer and
		// take down the host, which every connected agent then lost.
		if msg.Claim == nil || msg.Claim.ID == "" {
			return fmt.Errorf("remark must name the claim it is about")
		}
		if err := hs.session.Remark(c.author, msg.Claim.ID, msg.Text); err != nil {
			return err
		}
		h.broadcastState(hs)
		return nil
	case MsgChat:
		if c.author == "" {
			return fmt.Errorf("join before speaking")
		}
		if strings.TrimSpace(msg.Text) == "" {
			return fmt.Errorf("empty message")
		}
		hs.session.Say(c.author, msg.Text)
		h.broadcastState(hs)
		return nil
	default:
		return fmt.Errorf("unknown message type %q", msg.Type)
	}
}

func (h *Host) handleJoin(hs *hosted, c *connection, msg Message) error {
	if err := hs.session.Join(msg.Author, msg.Provider); err != nil {
		return err
	}
	c.author = msg.Author

	hs.mu.Lock()
	hs.conns[msg.Author] = c
	hs.mu.Unlock()

	h.emit(hs.session.ID, fmt.Sprintf("%s joined (%s)", msg.Author, msg.Provider))

	warnings := hs.session.Warnings()
	note := ""
	if len(warnings) > 0 {
		// Same-provider rosters are allowed. The agent is told, because whoever
		// reads the outcome should know the agreement in it is not independent.
		note = "roster warning: " + strings.Join(warnings, "; ")
	}
	return c.send(Message{
		Type: MsgState, Phase: hs.session.Phase(),
		Claims: hs.session.VisibleTo(msg.Author), Warnings: warnings, Note: note,
	})
}

func (h *Host) handleClaim(hs *hosted, c *connection, msg Message) error {
	if c.author == "" {
		return fmt.Errorf("join before submitting")
	}
	if msg.Claim == nil {
		return fmt.Errorf("empty claim")
	}
	msg.Claim.Author = c.author
	if msg.Claim.ID == "" {
		id, err := randomToken()
		if err != nil {
			return err
		}
		msg.Claim.ID = id[:10]
	}
	if err := hs.session.Submit(msg.Claim); err != nil {
		return err
	}
	h.emit(hs.session.ID, fmt.Sprintf("%s submitted a %s claim", c.author, msg.Claim.Kind))

	// Only its own claims come back. The collect phase is blind, and that is
	// enforced here rather than trusted to the client.
	return c.send(Message{
		Type: MsgState, Phase: hs.session.Phase(), Claims: hs.session.VisibleTo(c.author),
	})
}

func (h *Host) handleDone(hs *hosted, c *connection) error {
	if c.author == "" {
		return fmt.Errorf("join before finishing")
	}
	hs.mu.Lock()
	hs.submitted[c.author] = true
	hs.mu.Unlock()
	h.emit(hs.session.ID, c.author+" finished submitting")
	h.maybeAdjudicate(hs)
	return nil
}

// maybeAdjudicate moves the session on once every participant — connected or
// polling over MCP — has finished.
//
// An MCP participant silent for longer than CollectWindow no longer counts:
// that is its version of a socket disconnect, and without it a laptop closed
// mid-review would hold every other agent's verdict hostage until someone
// found the Force button.
func (h *Host) maybeAdjudicate(hs *hosted) {
	hs.mu.Lock()
	live := 0
	for range hs.conns {
		live++
	}
	unfinished := false
	for author := range hs.conns {
		if !hs.submitted[author] {
			unfinished = true
		}
	}
	for author, seen := range hs.remote {
		if time.Since(seen) > CollectWindow && !hs.submitted[author] {
			continue // departed without finishing; do not wait for them
		}
		live++
		if !hs.submitted[author] {
			unfinished = true
		}
	}
	if hs.closed || hs.session.Phase() != PhaseCollect || live == 0 || unfinished {
		hs.mu.Unlock()
		return
	}
	hs.closed = true
	hs.mu.Unlock()

	go h.adjudicate(hs)
}

// ForceAdjudicate ends the collect window early, for a caller that does not want
// to wait on an agent that never reports finished.
func (h *Host) ForceAdjudicate(sessionID string) error {
	h.mu.Lock()
	hs, ok := h.sessions[sessionID]
	h.mu.Unlock()
	if !ok {
		return fmt.Errorf("no session %s", sessionID)
	}
	hs.mu.Lock()
	if hs.closed {
		hs.mu.Unlock()
		return fmt.Errorf("session %s has already left collect", sessionID)
	}
	hs.closed = true
	hs.mu.Unlock()

	h.adjudicate(hs)
	return nil
}

func (h *Host) adjudicate(hs *hosted) {
	if err := hs.session.Advance(PhaseAdjudicate); err != nil {
		h.broadcast(hs, Message{Type: MsgError, Error: err.Error()})
		return
	}
	h.emit(hs.session.ID, "running falsifiers")
	h.broadcastState(hs)

	if h.Runner != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		if err := h.Runner.Adjudicate(ctx, hs.session); err != nil {
			h.broadcast(hs, Message{Type: MsgError, Error: err.Error()})
		}
		cancel()
	}

	// Only now do the agents see each other, and they see the evidence with it.
	if err := hs.session.Advance(PhaseReveal); err != nil {
		h.broadcast(hs, Message{Type: MsgError, Error: err.Error()})
		return
	}
	h.emit(hs.session.ID, "evidence in; revealing")
	h.broadcastState(hs)
}

// OpenDebateRound moves a revealed session into discussion.
//
// Only opinions can be discussed. Everything the checks settled stays settled,
// and the host refuses a round past the cap rather than trusting the agents to
// stop — past a couple of exchanges, further rounds mostly measure who repeats
// themselves most confidently.
func (h *Host) OpenDebateRound(sessionID string) error {
	h.mu.Lock()
	hs, ok := h.sessions[sessionID]
	h.mu.Unlock()
	if !ok {
		return fmt.Errorf("no session %s", sessionID)
	}
	if err := hs.session.Advance(PhaseDebate); err != nil {
		return err
	}
	h.emit(sessionID, fmt.Sprintf("debate round %d open", hs.session.DebateRound()))
	h.broadcastState(hs)
	return nil
}

// Finish closes a session and sends the outcome to everyone still connected.
func (h *Host) Finish(sessionID string) (*Outcome, error) {
	h.mu.Lock()
	hs, ok := h.sessions[sessionID]
	h.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no session %s", sessionID)
	}

	out := hs.session.Close()
	hs.mu.Lock()
	hs.outcome = out
	hs.mu.Unlock()
	h.broadcast(hs, Message{Type: MsgOutcome, Outcome: out, Phase: PhaseRecord})
	h.emit(sessionID, "session closed")
	return out, nil
}

func (h *Host) broadcastState(hs *hosted) {
	hs.mu.Lock()
	conns := make([]*connection, 0, len(hs.conns))
	for _, c := range hs.conns {
		conns = append(conns, c)
	}
	hs.mu.Unlock()

	for _, c := range conns {
		_ = c.send(Message{
			Type: MsgState, Phase: hs.session.Phase(), Claims: hs.session.VisibleTo(c.author),
			// Chat goes to everyone, unlike claims, which stay hidden until the
			// collect window closes so agents cannot anchor on each other's
			// findings. Talk is meant to be seen — that is the point of it.
			Chat: hs.session.Chat(),
		})
	}
}

func (h *Host) broadcast(hs *hosted, m Message) {
	hs.mu.Lock()
	conns := make([]*connection, 0, len(hs.conns))
	for _, c := range hs.conns {
		conns = append(conns, c)
	}
	hs.mu.Unlock()

	for _, c := range conns {
		_ = c.send(m)
	}
}

// MarshalOutcome writes an outcome for a participant's machine.
func MarshalOutcome(o *Outcome) ([]byte, error) {
	return json.MarshalIndent(o, "", "  ")
}
