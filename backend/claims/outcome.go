package claims

import (
	"sort"
	"strings"
	"time"
)

// Outcome is what a session produces. It is written to every participant's
// machine, so it has to stand on its own without the live session.
type Outcome struct {
	SessionID string    `json:"session_id"`
	Subject   string    `json:"subject"`
	ClosedAt  time.Time `json:"closed_at"`

	// Blocking are confirmed, checkable defects. These are the only ones that
	// may stop a merge.
	Blocking []*Claim `json:"blocking"`
	// Refuted were checked and found not to hold. Kept deliberately: knowing
	// which review findings were wrong is how a team calibrates its agents.
	Refuted []*Claim `json:"refuted"`
	// Escalated need a person — opinions, and checks that could not conclude.
	Escalated []*Claim `json:"escalated"`

	// Dissent records positions that did not win. Consensus is not the goal, and
	// a minority view that turns out right months later is only recoverable if
	// it was written down.
	Dissent []Remark `json:"dissent"`

	// Warnings describe conditions that weaken this outcome, such as every agent
	// running the same model.
	Warnings []string `json:"warnings"`

	Participants []Participant `json:"participants"`
}

// Close freezes the session and produces its outcome.
//
// There is no vote anywhere in here. Counting agents was never the mechanism:
// evidence settles what it can, and everything else goes to a person.
func (s *Session) Close() *Outcome {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := &Outcome{
		SessionID: s.ID,
		Subject:   s.Subject,
		ClosedAt:  time.Now(),
		Warnings:  append([]string(nil), s.warnings...),
		Dissent:   append([]Remark(nil), s.remarks...),
	}

	for _, c := range s.claims {
		switch {
		case c.Kind == KindOpinion:
			c.Verdict = VerdictEscalated
			out.Escalated = append(out.Escalated, c)
		case c.Verdict == VerdictConfirmed:
			out.Blocking = append(out.Blocking, c)
		case c.Verdict == VerdictRefuted:
			out.Refuted = append(out.Refuted, c)
		default:
			// Pending or inconclusive: a check that timed out or never ran is not
			// a confirmation. Send it to a person rather than guessing.
			if c.Verdict == VerdictPending {
				c.Verdict = VerdictInconclusive
			}
			out.Escalated = append(out.Escalated, c)
		}
	}

	for _, p := range s.participants {
		out.Participants = append(out.Participants, *p)
	}
	sort.Slice(out.Participants, func(i, j int) bool {
		return out.Participants[i].Author < out.Participants[j].Author
	})

	s.Phase = PhaseRecord
	return out
}

// Summary renders the outcome for a terminal or a PR comment.
func (o *Outcome) Summary() string {
	var b strings.Builder

	b.WriteString("Adjudication: " + o.Subject + "\n")
	for _, w := range o.Warnings {
		b.WriteString("  ! " + w + "\n")
	}
	if len(o.Warnings) > 0 {
		b.WriteString("\n")
	}

	write := func(label string, claims []*Claim) {
		for _, c := range claims {
			b.WriteString("  " + label + "  " + c.Subject + "\n")
			b.WriteString("      " + c.Assertion + "\n")
			b.WriteString("      by " + c.Author + "\n")
			if c.Falsifier != "" {
				b.WriteString("      check: " + c.Falsifier + "\n")
			}
			if ev := strings.TrimSpace(c.Evidence); ev != "" {
				for _, line := range strings.Split(firstLines(ev, 4), "\n") {
					b.WriteString("      | " + line + "\n")
				}
			}
			b.WriteString("\n")
		}
	}

	write("BLOCKING ", o.Blocking)
	write("REFUTED  ", o.Refuted)
	write("ESCALATED", o.Escalated)

	if len(o.Dissent) > 0 {
		b.WriteString("  Dissenting positions kept on record:\n")
		for _, r := range o.Dissent {
			b.WriteString("    - " + r.Author + ": " + r.Text + "\n")
		}
	}
	return b.String()
}

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n") + "\n… (" + itoa(len(lines)-n) + " more lines)"
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
