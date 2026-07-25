package claims

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// agent is a test client speaking the host's protocol over a real socket.
type agent struct {
	t  *testing.T
	ws *websocket.Conn
}

func dial(t *testing.T, srv *httptest.Server, sessionID, token string) (*agent, *http.Response, error) {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/claims/" + sessionID + "?token=" + token
	ws, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, resp, err
	}
	t.Cleanup(func() { _ = ws.Close() })
	return &agent{t: t, ws: ws}, resp, nil
}

func (a *agent) send(m Message) {
	a.t.Helper()
	if err := a.ws.WriteJSON(m); err != nil {
		a.t.Fatalf("write %s: %v", m.Type, err)
	}
}

// await reads until a message of the wanted type arrives.
func (a *agent) await(want string) Message {
	a.t.Helper()
	_ = a.ws.SetReadDeadline(time.Now().Add(15 * time.Second))
	for {
		var m Message
		if err := a.ws.ReadJSON(&m); err != nil {
			a.t.Fatalf("waiting for %s: %v", want, err)
		}
		if m.Type == want {
			return m
		}
		if m.Type == MsgError && want != MsgError {
			a.t.Fatalf("host error while waiting for %s: %s", want, m.Error)
		}
	}
}

// awaitPhase reads state messages until the session reaches a phase.
func (a *agent) awaitPhase(p Phase) Message {
	a.t.Helper()
	_ = a.ws.SetReadDeadline(time.Now().Add(20 * time.Second))
	for {
		var m Message
		if err := a.ws.ReadJSON(&m); err != nil {
			a.t.Fatalf("waiting for phase %s: %v", p, err)
		}
		if m.Type == MsgError {
			a.t.Fatalf("host error while waiting for phase %s: %s", p, m.Error)
		}
		if m.Phase == p {
			return m
		}
	}
}

func hostFixture(t *testing.T, checks ...Check) (*Host, *httptest.Server) {
	t.Helper()
	ws := writeCatalogue(t, checks...)
	h := NewHost(runnerFor(t, ws))
	srv := httptest.NewServer(h.Handler())
	t.Cleanup(srv.Close)
	return h, srv
}

func TestWrongTokenIsRejected(t *testing.T) {
	h, srv := hostFixture(t)
	id, _, err := h.Open("subject", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if _, resp, err := dial(t, srv, id, "not-the-token"); err == nil {
		t.Fatal("connected with a wrong token")
	} else if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong token gave %v, want 401", resp)
	}
}

func TestExpiredSessionIsRejected(t *testing.T) {
	h, srv := hostFixture(t)
	id, token, err := h.Open("subject", time.Nanosecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)

	if _, resp, err := dial(t, srv, id, token); err == nil {
		t.Fatal("connected to an expired session")
	} else if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired session gave %v, want 401", resp)
	}
}

