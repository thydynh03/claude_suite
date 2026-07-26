package claims

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// MCP endpoint: /mcp/{sessionID}?token=...
//
// This is the zero-download way into a session. The claim CLI covers machines
// that can fetch a binary; this covers the ones that will not — the teammate
// registers the URL with their agent (`claude mcp add --transport http ...`)
// and the agent gets join/submit/finish/state as tools. Nothing is installed,
// nothing is downloaded, and the session token stays in the URL they were
// handed, scoped and expiring like every other join.
//
// The transport is Streamable HTTP in its stateless form: every request is a
// self-contained JSON-RPC POST, authenticated by the session token. There is
// no SSE stream and no server-side connection state — MCP agents poll
// get_session_state, which is why hosted.remote exists: adjudication has to
// wait for a participant it cannot push to.

const mcpProtocolVersion = "2025-03-26"

type mcpRequest struct {
	Jsonrpc string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// toolArgs is the union of every tool's arguments. One struct rather than one
// per tool: the dispatch stays a flat switch, and an argument sent to the
// wrong tool is simply ignored instead of failing to parse.
type toolArgs struct {
	Author    string `json:"author"`
	Provider  string `json:"provider"`
	Subject   string `json:"subject"`
	Assertion string `json:"assertion"`
	Falsifier string `json:"falsifier"`
	Text      string `json:"text"`
	ClaimID   string `json:"claim_id"`
	// ParticipantKey is issued by join_session and required on every later
	// call. The session token in the URL is shared by the whole roster, so it
	// proves membership, not identity — without this key any participant
	// could read, submit or finish as any other.
	ParticipantKey string `json:"participant_key"`
}

func (h *Host) serveMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		// Stateless server: no SSE stream to GET, no session to DELETE.
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "this MCP endpoint is stateless; POST JSON-RPC requests", http.StatusMethodNotAllowed)
		return
	}

	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/mcp/"), "/")
	hs, err := h.lookup(id, r.URL.Query().Get("token"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxMessageSize))
	if err != nil {
		http.Error(w, "unreadable body", http.StatusBadRequest)
		return
	}

	var req mcpRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeMCPError(w, nil, -32700, "parse error: "+err.Error())
		return
	}

	// A notification (no id) expects no response body.
	if len(req.ID) == 0 || string(req.ID) == "null" {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		ver := p.ProtocolVersion
		if ver == "" {
			ver = mcpProtocolVersion
		}
		writeMCPResult(w, req.ID, map[string]any{
			"protocolVersion": ver,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "claude-suite-claims", "version": "1.0.0"},
			"instructions": "You are joining a Claude Suite adjudication session: agents state defects " +
				"as falsifiable claims, the host runs the named checks, and the output — not the " +
				"discussion — decides. Flow: join_session (keep the participant_key it returns — every " +
				"later call needs it with your author) → list_checks → investigate the subject in " +
				"your own workspace → submit_claim (one per finding; omit falsifier for an opinion, " +
				"which cannot block) → finish_reporting → poll get_session_state for verdicts and the " +
				"outcome. A falsifier PASSES when the claim is WRONG, so a failing check confirms the defect.",
		})
	case "ping":
		writeMCPResult(w, req.ID, map[string]any{})
	case "tools/list":
		writeMCPResult(w, req.ID, map[string]any{"tools": mcpTools()})
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			writeMCPError(w, req.ID, -32602, "invalid params: "+err.Error())
			return
		}
		var args toolArgs
		if len(p.Arguments) > 0 {
			if err := json.Unmarshal(p.Arguments, &args); err != nil {
				writeMCPError(w, req.ID, -32602, "invalid arguments: "+err.Error())
				return
			}
		}
		text, err := h.callMCPTool(hs, p.Name, args)
		if err != nil {
			// Tool-level failures travel as a result with isError, so the
			// agent sees the message and can correct itself; -32602 is kept
			// for a tool that does not exist at all.
			if strings.HasPrefix(err.Error(), "unknown tool") {
				writeMCPError(w, req.ID, -32602, err.Error())
				return
			}
			writeMCPResult(w, req.ID, map[string]any{
				"content": []map[string]any{{"type": "text", "text": err.Error()}},
				"isError": true,
			})
			return
		}
		writeMCPResult(w, req.ID, map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
		})
	default:
		writeMCPError(w, req.ID, -32601, "method not found: "+req.Method)
	}
}

