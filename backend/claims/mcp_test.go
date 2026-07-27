package claims

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

// rpc posts one JSON-RPC request and decodes the response envelope.
func rpc(t *testing.T, url, method string, params any) map[string]any {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": method, "params": params,
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decoding %s response: %v", method, err)
	}
	return out
}

// callTool runs tools/call and returns the text content plus the isError flag.
func callTool(t *testing.T, url, name string, args map[string]any) (string, bool) {
	t.Helper()
	out := rpc(t, url, "tools/call", map[string]any{"name": name, "arguments": args})
	result, ok := out["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/call %s: no result in %v", name, out)
	}
	isErr, _ := result["isError"].(bool)
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("tools/call %s: empty content", name)
	}
	first, _ := content[0].(map[string]any)
	text, _ := first["text"].(string)
	return text, isErr
}

func openMCPSession(t *testing.T) (h *Host, url string, sessionID string) {
	t.Helper()
	h = NewHost(&Runner{Catalogue: &Catalogue{Checks: []Check{
		{Name: "go-test", Description: "runs go test"},
	}}})
	id, token, err := h.Open("backend/cli/process_windows.go:17", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h.Handler())
	t.Cleanup(srv.Close)
	return h, srv.URL + "/mcp/" + id + "?token=" + token, id
}

// joinMCP joins and returns the participant_key the session issued.
func joinMCP(t *testing.T, url, author string) string {
	t.Helper()
	text, isErr := callTool(t, url, "join_session", map[string]any{"author": author, "provider": "claude"})
	if isErr {
		t.Fatalf("join_session %s failed: %s", author, text)
	}
	m := regexp.MustCompile(`participant_key: ([0-9a-f]+)`).FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("join_session did not issue a participant_key:\n%s", text)
	}
	return m[1]
}

func TestMCPRejectsABadToken(t *testing.T) {
	h := NewHost(nil)
	id, _, err := h.Open("subject", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/mcp/"+id+"?token=wrong", "application/json",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad token got HTTP %d, want 401", resp.StatusCode)
	}
}

func TestMCPInitializeAndToolListing(t *testing.T) {
	_, url, _ := openMCPSession(t)

	init := rpc(t, url, "initialize", map[string]any{"protocolVersion": "2025-03-26"})
	result, _ := init["result"].(map[string]any)
	if result == nil {
		t.Fatalf("initialize returned %v", init)
	}
	if _, ok := result["capabilities"].(map[string]any)["tools"]; !ok {
		t.Error("initialize does not advertise tools")
	}
	if instructions, _ := result["instructions"].(string); !strings.Contains(instructions, "falsifier PASSES when the claim is WRONG") {
		t.Error("initialize instructions do not state the falsifier convention")
	}

	list := rpc(t, url, "tools/list", nil)
	tools, _ := list["result"].(map[string]any)["tools"].([]any)
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.(map[string]any)["name"].(string)] = true
	}
	for _, want := range []string{"join_session", "list_checks", "submit_claim", "finish_reporting", "get_session_state", "say", "remark"} {
		if !names[want] {
			t.Errorf("tools/list is missing %q (got %v)", want, names)
		}
	}
}