// Over the wire, an agent must not receive another's claims during collect. This
// is the anti-anchoring rule, enforced at the host rather than trusted to the
// client.
func TestCollectStaysBlindOverTheWire(t *testing.T) {
	h, srv := hostFixture(t, shellCheck("reproduces", 1))
	id, token, err := h.Open("backend/cli/process_windows.go:17", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	an, _, err := dial(t, srv, id, token)
	if err != nil {
		t.Fatal(err)
	}
	binh, _, err := dial(t, srv, id, token)
	if err != nil {
		t.Fatal(err)
	}

	an.send(Message{Type: MsgJoin, Author: "an", Provider: "claude"})
	an.await(MsgState)
	binh.send(Message{Type: MsgJoin, Author: "binh", Provider: "gemini"})
	binh.await(MsgState)

	an.send(Message{Type: MsgClaim, Claim: &Claim{
		Subject: "x", Assertion: "/k hangs cmd.Wait()", Falsifier: "reproduces",
	}})
	state := an.await(MsgState)
	if len(state.Claims) != 1 || state.Claims[0].Author != "an" {
		t.Fatalf("an sees %d claims after submitting", len(state.Claims))
	}

	binh.send(Message{Type: MsgClaim, Claim: &Claim{
		Subject: "x", Assertion: "it is fine", Falsifier: "reproduces",
	}})
	state = binh.await(MsgState)
	for _, c := range state.Claims {
		if c.Author != "binh" {
			t.Fatalf("binh can see %s's claim during collect", c.Author)
		}
	}
}

// The full path: both finish, the host runs the falsifiers, and only then does
// anyone see anyone else.
func TestSessionAdjudicatesThenReveals(t *testing.T) {
	h, srv := hostFixture(t, shellCheck("reproduces", 1), shellCheck("clean", 0))
	id, token, err := h.Open("subject", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	an, _, err := dial(t, srv, id, token)
	if err != nil {
		t.Fatal(err)
	}
	binh, _, err := dial(t, srv, id, token)
	if err != nil {
		t.Fatal(err)
	}

	an.send(Message{Type: MsgJoin, Author: "an", Provider: "claude"})
	an.await(MsgState)
	binh.send(Message{Type: MsgJoin, Author: "binh", Provider: "gemini"})
	binh.await(MsgState)

	an.send(Message{Type: MsgClaim, Claim: &Claim{
		ID: "c1", Subject: "x", Assertion: "the defect is real", Falsifier: "reproduces",
	}})
	an.await(MsgState)
	binh.send(Message{Type: MsgClaim, Claim: &Claim{
		ID: "c2", Subject: "x", Assertion: "no it is not", Falsifier: "clean",
	}})
	binh.await(MsgState)

	an.send(Message{Type: MsgDone})
	binh.send(Message{Type: MsgDone})

	revealed := an.awaitPhase(PhaseReveal)
	if len(revealed.Claims) != 2 {
		t.Fatalf("after reveal an sees %d claims, want both", len(revealed.Claims))
	}

	var confirmed, refuted int
	for _, c := range revealed.Claims {
		switch c.Verdict {
		case VerdictConfirmed:
			confirmed++
			if c.Evidence == "" && c.ExitCode == 0 {
				t.Errorf("claim %s confirmed with no evidence", c.ID)
			}
		case VerdictRefuted:
			refuted++
		default:
			t.Errorf("claim %s reached reveal as %s", c.ID, c.Verdict)
		}
	}
	if confirmed != 1 || refuted != 1 {
		t.Fatalf("confirmed=%d refuted=%d, want 1 and 1", confirmed, refuted)
	}

	out, err := h.Finish(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Blocking) != 1 || out.Blocking[0].ID != "c1" {
		t.Fatalf("blocking = %v, want c1", out.Blocking)
	}
}

// Same provider on both sides is allowed, and the agent is told at join time.
func TestSameProviderJoinsWithAWarning(t *testing.T) {
	h, srv := hostFixture(t, shellCheck("reproduces", 1))
	id, token, err := h.Open("subject", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	an, _, err := dial(t, srv, id, token)
	if err != nil {
		t.Fatal(err)
	}
	binh, _, err := dial(t, srv, id, token)
	if err != nil {
		t.Fatal(err)
	}

	an.send(Message{Type: MsgJoin, Author: "an", Provider: "claude"})
	an.await(MsgState)

	binh.send(Message{Type: MsgJoin, Author: "binh", Provider: "claude"})
	state := binh.await(MsgState)

	if len(state.Warnings) == 0 {
		t.Fatal("a same-provider join carried no warning")
	}
	if !strings.Contains(strings.ToLower(state.Note), "claude") {
		t.Fatalf("the note does not name the shared provider: %q", state.Note)
	}

	// Allowed: the session continues normally.
	binh.send(Message{Type: MsgClaim, Claim: &Claim{
		Subject: "x", Assertion: "the defect is real", Falsifier: "reproduces",
	}})
	binh.await(MsgState)
}

// An agent that never reports finished must not hold the session forever.
func TestForceAdjudicateEndsAStalledCollect(t *testing.T) {
	h, srv := hostFixture(t, shellCheck("reproduces", 1))
	id, token, err := h.Open("subject", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	an, _, err := dial(t, srv, id, token)
	if err != nil {
		t.Fatal(err)
	}
	an.send(Message{Type: MsgJoin, Author: "an", Provider: "claude"})
	an.await(MsgState)
	an.send(Message{Type: MsgClaim, Claim: &Claim{
		Subject: "x", Assertion: "the defect is real", Falsifier: "reproduces",
	}})
	an.await(MsgState)

	// No MsgDone from anyone.
	if err := h.ForceAdjudicate(id); err != nil {
		t.Fatalf("force: %v", err)
	}
	if got := an.awaitPhase(PhaseReveal); len(got.Claims) != 1 {
		t.Fatalf("after forced adjudication an sees %d claims", len(got.Claims))
	}
}

func TestClaimBeforeJoinIsRefused(t *testing.T) {
	h, srv := hostFixture(t)
	id, token, err := h.Open("subject", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	a, _, err := dial(t, srv, id, token)
	if err != nil {
		t.Fatal(err)
	}

	a.send(Message{Type: MsgClaim, Claim: &Claim{Subject: "x", Assertion: "a"}})
	if got := a.await(MsgError); !strings.Contains(got.Error, "join") {
		t.Fatalf("error = %q, want it to say join first", got.Error)
	}
}
