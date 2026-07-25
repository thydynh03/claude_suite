package claims

import (
	"strings"
	"testing"
)

func newSessionWith(t *testing.T, providers map[string]string) *Session {
	t.Helper()
	s := NewSession("s1", "backend/cli/process_windows.go:17")
	for author, provider := range providers {
		if err := s.Join(author, provider); err != nil {
			t.Fatalf("join %s: %v", author, err)
		}
	}
	return s
}

func submit(t *testing.T, s *Session, id, author, assertion, falsifier string) *Claim {
	t.Helper()
	c := &Claim{
		ID: id, Author: author, Subject: s.Subject,
		Assertion: assertion, Falsifier: falsifier,
	}
	if err := s.Submit(c); err != nil {
		t.Fatalf("submit %s: %v", id, err)
	}
	return c
}

// A claim with no check is an opinion, whatever it asserts. This is what keeps a
// confidently hallucinated finding from blocking a merge.
func TestClaimWithoutFalsifierCannotBlock(t *testing.T) {
	s := newSessionWith(t, map[string]string{"an": "claude"})

	opinion := submit(t, s, "c1", "an", "this file should be split up", "")
	checkable := submit(t, s, "c2", "an", "/k hangs cmd.Wait()", "cli-visible-console")

	if opinion.Kind != KindOpinion {
		t.Fatalf("claim without a falsifier classified as %s", opinion.Kind)
	}
	if checkable.Kind != KindVerifiable {
		t.Fatalf("claim with a falsifier classified as %s", checkable.Kind)
	}

	opinion.Verdict = VerdictConfirmed
	if opinion.Blocking() {
		t.Fatal("an opinion is blocking a merge; nothing can disprove it")
	}

	checkable.Verdict = VerdictConfirmed
	if !checkable.Blocking() {
		t.Fatal("a confirmed, checkable defect should block")
	}
}

// The collect phase is blind. An agent that has read another's wording drifts
// toward it, so nothing is shared until the evidence is in.
func TestCollectPhaseHidesOtherAgentsClaims(t *testing.T) {
	s := newSessionWith(t, map[string]string{"an": "claude", "binh": "gemini"})

	submit(t, s, "c1", "an", "/k hangs cmd.Wait()", "cli-visible-console")
	submit(t, s, "c2", "binh", "/k is fine", "cli-visible-console")

	visible := s.VisibleTo("an")
	if len(visible) != 1 || visible[0].Author != "an" {
		t.Fatalf("an can see %d claims during collect, want only its own", len(visible))
	}
}

// Revealing before adjudicating is the failure this whole protocol exists to
// prevent: agents would take positions against each other rather than against
// the output.
func TestRevealIsRefusedBeforeAdjudication(t *testing.T) {
	s := newSessionWith(t, map[string]string{"an": "claude"})
	submit(t, s, "c1", "an", "/k hangs cmd.Wait()", "cli-visible-console")

	if err := s.Advance(PhaseReveal); err == nil {
		t.Fatal("reveal was allowed straight from collect")
	}

	if err := s.Advance(PhaseAdjudicate); err != nil {
		t.Fatalf("adjudicate: %v", err)
	}

	// Still refused: the falsifier has not actually been run.
	if err := s.Advance(PhaseReveal); err == nil {
		t.Fatal("reveal was allowed with a claim still pending")
	}

	if err := s.Settle("c1", VerdictConfirmed, "exit 1, timed out after 30s", 1); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if err := s.Advance(PhaseReveal); err != nil {
		t.Fatalf("reveal after adjudication: %v", err)
	}

	if got := len(s.VisibleTo("an")); got != 1 {
		t.Fatalf("after reveal an sees %d claims", got)
	}
}

// Evidence is not reopened by discussion.
func TestSettledClaimsCannotBeDebated(t *testing.T) {
	s := newSessionWith(t, map[string]string{"an": "claude", "binh": "gemini"})
	submit(t, s, "c1", "an", "/k hangs cmd.Wait()", "cli-visible-console")
	submit(t, s, "c2", "binh", "this file should be split up", "")

	mustAdvance(t, s, PhaseAdjudicate)
	if err := s.Settle("c1", VerdictConfirmed, "exit 1", 1); err != nil {
		t.Fatal(err)
	}
	mustAdvance(t, s, PhaseReveal)
	mustAdvance(t, s, PhaseDebate)

	if err := s.Remark("binh", "c1", "I still think it is fine"); err == nil {
		t.Fatal("a claim settled by evidence was reopened by talking about it")
	}
	if err := s.Remark("binh", "c2", "splitting it would help reviews"); err != nil {
		t.Fatalf("an opinion should be discussable: %v", err)
	}
}

