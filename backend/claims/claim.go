// Package claims adjudicates disagreements between AI agents.
//
// The premise: agents disagreeing is not resolved by letting them argue. Left to
// argue, they converge on whichever side sounds most confident — measured across
// several studies, most stance flips under peer pressure move a correct answer to
// an incorrect one. So the protocol makes every conclusion carry the command that
// would disprove it, runs those commands, and lets the output decide. Discussion
// happens only for claims nothing can check, and never produces a verdict.
package claims

import (
	"fmt"
	"strings"
	"time"
)

// Kind is how a claim can be settled.
type Kind string

const (
	// KindVerifiable carries a falsifier the host can run.
	KindVerifiable Kind = "verifiable"
	// KindCostly can be checked, but the check is expensive enough that a human
	// decides whether to spend it.
	KindCostly Kind = "costly"
	// KindOpinion has no check: architecture, trade-offs, taste. These may be
	// discussed, and are always escalated rather than decided.
	KindOpinion Kind = "opinion"
)

// Verdict is the outcome of adjudication.
type Verdict string

const (
	VerdictPending      Verdict = "pending"
	VerdictConfirmed    Verdict = "confirmed"
	VerdictRefuted      Verdict = "refuted"
	VerdictInconclusive Verdict = "inconclusive"
	// VerdictEscalated means no machine can settle this; a person must.
	VerdictEscalated Verdict = "escalated"
)

// Claim is one agent's conclusion, stated so it can be proven wrong.
type Claim struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`

	// Author identifies the agent and the machine it ran on, e.g.
	// "an/claude-code". Two agents may share a provider; see Session.Homogeneous.
	Author   string `json:"author"`
	Provider string `json:"provider"`

	// Subject is what the claim is about: "backend/cli/process_windows.go:17",
	// a task id, or a pull request URL.
	Subject string `json:"subject"`

	// Assertion states the defect in one sentence.
	Assertion string `json:"assertion"`

	// Falsifier names a check from the repository's catalogue. Empty means the
	// claim is an opinion and cannot block anything.
	Falsifier string `json:"falsifier"`

	Kind Kind `json:"kind"`

	Verdict  Verdict `json:"verdict"`
	Evidence string  `json:"evidence"`
	ExitCode int     `json:"exit_code"`

	SubmittedAt time.Time `json:"submitted_at"`
	SettledAt   time.Time `json:"settled_at,omitempty"`
}

// Classify decides a claim's kind from what it carries. A claim is only
// verifiable if it names a check; asserting confidently is not evidence.
func (c *Claim) Classify() {
	if strings.TrimSpace(c.Falsifier) == "" {
		c.Kind = KindOpinion
		return
	}
	if c.Kind == KindCostly {
		return
	}
	c.Kind = KindVerifiable
}

// Blocking reports whether this claim may stop a merge.
//
// An opinion never blocks. This is the rule that keeps a hallucinated review
// from holding up work: a claim nobody can check is a suggestion, and a
// fabricated defect rarely survives being asked for the command that disproves
// it.
func (c *Claim) Blocking() bool {
	return c.Kind != KindOpinion && c.Verdict == VerdictConfirmed
}

// Validate rejects claims that cannot be adjudicated at all.
func (c *Claim) Validate() error {
	if strings.TrimSpace(c.Author) == "" {
		return fmt.Errorf("claim has no author")
	}
	if strings.TrimSpace(c.Subject) == "" {
		return fmt.Errorf("claim has no subject: name the file:line, task or PR it is about")
	}
	if strings.TrimSpace(c.Assertion) == "" {
		return fmt.Errorf("claim has no assertion")
	}
	return nil
}