func (h *Host) callMCPTool(hs *hosted, name string, args toolArgs) (string, error) {
	switch name {
	case "join_session":
		if strings.TrimSpace(args.Author) == "" {
			return "", fmt.Errorf("author is required — name yourself like user@machine/agent")
		}
		key, err := randomToken()
		if err != nil {
			return "", err
		}
		if err := hs.session.Join(args.Author, args.Provider); err != nil {
			return "", err
		}
		hs.mu.Lock()
		hs.remote[args.Author] = time.Now()
		hs.remoteKeys[args.Author] = key
		hs.mu.Unlock()
		h.emit(hs.session.ID, fmt.Sprintf("%s joined over MCP (%s)", args.Author, args.Provider))

		// The disconnect analogue for a participant nothing can push to: once
		// the collect window has passed, re-check whether the session is only
		// waiting on agents that went silent.
		time.AfterFunc(CollectWindow+time.Second, func() { h.maybeAdjudicate(hs) })

		note := ""
		if warnings := hs.session.Warnings(); len(warnings) > 0 {
			note = "\nRoster warning: " + strings.Join(warnings, "; ")
		}
		return fmt.Sprintf("Joined session %s.\nSubject: %s\nPhase: %s\nYour participant_key: %s — "+
			"pass it, with your author, on every later call; it is what proves the call is yours.%s\n\n"+
			"Next: call list_checks, investigate the subject in your own checkout, submit_claim for "+
			"each finding, then finish_reporting. The collect phase is blind — you will not see other "+
			"agents' claims until the checks have run. Stay in touch: a participant silent for over "+
			"%s is treated as departed.",
			hs.session.ID, hs.session.Subject, hs.session.Phase(), key, note, CollectWindow), nil

	case "list_checks":
		if h.Runner == nil || h.Runner.Catalogue == nil || len(h.Runner.Catalogue.Checks) == 0 {
			return "No checks in this workspace's catalogue: every claim will be an opinion, " +
				"recorded and shown but unable to block a merge.", nil
		}
		var b strings.Builder
		b.WriteString("Checks a claim may name as its falsifier. A falsifier PASSES when the claim " +
			"is WRONG — a failing check confirms the defect.\n\n")
		for _, c := range h.Runner.Catalogue.Checks {
			fmt.Fprintf(&b, "  %-24s %s\n", c.Name, c.Description)
		}
		return b.String(), nil

	case "submit_claim":
		if err := h.requireMCPJoin(hs, args.Author, args.ParticipantKey); err != nil {
			return "", err
		}
		claim := &Claim{
			Author:    args.Author,
			Provider:  args.Provider,
			Subject:   args.Subject,
			Assertion: args.Assertion,
			Falsifier: args.Falsifier,
		}
		id, err := randomToken()
		if err != nil {
			return "", err
		}
		claim.ID = id[:10]
		if err := hs.session.Submit(claim); err != nil {
			return "", err
		}
		h.emit(hs.session.ID, fmt.Sprintf("%s submitted a %s claim", args.Author, claim.Kind))
		if claim.Kind == KindOpinion {
			return fmt.Sprintf("Claim %s recorded as an OPINION (no falsifier): it will be shown and "+
				"escalated to a person, but it cannot block. Name a check from list_checks to make it "+
				"verifiable.", claim.ID), nil
		}
		return fmt.Sprintf("Claim %s recorded (%s). Its falsifier %q will run during adjudication.",
			claim.ID, claim.Kind, claim.Falsifier), nil

	case "finish_reporting":
		if err := h.requireMCPJoin(hs, args.Author, args.ParticipantKey); err != nil {
			return "", err
		}
		hs.mu.Lock()
		hs.submitted[args.Author] = true
		hs.mu.Unlock()
		h.emit(hs.session.ID, args.Author+" finished submitting (MCP)")
		h.maybeAdjudicate(hs)
		return "Recorded. Once every participant finishes, the host runs the falsifiers and reveals " +
			"all claims with their evidence — poll get_session_state to follow it.", nil

	case "say":
		if err := h.requireMCPJoin(hs, args.Author, args.ParticipantKey); err != nil {
			return "", err
		}
		if strings.TrimSpace(args.Text) == "" {
			return "", fmt.Errorf("empty message")
		}
		hs.session.Say(args.Author, args.Text)
		return "Sent.", nil

	case "remark":
		if err := h.requireMCPJoin(hs, args.Author, args.ParticipantKey); err != nil {
			return "", err
		}
		if args.ClaimID == "" {
			return "", fmt.Errorf("remark must name the claim it is about (claim_id)")
		}
		if err := hs.session.Remark(args.Author, args.ClaimID, args.Text); err != nil {
			return "", err
		}
		return "Remark recorded.", nil

	case "get_session_state":
		// State is scoped to a verified identity. Without this, anyone
		// holding the shared URL could read another agent's blind claims
		// during collect by naming that agent — or read without joining at
		// all, silently.
		if err := h.requireMCPJoin(hs, args.Author, args.ParticipantKey); err != nil {
			return "", err
		}
		hs.mu.Lock()
		outcome := hs.outcome
		hs.mu.Unlock()

		state := map[string]any{
			"session_id":   hs.session.ID,
			"subject":      hs.session.Subject,
			"phase":        hs.session.Phase(),
			"warnings":     hs.session.Warnings(),
			"claims":       hs.session.VisibleTo(args.Author),
			"chat":         hs.session.Chat(),
			"debate_round": hs.session.DebateRound(),
		}
		if outcome != nil {
			state["outcome"] = outcome
		}
		data, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			return "", err
		}
		return mcpPhaseGuidance(hs.session.Phase(), outcome != nil) + "\n\n" + string(data), nil

	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}