// The whole point of the endpoint: an agent with no binary at all can join,
// claim, finish — and its finish is what lets adjudication run.
func TestMCPFullSessionFlow(t *testing.T) {
	h, url, id := openMCPSession(t)
	// A lone participant's finish is held in collect for SoloHold before the
	// session moves on; the test only cares that it eventually does.
	h.SoloHold = 100 * time.Millisecond

	if _, isErr := callTool(t, url, "submit_claim", map[string]any{
		"author": "mai@laptop/claude-code", "subject": "x.go:1", "assertion": "broken",
	}); !isErr {
		t.Fatal("submitting before joining should fail")
	}

	key := joinMCP(t, url, "mai@laptop/claude-code")

	if text, _ := callTool(t, url, "list_checks", nil); !strings.Contains(text, "go-test") {
		t.Fatalf("list_checks does not list the catalogue: %q", text)
	}

	text, isErr := callTool(t, url, "submit_claim", map[string]any{
		"author": "mai@laptop/claude-code", "participant_key": key, "subject": "x.go:1",
		"assertion": "cmd.Wait() never returns", "falsifier": "go-test",
	})
	if isErr || !strings.Contains(text, "go-test") {
		t.Fatalf("verifiable claim not recorded: %q (err=%v)", text, isErr)
	}

	if text, isErr = callTool(t, url, "submit_claim", map[string]any{
		"author": "mai@laptop/claude-code", "participant_key": key, "subject": "y.go:2", "assertion": "naming is off",
	}); isErr || !strings.Contains(text, "OPINION") {
		t.Fatalf("falsifier-less claim not flagged as opinion: %q (err=%v)", text, isErr)
	}

	if _, isErr = callTool(t, url, "finish_reporting", map[string]any{
		"author": "mai@laptop/claude-code", "participant_key": key,
	}); isErr {
		t.Fatal("finish_reporting failed")
	}

	// The MCP participant was the only one, so its finish must move the
	// session through adjudication to reveal.
	session, _ := h.Session(id)
	deadline := time.Now().Add(5 * time.Second)
	for session.Phase() != PhaseReveal {
		if time.Now().After(deadline) {
			t.Fatalf("session stuck in %s; an MCP-only roster never adjudicates", session.Phase())
		}
		time.Sleep(10 * time.Millisecond)
	}

	state, _ := callTool(t, url, "get_session_state", map[string]any{
		"author": "mai@laptop/claude-code", "participant_key": key,
	})
	if !strings.Contains(state, `"phase": "reveal"`) || !strings.Contains(state, "cmd.Wait() never returns") {
		t.Fatalf("state after reveal is missing phase or claims:\n%s", state)
	}

	if _, err := h.Finish(id); err != nil {
		t.Fatal(err)
	}
	state, _ = callTool(t, url, "get_session_state", map[string]any{
		"author": "mai@laptop/claude-code", "participant_key": key,
	})
	if !strings.Contains(state, `"outcome"`) {
		t.Fatalf("a poller cannot see the outcome after Finish:\n%s", state)
	}
}

// The session token is shared by the roster; only the participant_key proves
// which participant is calling. Without it, one reviewer could read another's
// blind claims, forge findings under their name, or close the collect phase
// over their head — all reproduced against the keyless version of this
// endpoint.
func TestMCPParticipantKeyStopsImpersonation(t *testing.T) {
	_, url, _ := openMCPSession(t)

	victimKey := joinMCP(t, url, "victim@laptop/claude")
	attackerKey := joinMCP(t, url, "attacker@pc/claude")

	if _, isErr := callTool(t, url, "submit_claim", map[string]any{
		"author": "victim@laptop/claude", "participant_key": victimKey,
		"subject": "secret.go:42", "assertion": "VICTIM-SECRET-ASSERTION", "falsifier": "go-test",
	}); isErr {
		t.Fatal("victim's own claim rejected")
	}

	// Reading the victim's blind claims with the attacker's key must fail.
	text, isErr := callTool(t, url, "get_session_state", map[string]any{
		"author": "victim@laptop/claude", "participant_key": attackerKey,
	})
	if !isErr {
		t.Fatalf("attacker read the victim's blind state:\n%s", text)
	}

	// Forging a claim as the victim must fail.
	if _, isErr := callTool(t, url, "submit_claim", map[string]any{
		"author": "victim@laptop/claude", "participant_key": attackerKey,
		"subject": "z.go:1", "assertion": "FORGED",
	}); !isErr {
		t.Fatal("attacker submitted a claim under the victim's name")
	}

	// Finishing the collect phase as the victim must fail.
	if _, isErr := callTool(t, url, "finish_reporting", map[string]any{
		"author": "victim@laptop/claude", "participant_key": attackerKey,
	}); !isErr {
		t.Fatal("attacker finished the collect phase over the victim's head")
	}

	// A caller with no key at all gets nothing either.
	if _, isErr := callTool(t, url, "get_session_state", map[string]any{
		"author": "nobody@nowhere/agent",
	}); !isErr {
		t.Fatal("an unjoined caller read session state")
	}
}