// Rounds are capped by the host, not requested of the agents.
func TestDebateRoundsAreCapped(t *testing.T) {
	s := newSessionWith(t, map[string]string{"an": "claude", "binh": "gemini"})
	submit(t, s, "c1", "an", "this file should be split up", "")

	mustAdvance(t, s, PhaseAdjudicate)
	mustAdvance(t, s, PhaseReveal)

	for i := 0; i < MaxDebateRounds; i++ {
		if err := s.Advance(PhaseDebate); err != nil {
			t.Fatalf("round %d: %v", i+1, err)
		}
	}
	if err := s.Advance(PhaseDebate); err == nil {
		t.Fatalf("a %dth round was allowed", MaxDebateRounds+1)
	}
}

// Same-provider agents are allowed to debate, but the roster is recorded as a
// warning: two instances of one model share blind spots, so their agreement is
// not independent confirmation.
func TestSameProviderIsAllowedButWarned(t *testing.T) {
	s := newSessionWith(t, map[string]string{"an": "claude", "binh": "claude"})

	if !s.Homogeneous() {
		t.Fatal("a roster of two claude agents should report as homogeneous")
	}
	warnings := s.Warnings()
	if len(warnings) == 0 {
		t.Fatal("no warning was recorded for a same-provider roster")
	}
	if !strings.Contains(strings.ToLower(warnings[0]), "claude") {
		t.Fatalf("warning does not name the shared provider: %q", warnings[0])
	}

	// Allowed, not blocked: the session still runs.
	submit(t, s, "c1", "an", "/k hangs cmd.Wait()", "cli-visible-console")
	submit(t, s, "c2", "binh", "this file should be split up", "")
	mustAdvance(t, s, PhaseAdjudicate)
	if err := s.Settle("c1", VerdictConfirmed, "exit 1", 1); err != nil {
		t.Fatal(err)
	}
	mustAdvance(t, s, PhaseReveal)

	out := s.Close()
	if len(out.Warnings) == 0 {
		t.Fatal("the outcome dropped the homogeneous-roster warning")
	}
	if len(out.Blocking) != 1 {
		t.Fatalf("a homogeneous roster should still adjudicate: %d blocking", len(out.Blocking))
	}
}

func TestMixedProviderRosterHasNoWarning(t *testing.T) {
	s := newSessionWith(t, map[string]string{"an": "claude", "binh": "gemini"})
	if s.Homogeneous() {
		t.Fatal("claude + gemini reported as homogeneous")
	}
	if got := s.Warnings(); len(got) != 0 {
		t.Fatalf("unexpected warnings for a mixed roster: %v", got)
	}
}

// A check that never concluded is not a confirmation.
func TestUnrunChecksEscalateRatherThanConfirm(t *testing.T) {
	s := newSessionWith(t, map[string]string{"an": "claude"})
	submit(t, s, "c1", "an", "the build is broken", "build-all")

	mustAdvance(t, s, PhaseAdjudicate)
	if err := s.Settle("c1", VerdictInconclusive, "timed out after 300s", -1); err != nil {
		t.Fatal(err)
	}
	mustAdvance(t, s, PhaseReveal)

	out := s.Close()
	if len(out.Blocking) != 0 {
		t.Fatal("an inconclusive check produced a blocking finding")
	}
	if len(out.Escalated) != 1 {
		t.Fatalf("escalated %d claims, want 1", len(out.Escalated))
	}
}

// Losing positions stay on the record.
func TestOutcomeKeepsDissent(t *testing.T) {
	s := newSessionWith(t, map[string]string{"an": "claude", "binh": "gemini"})
	submit(t, s, "c1", "an", "this file should be split up", "")

	mustAdvance(t, s, PhaseAdjudicate)
	mustAdvance(t, s, PhaseReveal)
	mustAdvance(t, s, PhaseDebate)
	if err := s.Remark("binh", "c1", "splitting it would make blame useless"); err != nil {
		t.Fatal(err)
	}

	out := s.Close()
	if len(out.Dissent) != 1 {
		t.Fatalf("dissent dropped: %d remarks kept", len(out.Dissent))
	}
	if !strings.Contains(out.Summary(), "blame useless") {
		t.Fatal("the summary does not carry the dissenting position")
	}
}