// requireMCPJoin rejects tool calls whose author never joined or whose
// participant_key does not match the one issued at join. The websocket path
// gets identity for free from the connection's join handshake; here every
// request stands alone, so both membership and identity are checked
// explicitly. A valid call also refreshes the author's last-seen time — the
// poller's heartbeat.
func (h *Host) requireMCPJoin(hs *hosted, author, key string) error {
	if strings.TrimSpace(author) == "" {
		return fmt.Errorf("author is required")
	}
	hs.mu.Lock()
	defer hs.mu.Unlock()
	issued, joined := hs.remoteKeys[author]
	if !joined {
		return fmt.Errorf("%s has not joined over MCP — call join_session first", author)
	}
	// Constant time for the same reason the session token is: a comparison
	// that returns early leaks the key one byte at a time.
	if subtle.ConstantTimeCompare([]byte(issued), []byte(key)) != 1 {
		return fmt.Errorf("participant_key does not match the one issued to %s at join_session", author)
	}
	hs.remote[author] = time.Now()
	return nil
}

func mcpPhaseGuidance(p Phase, closed bool) string {
	if closed {
		return "The session is closed; the outcome below is final. Blocking lists confirmed defects, " +
			"escalated needs a person."
	}
	switch p {
	case PhaseCollect:
		return "Collect phase (blind): you see only your own claims. submit_claim for each finding, " +
			"then finish_reporting."
	case PhaseAdjudicate:
		return "Falsifiers are running. Poll get_session_state until the phase moves to reveal."
	case PhaseReveal:
		return "All claims are revealed with their evidence. Confirmed verifiable claims block; " +
			"opinions may move to a debate round."
	case PhaseDebate:
		return "Debate round open, opinions only — use remark with the claim_id you are answering. " +
			"Nothing the checks settled is reopened."
	default:
		return "Session is in phase " + string(p) + "."
	}
}

