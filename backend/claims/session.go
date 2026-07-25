package claims

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Phase is where a session is in the protocol.
//
// The order matters more than anything else here. Adjudication happens before
// the agents ever see each other, so evidence lands before social pressure can.
// Reversing these two is what turns a review into a popularity contest.
type Phase string

const (
	// PhaseCollect gathers claims blind: nothing is broadcast until everyone has
	// submitted or the window closes, so no agent anchors on another's wording.
	PhaseCollect Phase = "collect"
	// PhaseAdjudicate runs the falsifiers. Nobody talks.
	PhaseAdjudicate Phase = "adjudicate"
	// PhaseReveal shows every claim together with the evidence for and against.
	PhaseReveal Phase = "reveal"
	// PhaseDebate is only for opinions, and produces no verdict.
	PhaseDebate Phase = "debate"
	// PhaseRecord freezes the transcript, dissent included.
	PhaseRecord Phase = "record"
)

// MaxDebateRounds is enforced by the host rather than requested of the agents.
// Past a couple of exchanges, further rounds mostly measure who repeats
// themselves most confidently.
const MaxDebateRounds = 2

// Participant is one agent connected to a session.
type Participant struct {
	Author   string    `json:"author"`
	Provider string    `json:"provider"`
	JoinedAt time.Time `json:"joined_at"`
	LastSeen time.Time `json:"last_seen"`
}

// Remark is one turn of discussion. Only opinions get these.
type Remark struct {
	Round   int       `json:"round"`
	Author  string    `json:"author"`
	ClaimID string    `json:"claim_id"`
	Text    string    `json:"text"`
	At      time.Time `json:"at"`
}

// Session is one disagreement being worked through.
type Session struct {
	mu sync.Mutex

	// ID, Subject and Opened are set once at construction and never written
	// again, so they are safe to read without the lock.
	ID      string    `json:"id"`
	Subject string    `json:"subject"`
	Opened  time.Time `json:"opened"`

	// phase is written by Advance and read from several goroutines: the host's
	// adjudication goroutine, each connection's read loop, and the runner. It
	// stays unexported so every reader has to go through Phase(), which takes
	// the lock. As an exported field it raced, and the race detector caught it.
	phase Phase

	participants map[string]*Participant
	claims       []*Claim
	remarks      []Remark
	debateRound  int

	// warnings surface conditions that weaken a session's result without
	// stopping it — the caller decides what to do about them.
	warnings []string
}

// NewSession opens a session for one subject.
func NewSession(id, subject string) *Session {
	return &Session{
		ID:           id,
		Subject:      subject,
		phase:        PhaseCollect,
		Opened:       time.Now(),
		participants: map[string]*Participant{},
	}
}

// Phase reports where the session is, safely from any goroutine.
func (s *Session) Phase() Phase {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.phase
}

// Join adds an agent.
//
// Agents sharing a provider are allowed. They are also recorded as a warning,
// because two instances of one model share the same blind spots: they can agree
// with each other while both being wrong, and their agreement is not additional
// evidence. The evidence phase does not care what model produced a claim, so a
// homogeneous roster still adjudicates normally — only the discussion phase is
// weakened.
func (s *Session) Join(author, provider string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(author) == "" {
		return fmt.Errorf("participant has no author id")
	}
	if s.phase != PhaseCollect {
		return fmt.Errorf("session %s is past the collect phase (%s); cannot join now", s.ID, s.phase)
	}
	if _, exists := s.participants[author]; exists {
		return fmt.Errorf("%s has already joined", author)
	}

	for _, p := range s.participants {
		if strings.EqualFold(p.Provider, provider) && provider != "" {
			s.addWarning(fmt.Sprintf(
				"%s and %s both run %s: agreement between them is not independent confirmation",
				p.Author, author, provider))
			break
		}
	}

	now := time.Now()
	s.participants[author] = &Participant{
		Author: author, Provider: provider, JoinedAt: now, LastSeen: now,
	}
	return nil
}

func (s *Session) addWarning(w string) {
	for _, existing := range s.warnings {
		if existing == w {
			return
		}
	}
	s.warnings = append(s.warnings, w)
}

// Warnings lists conditions that weaken this session's conclusions.
func (s *Session) Warnings() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.warnings...)
}

// Homogeneous reports whether every participant runs the same provider.
func (s *Session) Homogeneous() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.participants) < 2 {
		return false
	}
	var first string
	for _, p := range s.participants {
		if first == "" {
			first = p.Provider
			continue
		}
		if !strings.EqualFold(p.Provider, first) {
			return false
		}
	}
	return true
}