// A joined MCP participant that goes silent for longer than the collect
// window must stop holding adjudication open — it is the polling analogue of
// a websocket disconnect.
func TestMCPSilentParticipantStopsBlockingAdjudication(t *testing.T) {
	h, url, id := openMCPSession(t)

	activeKey := joinMCP(t, url, "active@pc/claude")
	joinMCP(t, url, "ghost@laptop/claude") // joins, then never calls again

	if _, isErr := callTool(t, url, "finish_reporting", map[string]any{
		"author": "active@pc/claude", "participant_key": activeKey,
	}); isErr {
		t.Fatal("finish_reporting failed")
	}

	session, _ := h.Session(id)
	time.Sleep(50 * time.Millisecond)
	if p := session.Phase(); p != PhaseCollect {
		t.Fatalf("session left collect (%s) while a live participant had not finished", p)
	}

	// Backdate the ghost's heartbeat past the collect window and nudge the
	// host the way its join timer would.
	h.mu.Lock()
	hs := h.sessions[id]
	h.mu.Unlock()
	hs.mu.Lock()
	hs.remote["ghost@laptop/claude"] = time.Now().Add(-CollectWindow - time.Minute)
	hs.mu.Unlock()
	h.maybeAdjudicate(hs)

	deadline := time.Now().Add(5 * time.Second)
	for session.Phase() == PhaseCollect {
		if time.Now().After(deadline) {
			t.Fatal("a silent participant still blocks adjudication after the collect window")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Adjudication must wait for a joined MCP agent that has not finished, even
// while every websocket participant is done — the remote set exists exactly
// so a session cannot close over a reviewer's head.
func TestMCPParticipantHoldsAdjudicationOpen(t *testing.T) {
	h, url, id := openMCPSession(t)
	// Shortened so the solo hold does not dominate the 5s adjudication wait —
	// this test is about the unfinished participant, not the hold.
	h.SoloHold = 100 * time.Millisecond

	key := joinMCP(t, url, "slow@pc/agent")

	session, _ := h.Session(id)
	// Nothing has finished; give the host a moment and confirm collect holds.
	time.Sleep(50 * time.Millisecond)
	if p := session.Phase(); p != PhaseCollect {
		t.Fatalf("session left collect (%s) with an unfinished MCP participant", p)
	}

	if _, isErr := callTool(t, url, "finish_reporting", map[string]any{
		"author": "slow@pc/agent", "participant_key": key,
	}); isErr {
		t.Fatal("finish_reporting failed")
	}
	deadline := time.Now().Add(5 * time.Second)
	for session.Phase() == PhaseCollect {
		if time.Now().After(deadline) {
			t.Fatal("session never adjudicated after the last participant finished")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestMCPNotificationsGetNoBody(t *testing.T) {
	_, url, _ := openMCPSession(t)
	resp, err := http.Post(url, "application/json",
		strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("notification got HTTP %d, want 202", resp.StatusCode)
	}
}

func TestMCPGetIsRefusedPolitely(t *testing.T) {
	_, url, _ := openMCPSession(t)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET got HTTP %d, want 405", resp.StatusCode)
	}
}

func TestMCPJoinCommandAndURL(t *testing.T) {
	url := MCPJoinURL("ws://192.168.1.5:9111", "abc123", "tok")
	if url != "http://192.168.1.5:9111/mcp/abc123?token=tok" {
		t.Errorf("MCPJoinURL = %q", url)
	}
	cmd := MCPJoinCommand("ws://192.168.1.5:9111", "abc123", "tok")
	for _, want := range []string{"claude mcp add", "--transport http", url} {
		if !strings.Contains(cmd, want) {
			t.Errorf("MCPJoinCommand = %q, missing %q", cmd, want)
		}
	}
}

// Guard against drift between the prompt and the endpoint: the prompt tells an
// agent which tools exist, so a rename here must fail the build.
func TestAgentJoinPromptNamesTheRealMCPTools(t *testing.T) {
	p := AgentJoinPrompt("ws://192.168.1.5:9111", "abc123", "tok", "subject")
	if !strings.Contains(p, MCPJoinURL("ws://192.168.1.5:9111", "abc123", "tok")) {
		t.Error("prompt does not carry the MCP URL")
	}
	toolNames := map[string]bool{}
	for _, tool := range mcpTools() {
		toolNames[tool["name"].(string)] = true
	}
	for _, named := range []string{"join_session", "list_checks", "submit_claim", "finish_reporting", "get_session_state", "say", "wait_for_chat"} {
		if !toolNames[named] {
			t.Errorf("prompt references tool %q which the endpoint does not serve", named)
		}
		if !strings.Contains(p, named) {
			t.Errorf("prompt does not mention tool %q", named)
		}
	}
	if !strings.Contains(p, fmt.Sprintf("claude mcp add --transport http agent-center-debate")) {
		t.Error("prompt does not carry the claude mcp add command")
	}
}
