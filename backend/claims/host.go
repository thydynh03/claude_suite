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
	MsgChat = "chat"
	// MsgWatch attaches as an observer: authenticated by the session token,
	// receiving broadcasts and allowed to chat, but never a participant.
	// Joining is permanent and single-shot per author — which made every
	// second connection from the same agent (--ping before a claim run, --say
	// after one) fail with "has already joined". Watching is what those
	// actually needed: presence without a seat at the table.
	MsgWatch   = "watch"
	MsgDone    = "done" // "I have nothing further to submit"
	MsgState   = "state"
	MsgOutcome = "outcome"
	MsgError   = "error"
)

// Timeouts. The collect window matters: a session cannot wait forever for an
// agent whose IDE was closed, but closing it early loses that agent's claims.
const (
	CollectWindow = 5 * time.Minute
	// DefaultSoloHold is how long a one-agent session lingers in collect after
	// that agent finishes. The first agent typically joins, submits and
	// finishes within a second; without the hold it slams the window shut
	// before the second side of the dispute has even pasted its join command.
	DefaultSoloHold = 90 * time.Second
	writeWait       = 10 * time.Second
	pongWait        = 60 * time.Second
	pingInterval    = (pongWait * 9) / 10
	maxMessageSize  = 1 << 20
)

type connection struct {
	author string
	// watcher marks an observer connection: it may chat but not claim or
	// finish, and adjudication never waits for it.
	watcher bool
	ws      *websocket.Conn
	mu      sync.Mutex // serialises writes; gorilla forbids concurrent writers
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
	// hardExpires is the ceiling the sliding expiry may not pass, so a polling
	// agent cannot keep a session alive indefinitely.
	hardExpires time.Time

	mu        sync.Mutex
	conns     map[string]*connection
	submitted map[string]bool // authors who said they are finished
	// watchers receive every broadcast but hold no seat: they are not in
	// conns (which is what adjudication waits on) and several may share an
	// author name, so a slice rather than a map.
	watchers []*connection
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
	// holdUntil, once set, keeps a one-participant session in collect until
	// the deadline passes, so the other side of the dispute can still join.
	holdUntil time.Time
	closed    bool
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

	// SoloHold keeps collect open this long after a lone participant finishes,
	// so the other side of the dispute can still join. Zero disables the hold.
	SoloHold time.Duration
}

