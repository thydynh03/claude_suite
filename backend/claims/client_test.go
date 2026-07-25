package claims

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// The whole path an IDE agent takes: connect, submit, finish, receive the
// outcome, and find it on disk where it can read it back.
func TestTwoClientsReachAnOutcomeOnDisk(t *testing.T) {
	ws := writeCatalogue(t, shellCheck("reproduces", 1), shellCheck("clean", 0))
	h := NewHost(runnerFor(t, ws))
	srv := httptest.NewServer(h.Handler())
	defer srv.Close()

	id, token, err := h.Open("backend/cli/process_windows.go:17", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	anOut := filepath.Join(t.TempDir(), "an")
	binhOut := filepath.Join(t.TempDir(), "binh")

	an := &Client{HostURL: wsURL, SessionID: id, Token: token,
		Author: "an", Provider: "claude", OutDir: anOut}
	binh := &Client{HostURL: wsURL, SessionID: id, Token: token,
		Author: "binh", Provider: "gemini", OutDir: binhOut}

	for _, c := range []*Client{an, binh} {
		if err := c.Connect(); err != nil {
			t.Fatalf("%s connect: %v", c.Author, err)
		}
		defer c.Close()
	}

	if err := an.Submit("process_windows.go:17", "/k hangs cmd.Wait()", "reproduces"); err != nil {
		t.Fatal(err)
	}
	if err := binh.Submit("process_windows.go:17", "/k is fine", "clean"); err != nil {
		t.Fatal(err)
	}
	// An opinion from binh, to prove it survives to the outcome without blocking.
	if err := binh.Submit("process_windows.go", "this file should be split up", ""); err != nil {
		t.Fatal(err)
	}

	// Both wait concurrently, as they would on two machines.
	var wg sync.WaitGroup
	outcomes := make([]*Outcome, 2)
	errs := make([]error, 2)
	for i, c := range []*Client{an, binh} {
		wg.Add(1)
		go func(i int, c *Client) {
			defer wg.Done()
			outcomes[i], errs[i] = c.Await(30 * time.Second)
		}(i, c)
	}

	if err := an.Done(); err != nil {
		t.Fatal(err)
	}
	if err := binh.Done(); err != nil {
		t.Fatal(err)
	}

	// Collect closes on its own once both are finished; the host then adjudicates
	// and reveals. Finish freezes it and sends the outcome.
	waitForPhase(t, h, id, PhaseReveal, 20*time.Second)
	if _, err := h.Finish(id); err != nil {
		t.Fatal(err)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("client %d await: %v", i, err)
		}
	}

	// Both machines must receive the same verdict; a per-machine truth would
	// defeat the point.
	for i, out := range outcomes {
		if len(out.Blocking) != 1 {
			t.Fatalf("client %d got %d blocking findings, want 1", i, len(out.Blocking))
		}
		if out.Blocking[0].Author != "an" {
			t.Fatalf("client %d: blocking claim attributed to %s", i, out.Blocking[0].Author)
		}
		if len(out.Refuted) != 1 {
			t.Fatalf("client %d got %d refuted, want binh's", i, len(out.Refuted))
		}
		if len(out.Escalated) != 1 {
			t.Fatalf("client %d got %d escalated, want the opinion", i, len(out.Escalated))
		}
	}

	// And it is on disk, which is how the IDE agent actually reads it.
	for _, dir := range []string{anOut, binhOut} {
		verdict := filepath.Join(dir, "session-"+id, "verdict.json")
		data, err := os.ReadFile(verdict)
		if err != nil {
			t.Fatalf("read %s: %v", verdict, err)
		}
		var parsed Outcome
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("verdict.json is not valid json: %v", err)
		}
		if parsed.SessionID != id {
			t.Fatalf("verdict.json is for session %s, want %s", parsed.SessionID, id)
		}
		if _, err := os.Stat(filepath.Join(dir, "session-"+id, "transcript.md")); err != nil {
			t.Fatalf("transcript missing: %v", err)
		}
	}
}

// A same-provider roster still produces a verdict, and the warning reaches the
// file the agent reads.
func TestHomogeneousRosterStillProducesAVerdict(t *testing.T) {
	ws := writeCatalogue(t, shellCheck("reproduces", 1))
	h := NewHost(runnerFor(t, ws))
	srv := httptest.NewServer(h.Handler())
	defer srv.Close()

	id, token, err := h.Open("subject", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	out := t.TempDir()

	an := &Client{HostURL: wsURL, SessionID: id, Token: token, Author: "an", Provider: "claude", OutDir: out}
	binh := &Client{HostURL: wsURL, SessionID: id, Token: token, Author: "binh", Provider: "claude", OutDir: out}
	for _, c := range []*Client{an, binh} {
		if err := c.Connect(); err != nil {
			t.Fatalf("%s: %v", c.Author, err)
		}
		defer c.Close()
	}

	if err := an.Submit("x", "the defect is real", "reproduces"); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	var outcome *Outcome
	var awaitErr error
	wg.Add(1)
	go func() { defer wg.Done(); outcome, awaitErr = an.Await(30 * time.Second) }()

	_ = an.Done()
	_ = binh.Done()
	waitForPhase(t, h, id, PhaseReveal, 20*time.Second)
	if _, err := h.Finish(id); err != nil {
		t.Fatal(err)
	}
	wg.Wait()

	if awaitErr != nil {
		t.Fatal(awaitErr)
	}
	if len(outcome.Blocking) != 1 {
		t.Fatalf("a homogeneous roster produced %d blocking findings, want 1", len(outcome.Blocking))
	}
	if len(outcome.Warnings) == 0 {
		t.Fatal("the warning about a same-provider roster did not reach the outcome")
	}
	if !strings.Contains(strings.ToLower(outcome.Summary()), "claude") {
		t.Fatal("the transcript does not mention the shared provider")
	}
}

func waitForPhase(t *testing.T, h *Host, id string, want Phase, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if s, ok := h.Session(id); ok && s.Phase() == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("session %s never reached %s", id, want)
}