func mcpTools() []map[string]any {
	obj := func(props map[string]any, required ...string) map[string]any {
		s := map[string]any{"type": "object", "properties": props}
		if len(required) > 0 {
			s["required"] = required
		}
		return s
	}
	author := map[string]any{"type": "string", "description": "Who you are: user@machine/agent, stable across calls"}
	pkey := map[string]any{"type": "string", "description": "The participant_key issued to you by join_session"}

	return []map[string]any{
		{
			"name": "join_session",
			"description": "Join this adjudication session as a reviewer. Do this first; it returns your " +
				"participant_key, which every other tool requires together with the same author string.",
			"inputSchema": obj(map[string]any{
				"author":   author,
				"provider": map[string]any{"type": "string", "description": "Model provider, e.g. claude or gemini"},
			}, "author"),
		},
		{
			"name": "list_checks",
			"description": "List the checks a claim may name as its falsifier. A falsifier PASSES when " +
				"the claim is WRONG, so a failing check confirms the defect.",
			"inputSchema": obj(map[string]any{}),
		},
		{
			"name": "submit_claim",
			"description": "Submit one finding, stated so it can be proven wrong. Omit falsifier for an " +
				"opinion — recorded and shown, but unable to block a merge.",
			"inputSchema": obj(map[string]any{
				"author":          author,
				"participant_key": pkey,
				"subject":         map[string]any{"type": "string", "description": "What the claim is about: file:line, task id, or PR URL"},
				"assertion":       map[string]any{"type": "string", "description": "The defect, in one sentence"},
				"falsifier":       map[string]any{"type": "string", "description": "A check name from list_checks; empty for an opinion"},
				"provider":        map[string]any{"type": "string", "description": "Model provider, e.g. claude or gemini"},
			}, "author", "participant_key", "subject", "assertion"),
		},
		{
			"name": "finish_reporting",
			"description": "Declare you have nothing further to submit. When every participant has " +
				"finished, the host runs the falsifiers and reveals all claims.",
			"inputSchema": obj(map[string]any{"author": author, "participant_key": pkey}, "author", "participant_key"),
		},
		{
			"name": "get_session_state",
			"description": "The session as you are allowed to see it right now: phase, your visible " +
				"claims, chat, and the outcome once closed. Poll this after finish_reporting.",
			"inputSchema": obj(map[string]any{"author": author, "participant_key": pkey}, "author", "participant_key"),
		},
		{
			"name":        "say",
			"description": "Free-form discussion visible to everyone. Talk is not evidence and never changes a verdict.",
			"inputSchema": obj(map[string]any{
				"author":          author,
				"participant_key": pkey,
				"text":            map[string]any{"type": "string"},
			}, "author", "participant_key", "text"),
		},
		{
			"name":        "remark",
			"description": "One turn of debate about an opinion claim. Only accepted during the debate phase.",
			"inputSchema": obj(map[string]any{
				"author":          author,
				"participant_key": pkey,
				"claim_id":        map[string]any{"type": "string", "description": "The claim this remark is about"},
				"text":            map[string]any{"type": "string"},
			}, "author", "participant_key", "claim_id", "text"),
		},
	}
}

func writeMCPResult(w http.ResponseWriter, id json.RawMessage, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0", "id": id, "result": result,
	})
}

func writeMCPError(w http.ResponseWriter, id json.RawMessage, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	if id == nil {
		id = json.RawMessage("null")
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0", "id": id, "error": mcpError{Code: code, Message: msg},
	})
}
