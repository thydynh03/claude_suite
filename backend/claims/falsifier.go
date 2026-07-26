package claims

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"claude_suite/backend/sysproc"
)

// Check is one entry in the repository's catalogue of things a claim may point at.
//
// Command is an argv slice, never a shell string. That is deliberate: a claim
// arrives from another machine, and a string handed to a shell is a remote code
// execution hole wearing a costume. As argv there is nothing for `;` or `&&` or
// backticks to do.
type Check struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Command     []string `json:"command"`
	TimeoutSec  int      `json:"timeout_sec"`
}

// Catalogue is the set of checks a session is allowed to run.
//
// It lives in the repository and changes through review like any other code. An
// agent may only name an entry; it cannot supply a command. Agents run on other
// people's machines, and the host running their falsifiers has that person's ssh
// keys and source on disk.
type Catalogue struct {
	Checks []Check `json:"checks"`
}

// CatalogueFile is where the catalogue is read from, relative to the workspace.
const CatalogueFile = ".claude-suite/checks.json"

// LoadCatalogue reads the catalogue from a workspace. A missing file yields an
// empty catalogue, which means every claim is an opinion — safe, and obvious to
// diagnose.
func LoadCatalogue(workspace string) (*Catalogue, error) {
	path := filepath.Join(workspace, filepath.FromSlash(CatalogueFile))
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Catalogue{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", CatalogueFile, err)
	}

	var c Catalogue
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", CatalogueFile, err)
	}
	for i, check := range c.Checks {
		if strings.TrimSpace(check.Name) == "" {
			return nil, fmt.Errorf("%s: check %d has no name", CatalogueFile, i)
		}
		if len(check.Command) == 0 {
			return nil, fmt.Errorf("%s: check %q has no command", CatalogueFile, check.Name)
		}
	}
	return &c, nil
}

// AppendCheck adds one entry to the workspace's catalogue file.
//
// This deliberately relaxes the "changes through review like any other code"
// stance above, under strict conditions: it is reachable ONLY from the Memory
// page's guard-approval flow, where a human has just been shown the exact
// argv (never a summary) and clicked approve. The command stays an argv
// slice — nothing here accepts a shell string — and AutoSnapshot will commit
// the file, so the change still lands in history. Recorded in
// docs/ARCHITECTURE_DECISIONS.md.
func AppendCheck(workspace string, check Check) error {
	if strings.TrimSpace(check.Name) == "" {
		return fmt.Errorf("check cần có tên")
	}
	if len(check.Command) == 0 {
		return fmt.Errorf("check %q cần một lệnh dạng argv", check.Name)
	}
	if check.TimeoutSec <= 0 {
		check.TimeoutSec = 60
	}

	catalogue, err := LoadCatalogue(workspace)
	if err != nil {
		return err
	}
	if _, exists := catalogue.Lookup(check.Name); exists {
		return fmt.Errorf("check %q đã tồn tại trong catalogue", check.Name)
	}
	catalogue.Checks = append(catalogue.Checks, check)

	data, err := json.MarshalIndent(catalogue, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(workspace, filepath.FromSlash(CatalogueFile))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// Lookup finds a check by name.
func (c *Catalogue) Lookup(name string) (Check, bool) {
	for _, check := range c.Checks {
		if check.Name == name {
			return check, true
		}
	}
	return Check{}, false
}

// Names lists the catalogue, for error messages an agent can act on.
func (c *Catalogue) Names() []string {
	out := make([]string, 0, len(c.Checks))
	for _, check := range c.Checks {
		out = append(out, check.Name)
	}
	return out
}

// DefaultTimeout bounds a check that does not set its own.
const DefaultTimeout = 5 * time.Minute

// Runner executes catalogue checks in a workspace.
type Runner struct {
	Workspace string
	Catalogue *Catalogue
}

// Run adjudicates one claim.
//
// The convention, and it has to be stated because reading it backwards inverts
// every verdict: **a falsifier is a check that passes when the claim is wrong.**
// So a non-zero exit confirms the claim — the defect reproduced — and a zero
// exit refutes it.
func (r *Runner) Run(ctx context.Context, c *Claim) (Verdict, string, int) {
	if c.Kind != KindVerifiable {
		return VerdictEscalated, "no check was named; this is an opinion", 0
	}

	check, ok := r.Catalogue.Lookup(c.Falsifier)
	if !ok {
		return VerdictInconclusive, fmt.Sprintf(
			"%q is not in %s. Available: %s",
			c.Falsifier, CatalogueFile, strings.Join(r.Catalogue.Names(), ", "),
		), 0
	}

	// A catalogue entry with no command cannot decide anything — and indexing
	// it panicked the adjudication goroutine, which took the whole host down
	// with every connected agent. Malformed checks.json entries are user
	// input; they get a verdict, not a crash.
	if len(check.Command) == 0 {
		return VerdictInconclusive, fmt.Sprintf(
			"check %q in %s has no command to run", check.Name, CatalogueFile), -1
	}

	timeout := DefaultTimeout
	if check.TimeoutSec > 0 {
		timeout = time.Duration(check.TimeoutSec) * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := sysproc.CommandContext(runCtx, check.Command[0], check.Command[1:]...)
	cmd.Dir = r.Workspace
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	evidence := strings.TrimSpace(out.String())

	// A check that ran out of time proved nothing. Calling that a confirmation
	// would let a slow machine invent defects.
	if runCtx.Err() == context.DeadlineExceeded {
		return VerdictInconclusive, fmt.Sprintf("%s timed out after %s\n%s", check.Name, timeout, evidence), -1
	}
	if ctx.Err() != nil {
		return VerdictInconclusive, "adjudication was cancelled", -1
	}

	exitCode := 0
	if err != nil {
		exitCode = 1
		if ee, ok := err.(interface{ ExitCode() int }); ok {
			exitCode = ee.ExitCode()
		}
	}

	if exitCode != 0 {
		return VerdictConfirmed, evidence, exitCode
	}
	return VerdictRefuted, evidence, 0
}

// Adjudicate runs every pending falsifier in the session and records the results.
func (r *Runner) Adjudicate(ctx context.Context, s *Session) error {
	if s.Phase() != PhaseAdjudicate {
		return fmt.Errorf("session is in %s, not %s", s.Phase(), PhaseAdjudicate)
	}
	for _, c := range s.PendingFalsifiers() {
		verdict, evidence, code := r.Run(ctx, c)
		if err := s.Settle(c.ID, verdict, evidence, code); err != nil {
			return err
		}
	}
	return nil
}
