package services

import (
	"testing"
	"time"
)

func watcher(t *testing.T, cfg GitWatchConfig) *GitWatchService {
	t.Helper()
	g := NewGitWatchService(t.TempDir())
	if err := g.SaveConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	return g
}

// The clock starts at the first dirty observation and the threshold is
// continuous dirtiness — a workspace that goes clean in between never nags.
func TestGitWatchNudgesOnlyAfterContinuousDirtyTime(t *testing.T) {
	g := watcher(t, GitWatchConfig{Enabled: true, ThresholdHours: 4})
	g.thresholdOverride = 10 * time.Minute

	dirty := true
	nudges := 0
	g.SetHooks(
		func() (bool, error) { return dirty, nil },
		func(time.Duration) { nudges++ },
		nil,
	)

	base := time.Now()
	g.tick(base)                        // clock starts
	g.tick(base.Add(5 * time.Minute))   // under threshold
	if nudges != 0 {
		t.Fatalf("nudged %d times before the threshold", nudges)
	}

	dirty = false
	g.tick(base.Add(6 * time.Minute)) // clean tick resets the clock
	dirty = true
	g.tick(base.Add(7 * time.Minute))  // clock restarts here
	g.tick(base.Add(12 * time.Minute)) // 5 dirty minutes — still under
	if nudges != 0 {
		t.Fatalf("the clean tick did not reset the clock (nudges=%d)", nudges)
	}

	g.tick(base.Add(18 * time.Minute)) // 11 continuous dirty minutes
	if nudges != 1 {
		t.Fatalf("nudges = %d after crossing the threshold, want 1", nudges)
	}

	// Still dirty a minute later: the nudge must not repeat every tick.
	g.tick(base.Add(19 * time.Minute))
	if nudges != 1 {
		t.Fatalf("nudge repeated within the hourly bound (nudges=%d)", nudges)
	}
}

func TestGitWatchAutoCommitClearsTheClock(t *testing.T) {
	g := watcher(t, GitWatchConfig{Enabled: true, ThresholdHours: 4, AutoCommit: true})
	g.thresholdOverride = 10 * time.Minute

	dirty := true
	commits, nudges := 0, 0
	g.SetHooks(
		func() (bool, error) { return dirty, nil },
		func(time.Duration) { nudges++ },
		func() error { commits++; dirty = false; return nil },
	)

	base := time.Now()
	g.tick(base)
	g.tick(base.Add(11 * time.Minute))
	if commits != 1 {
		t.Fatalf("commits = %d, want the auto-commit to fire once", commits)
	}
	if nudges != 0 {
		t.Fatalf("a successful auto-commit must not also nudge (nudges=%d)", nudges)
	}

	// Clean now; a later dirty stretch starts a fresh clock.
	g.tick(base.Add(12 * time.Minute))
	dirty = true
	g.tick(base.Add(13 * time.Minute))
	g.tick(base.Add(20 * time.Minute)) // 7 dirty minutes — under threshold
	if commits != 1 {
		t.Fatalf("auto-commit fired again before a full threshold elapsed (commits=%d)", commits)
	}
}

// A failed auto-commit must not be silent: the person should hear both that
// the diff is old and that the automation could not save it.
func TestGitWatchFailedAutoCommitFallsBackToNudging(t *testing.T) {
	g := watcher(t, GitWatchConfig{Enabled: true, ThresholdHours: 4, AutoCommit: true})
	g.thresholdOverride = time.Minute

	nudges := 0
	g.SetHooks(
		func() (bool, error) { return true, nil },
		func(time.Duration) { nudges++ },
		func() error { return errTest },
	)

	base := time.Now()
	g.tick(base)
	g.tick(base.Add(2 * time.Minute))
	if nudges != 1 {
		t.Fatalf("nudges = %d, want the failure surfaced as a nudge", nudges)
	}
}

// Disabled means silent — and stale state gathered while enabled must not
// fire the moment someone switches it back on.
func TestGitWatchDisabledStaysSilentAndResets(t *testing.T) {
	g := watcher(t, GitWatchConfig{Enabled: false, ThresholdHours: 4})
	g.thresholdOverride = time.Minute

	nudges := 0
	g.SetHooks(
		func() (bool, error) { return true, nil },
		func(time.Duration) { nudges++ },
		nil,
	)

	base := time.Now()
	g.tick(base)
	g.tick(base.Add(5 * time.Minute))
	if nudges != 0 {
		t.Fatalf("a disabled watcher nudged %d times", nudges)
	}

	// Re-enable: the config save reset the clock, so no instant nudge.
	if err := g.SaveConfig(GitWatchConfig{Enabled: true, ThresholdHours: 4}); err != nil {
		t.Fatal(err)
	}
	g.thresholdOverride = time.Minute
	g.tick(base.Add(6 * time.Minute))
	if nudges != 0 {
		t.Fatalf("stale dirty state fired immediately after enabling (nudges=%d)", nudges)
	}
}

func TestGitWatchConfigSurvivesReload(t *testing.T) {
	dir := t.TempDir()
	g := NewGitWatchService(dir)
	if err := g.SaveConfig(GitWatchConfig{Enabled: true, ThresholdHours: 7, AutoCommit: true}); err != nil {
		t.Fatal(err)
	}

	reloaded := NewGitWatchService(dir)
	cfg := reloaded.Config()
	if !cfg.Enabled || cfg.ThresholdHours != 7 || !cfg.AutoCommit {
		t.Fatalf("reloaded config = %+v", cfg)
	}
}

var errTest = &testError{}

type testError struct{}

func (*testError) Error() string { return "test failure" }