// Submit records a claim during the collect phase.
func (s *Session) Submit(c *Claim) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.phase != PhaseCollect {
		return fmt.Errorf("claims are only accepted during collect, session is in %s", s.phase)
	}
	if err := c.Validate(); err != nil {
		return err
	}
	if _, known := s.participants[c.Author]; !known {
		return fmt.Errorf("%s has not joined this session", c.Author)
	}

	c.SessionID = s.ID
	c.SubmittedAt = time.Now()
	c.Verdict = VerdictPending
	c.Classify()
	s.claims = append(s.claims, c)
	return nil
}

// VisibleTo returns what an agent is allowed to see right now.
//
// During collect this is only its own claims. That is the whole point of the
// phase: an agent that has read another's wording tends to drift toward it, and
// the drift is toward confidence rather than toward correctness.
//
// Copies, not pointers. A caller marshalling these to a websocket would
// otherwise be reading Verdict and Evidence while adjudication writes them.
func (s *Session) VisibleTo(author string) []*Claim {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []*Claim
	for _, c := range s.claims {
		if s.phase == PhaseCollect && c.Author != author {
			continue
		}
		snapshot := *c
		out = append(out, &snapshot)
	}
	return out
}

// PendingFalsifiers lists the claims adjudication should run, in a stable order.
func (s *Session) PendingFalsifiers() []*Claim {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Copies for the same reason as VisibleTo. The runner settles by id, so it
	// never needs to hold the live struct.
	var out []*Claim
	for _, c := range s.claims {
		if c.Kind == KindVerifiable && c.Verdict == VerdictPending {
			snapshot := *c
			out = append(out, &snapshot)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Settle records the outcome of running one claim's falsifier.
func (s *Session) Settle(claimID string, v Verdict, evidence string, exitCode int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, c := range s.claims {
		if c.ID != claimID {
			continue
		}
		c.Verdict = v
		c.Evidence = evidence
		c.ExitCode = exitCode
		c.SettledAt = time.Now()
		return nil
	}
	return fmt.Errorf("no claim %s in session %s", claimID, s.ID)
}

// Advance moves to the next phase, refusing orders that would break the protocol.
func (s *Session) Advance(to Phase) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch to {
	case PhaseAdjudicate:
		if s.phase != PhaseCollect {
			return fmt.Errorf("adjudicate follows collect, not %s", s.phase)
		}
	case PhaseReveal:
		// Refusing this is the core safeguard: revealing before the evidence is
		// in means agents form positions against each other rather than against
		// the output.
		if s.phase != PhaseAdjudicate {
			return fmt.Errorf("reveal follows adjudicate, not %s: agents must not see each other before the evidence is in", s.phase)
		}
		for _, c := range s.claims {
			if c.Kind == KindVerifiable && c.Verdict == VerdictPending {
				return fmt.Errorf("claim %s has not been adjudicated yet", c.ID)
			}
		}
	case PhaseDebate:
		if s.phase != PhaseReveal && s.phase != PhaseDebate {
			return fmt.Errorf("debate follows reveal, not %s", s.phase)
		}
		if !s.hasOpenOpinions() {
			return fmt.Errorf("nothing to debate: every claim was settled by evidence")
		}
		if s.debateRound >= MaxDebateRounds {
			return fmt.Errorf("debate is capped at %d rounds", MaxDebateRounds)
		}
		s.debateRound++
	case PhaseRecord:
		if s.phase == PhaseCollect {
			return fmt.Errorf("cannot record before adjudicating")
		}
	default:
		return fmt.Errorf("unknown phase %q", to)
	}

	s.phase = to
	return nil
}

func (s *Session) hasOpenOpinions() bool {
	for _, c := range s.claims {
		if c.Kind == KindOpinion {
			return true
		}
	}
	return false
}

// Remark records one turn of discussion. Only opinions can be discussed:
// anything the evidence already settled is not reopened by talking about it.
func (s *Session) Remark(author, claimID, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.phase != PhaseDebate {
		return fmt.Errorf("remarks are only accepted during debate, session is in %s", s.phase)
	}
	for _, c := range s.claims {
		if c.ID != claimID {
			continue
		}
		if c.Kind != KindOpinion {
			return fmt.Errorf("claim %s was settled by evidence (%s); it is not reopened by discussion", c.ID, c.Verdict)
		}
		s.remarks = append(s.remarks, Remark{
			Round: s.debateRound, Author: author, ClaimID: claimID, Text: text, At: time.Now(),
		})
		return nil
	}
	return fmt.Errorf("no claim %s in session %s", claimID, s.ID)
}

// DebateRound is how many rounds of discussion have opened.
func (s *Session) DebateRound() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.debateRound
}