// Nothing in this package counts agents. Majority voting is the weakest way to
// settle a disagreement between models that share a prior.
func TestOutcomeNeverDecidesByCountingAgents(t *testing.T) {
	s := newSessionWith(t, map[string]string{"an": "claude", "binh": "claude", "cuong": "claude"})

	// Two agents assert the same wrong thing; one asserts the opposite and is
	// backed by evidence.
	submit(t, s, "c1", "an", "/k is fine", "cli-visible-console")
	submit(t, s, "c2", "binh", "/k is fine", "cli-visible-console")
	submit(t, s, "c3", "cuong", "/k hangs cmd.Wait()", "cli-visible-console")

	mustAdvance(t, s, PhaseAdjudicate)
	for _, id := range []string{"c1", "c2"} {
		if err := s.Settle(id, VerdictRefuted, "exit 1, timed out", 1); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Settle("c3", VerdictConfirmed, "exit 1, timed out after 30s", 1); err != nil {
		t.Fatal(err)
	}
	mustAdvance(t, s, PhaseReveal)

	out := s.Close()
	if len(out.Blocking) != 1 || out.Blocking[0].Author != "cuong" {
		t.Fatalf("the majority overruled the evidence: blocking=%v", out.Blocking)
	}
	if len(out.Refuted) != 2 {
		t.Fatalf("refuted %d, want the two majority claims", len(out.Refuted))
	}
}

func TestJoiningAfterCollectIsRefused(t *testing.T) {
	s := newSessionWith(t, map[string]string{"an": "claude"})
	mustAdvance(t, s, PhaseAdjudicate)
	if err := s.Join("binh", "gemini"); err == nil {
		t.Fatal("an agent joined after the blind collect window closed")
	}
}

func mustAdvance(t *testing.T, s *Session, to Phase) {
	t.Helper()
	if err := s.Advance(to); err != nil {
		t.Fatalf("advance to %s: %v", to, err)
	}
}

// The operator opens a debate round explicitly, and only where there is
// something a check could not settle.
func TestOpenDebateNeedsAnUnsettledOpinion(t *testing.T) {
	s := newSessionWith(t, map[string]string{"an": "claude", "binh": "claude"})
	submit(t, s, "c1", "an", "/k hangs cmd.Wait()", "cli-visible-console")

	mustAdvance(t, s, PhaseAdjudicate)
	if err := s.Settle("c1", VerdictConfirmed, "exit 1", 1); err != nil {
		t.Fatal(err)
	}
	mustAdvance(t, s, PhaseReveal)

	// Every claim was settled by evidence; there is nothing to argue about.
	if err := s.Advance(PhaseDebate); err == nil {
		t.Fatal("a debate round opened with no opinions to discuss")
	}
	if got := s.OpenOpinions(); len(got) != 0 {
		t.Fatalf("OpenOpinions returned %d claims, want none", len(got))
	}
}

func TestRemarksAreReadableAndKeepLosingPositions(t *testing.T) {
	s := newSessionWith(t, map[string]string{"an": "claude", "binh": "claude"})
	submit(t, s, "c1", "an", "this file should be split up", "")

	mustAdvance(t, s, PhaseAdjudicate)
	mustAdvance(t, s, PhaseReveal)

	if got := s.OpenOpinions(); len(got) != 1 {
		t.Fatalf("OpenOpinions returned %d, want the one opinion", len(got))
	}

	mustAdvance(t, s, PhaseDebate)
	if err := s.Remark("binh", "c1", "splitting it would make blame useless"); err != nil {
		t.Fatal(err)
	}
	mustAdvance(t, s, PhaseDebate)
	if err := s.Remark("an", "c1", "the file is 3000 lines"); err != nil {
		t.Fatal(err)
	}

	got := s.Remarks()
	if len(got) != 2 {
		t.Fatalf("Remarks returned %d, want both turns", len(got))
	}
	if got[0].Round != 1 || got[1].Round != 2 {
		t.Fatalf("rounds recorded as %d and %d", got[0].Round, got[1].Round)
	}
	// Both sides survive: nothing is pruned for losing.
	joined := got[0].Text + " " + got[1].Text
	if !strings.Contains(joined, "blame useless") || !strings.Contains(joined, "3000 lines") {
		t.Fatalf("a position was dropped: %q", joined)
	}
}