// NewHost builds a host bound to a workspace's check catalogue.
func NewHost(runner *Runner) *Host {
	return &Host{
		sessions: map[string]*hosted{},
		Runner:   runner,
		SoloHold: DefaultSoloHold,
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
		session: NewSession(id, subject),
		token:   token,
		expires: time.Now().Add(ttl),
		// A day is far past any real adjudication (collect + up to 30 minutes
		// of falsifier runs + debate), and short enough that a forgotten
		// session and its token do not outlive the working day.
		hardExpires: time.Now().Add(24 * time.Hour),
		conns:       map[string]*connection{},
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
	// Sliding expiry for a session somebody is actually using. Websocket
	// agents pass through lookup once, at connect, and then keep working past
	// the TTL; an MCP participant re-runs it on every poll, so the fixed
	// 60-minute window kicked exactly the remote teammate who was following
	// the instructions to keep polling — mid-adjudication, with a 401, and
	// even the recorded verdict became unreachable. The TTL still ends
	// abandoned sessions: nothing renews a session nobody is calling.
	//
	// s.expires is only ever touched under h.mu (here and in Open), never
	// under the per-session lock, so this stays race-free.
	//
	// The slide is capped: the MCP instructions tell agents to keep polling, so
	// an agent left running would otherwise hold its session — and its join
	// token, which is short-lived on purpose — valid forever, with the whole
	// transcript pinned in memory.
	if min := time.Now().Add(15 * time.Minute); s.expires.Before(min) {
		if min.After(s.hardExpires) {
			min = s.hardExpires
		}
		if min.After(s.expires) {
			s.expires = min
		}
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
	if c.watcher {
		hs.mu.Lock()
		for i, w := range hs.watchers {
			if w == c {
				hs.watchers = append(hs.watchers[:i], hs.watchers[i+1:]...)
				break
			}
		}
		hs.mu.Unlock()
		// No emit and no adjudication nudge: an observer leaving changes
		// nothing anyone is waiting on.
		return
	}
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
	case MsgWatch:
		return h.handleWatch(hs, c, msg)
	case MsgClaim:
		return h.handleClaim(hs, c, msg)
	case MsgDone:
		if c.watcher {
			return fmt.Errorf("a watcher has nothing to finish — join to take part")
		}
		return h.handleDone(hs, c)
	case MsgRemark:
		if c.author == "" {
			return fmt.Errorf("join before speaking")
		}
		// Remarks land in the durable transcript under the connection's author.
		// A watcher names its own author freely, so letting one remark would
		// let anyone holding the session URL write debate turns as any
		// participant. Chat does not have this guard on purpose: talk is not
		// evidence, and the arbiter UI already posts chat under a chosen name.
		if c.watcher {
			return fmt.Errorf("watchers may chat but not remark — join to take part in debate")
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
		// History up front, so a joiner does not wait for the next broadcast
		// to know what has already been said.
		Chat: hs.session.Chat(),
	})
}

// handleWatch attaches an observer: it receives every broadcast and may chat,
// but holds no seat — adjudication never waits for it, and it cannot claim,
// finish, or remark. This is what --ping, --say and --listen ride on: joining
// is single-shot per author, so a second connection from the same agent used
// to die with "has already joined" when all it wanted was to look or speak.
func (h *Host) handleWatch(hs *hosted, c *connection, msg Message) error {
	if c.author != "" && !c.watcher {
		return fmt.Errorf("%s has already joined on this connection", c.author)
	}
	c.watcher = true
	c.author = strings.TrimSpace(msg.Author)
	if c.author == "" {
		c.author = "observer"
	}

	hs.mu.Lock()
	hs.watchers = append(hs.watchers, c)
	hs.mu.Unlock()

	// VisibleTo with an empty author: nothing during the blind collect phase,
	// everything after reveal. A watcher naming a participant's author must
	// not read that participant's blind claims — the MCP door needs a
	// participant_key for exactly this, and the websocket door has none.
	return c.send(Message{
		Type: MsgState, Phase: hs.session.Phase(),
		Claims: hs.session.VisibleTo(""), Chat: hs.session.Chat(),
	})
}

func (h *Host) handleClaim(hs *hosted, c *connection, msg Message) error {
	if c.author == "" {
		return fmt.Errorf("join before submitting")
	}
	// A watcher's author is self-declared; without this it could submit a
	// claim under the name of a participant who really joined.
	if c.watcher {
		return fmt.Errorf("watchers may chat but not claim — join to take part")
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
// A session whose only participant has finished is held in collect for
// SoloHold first: sessions are opened to settle disputes, and the second
// agent is usually seconds behind the first, not absent.
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

	// A roster of one that finished is usually not a completed review — it is
	// a dispute whose other side is still pasting the join command into a
	// terminal. Adjudicating "between agents" must not require winning a race
	// against the first agent's startup time, so the window stays open a
	// little longer. ForceAdjudicate skips the wait.
	soloHeld := false
	if h.SoloHold > 0 && hs.session.ParticipantCount() < 2 {
		if hs.holdUntil.IsZero() {
			hs.holdUntil = time.Now().Add(h.SoloHold)
			hs.mu.Unlock()
			h.emit(hs.session.ID, fmt.Sprintf(
				"one agent finished; holding collect open %s for others to join (force-run to skip)",
				h.SoloHold))
			time.AfterFunc(h.SoloHold+250*time.Millisecond, func() { h.maybeAdjudicate(hs) })
			return
		}
		if time.Now().Before(hs.holdUntil) {
			hs.mu.Unlock()
			return
		}
		soloHeld = true
	}
	hs.closed = true
	hs.mu.Unlock()

	if soloHeld {
		h.emit(hs.session.ID, "nobody else joined; adjudicating with one agent")
	}
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

// Say posts a chat message on behalf of the host's own user — the arbiter in
// the app — and pushes it to every connected agent and watcher at once.
// Writing straight into the session was not enough: websocket listeners only
// hear what is broadcast, so the arbiter's messages sat unread until some
// other event happened to trigger one.
func (h *Host) Say(sessionID, author, text string) error {
	h.mu.Lock()
	hs, ok := h.sessions[sessionID]
	h.mu.Unlock()
	if !ok {
		return fmt.Errorf("no session %s", sessionID)
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("empty message")
	}
	hs.session.Say(author, text)
	h.broadcastState(hs)
	return nil
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

	// Close() sets PhaseRecord itself, so it walks straight past Advance's
	// "cannot record before adjudicating" guard. Finishing during collect
	// turned every verifiable claim into Inconclusive→Escalated without a
	// single falsifier having run — one click, no undo, no way to reopen.
	//
	// `closed` is consulted too: maybeAdjudicate sets it and then advances the
	// phase from a goroutine, so for a moment the session is on its way out
	// while still reporting PhaseCollect. Refusing on the phase alone left that
	// window with both doors shut — Finish said "still collecting" and
	// ForceAdjudicate said "already left collect".
	hs.mu.Lock()
	closing := hs.closed
	hs.mu.Unlock()
	if hs.session.Phase() == PhaseCollect && !closing {
		return nil, fmt.Errorf("phiên đang thu thập claim — bấm \"Chạy kiểm chứng ngay\" trước khi chốt, nếu không mọi claim sẽ bị bỏ ngỏ")
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
	watchers := append([]*connection(nil), hs.watchers...)
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
	for _, c := range watchers {
		// Empty author on purpose: a watcher's name is self-declared, so it
		// earns no view into anyone's blind claims.
		_ = c.send(Message{
			Type: MsgState, Phase: hs.session.Phase(), Claims: hs.session.VisibleTo(""),
			Chat: hs.session.Chat(),
		})
	}
}

func (h *Host) broadcast(hs *hosted, m Message) {
	hs.mu.Lock()
	conns := make([]*connection, 0, len(hs.conns)+len(hs.watchers))
	for _, c := range hs.conns {
		conns = append(conns, c)
	}
	conns = append(conns, hs.watchers...)
	hs.mu.Unlock()

	for _, c := range conns {
		_ = c.send(m)
	}
}

// MarshalOutcome writes an outcome for a participant's machine.
func MarshalOutcome(o *Outcome) ([]byte, error) {
	return json.MarshalIndent(o, "", "  ")
}
