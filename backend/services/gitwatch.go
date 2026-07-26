package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// GitWatchConfig is what the user decides about the watcher; it persists in
// the data directory like the other configs.
type GitWatchConfig struct {
	Enabled bool `json:"enabled"`
	// ThresholdHours is how long the workspace may stay dirty before the
	// watcher acts. Whole hours: the point is "a night of agent work is
	// sitting uncommitted", not minute-precision.
	ThresholdHours int `json:"threshold_hours"`
	// AutoCommit commits (with an AI-written message) instead of only
	// nudging. Off by default — writing history on someone's behalf is a
	// decision they make, not a default they discover.
	AutoCommit bool `json:"auto_commit"`
}

// GitWatchService nudges — or commits — when the workspace has been
// continuously dirty for longer than the threshold.
//
// AutoSnapshot protects the diff DURING each task; nothing watched what
// happens after: a night of agent work routinely sat as one loose diff until
// someone remembered. The service measures dirty-time itself (first tick that
// finds changes starts the clock; a clean tick resets it), so a workspace
// that is merely busy is never mistaken for one that is abandoned.
type GitWatchService struct {
	mu         sync.Mutex
	cfg        GitWatchConfig
	storePath  string
	dirtySince time.Time
	lastNudge  time.Time
	// lastInspectErr throttles the "cannot inspect the workspace" report to the
	// same cadence as a nudge: the check runs every two minutes, and a machine
	// without git fails every single one of them.
	lastInspectErr time.Time
	running        bool
	stopCh         chan struct{}

	// Wired by the app; the service knows nothing about git or the UI.
	isDirty    func() (bool, error)
	onNudge    func(dirtyFor time.Duration)
	autoCommit func() error

	// thresholdOverride lets tests use sub-hour thresholds; zero means the
	// configured hours apply.
	thresholdOverride time.Duration
}

// DefaultGitWatchThresholdHours is the out-of-the-box "this has sat too
// long": long enough to ignore an afternoon of active work, short enough
// that overnight agent output is flagged by morning.
const DefaultGitWatchThresholdHours = 4

// gitWatchNudgeEvery bounds how often the non-committing nudge repeats while
// the workspace stays dirty. Once an hour is a reminder; once a tick is spam.
const gitWatchNudgeEvery = time.Hour

func NewGitWatchService(dir string) *GitWatchService {
	g := &GitWatchService{
		cfg:       GitWatchConfig{Enabled: false, ThresholdHours: DefaultGitWatchThresholdHours},
		storePath: filepath.Join(dir, "gitwatch_config.json"),
	}
	g.load()
	return g
}

// SetHooks wires the watcher's senses and hands. Call before Start.
func (g *GitWatchService) SetHooks(isDirty func() (bool, error), onNudge func(dirtyFor time.Duration), autoCommit func() error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.isDirty = isDirty
	g.onNudge = onNudge
	g.autoCommit = autoCommit
}

func (g *GitWatchService) load() {
	data, err := os.ReadFile(g.storePath)
	if err != nil {
		return
	}
	var cfg GitWatchConfig
	if json.Unmarshal(data, &cfg) == nil {
		if cfg.ThresholdHours <= 0 {
			cfg.ThresholdHours = DefaultGitWatchThresholdHours
		}
		g.cfg = cfg
	}
}

func (g *GitWatchService) Config() GitWatchConfig {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.cfg
}

// SaveConfig persists a new configuration and resets the dirty clock, so a
// freshly enabled watcher measures from now rather than from state gathered
// while it was off.
func (g *GitWatchService) SaveConfig(cfg GitWatchConfig) error {
	if cfg.ThresholdHours <= 0 {
		cfg.ThresholdHours = DefaultGitWatchThresholdHours
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cfg = cfg
	g.dirtySince = time.Time{}
	g.lastNudge = time.Time{}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(g.storePath, data, 0o600)
}

func (g *GitWatchService) Start() {
	g.mu.Lock()
	if g.running {
		g.mu.Unlock()
		return
	}
	g.running = true
	g.stopCh = make(chan struct{})
	stop := g.stopCh
	g.mu.Unlock()

	go g.loop(stop)
}

func (g *GitWatchService) Stop() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.running {
		return
	}
	g.running = false
	close(g.stopCh)
}

func (g *GitWatchService) loop(stop <-chan struct{}) {
	// Two minutes: dirty-time is measured in hours, and a git status every
	// couple of minutes is invisible next to the agents' own git traffic.
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			g.tick(time.Now())
		}
	}
}

func (g *GitWatchService) threshold() time.Duration {
	if g.thresholdOverride > 0 {
		return g.thresholdOverride
	}
	return time.Duration(g.cfg.ThresholdHours) * time.Hour
}

// tick is one observation. Split from the loop so tests can drive time.
func (g *GitWatchService) tick(now time.Time) {
	g.mu.Lock()
	cfg := g.cfg
	isDirty := g.isDirty
	onNudge := g.onNudge
	autoCommit := g.autoCommit
	g.mu.Unlock()

	if !cfg.Enabled || isDirty == nil {
		g.mu.Lock()
		g.dirtySince = time.Time{}
		g.mu.Unlock()
		return
	}

	dirty, err := isDirty()
	if err != nil {
		// A workspace that cannot be inspected is not one to nag about — but a
		// permanently failing check (git missing, workspace gone) must not look
		// like a healthy quiet one, so it is reported at the nudge cadence.
		g.mu.Lock()
		report := now.Sub(g.lastInspectErr) >= gitWatchNudgeEvery
		if report {
			g.lastInspectErr = now
		}
		g.mu.Unlock()
		if report && onNudge != nil {
			onNudge(0)
		}
		return
	}

	g.mu.Lock()
	if !dirty {
		g.dirtySince = time.Time{}
		g.lastNudge = time.Time{}
		g.mu.Unlock()
		return
	}
	if g.dirtySince.IsZero() {
		g.dirtySince = now
	}
	dirtyFor := now.Sub(g.dirtySince)
	over := dirtyFor >= g.threshold()
	nudgeDue := now.Sub(g.lastNudge) >= gitWatchNudgeEvery
	if over && nudgeDue {
		g.lastNudge = now
	}
	g.mu.Unlock()

	if !over || !nudgeDue {
		return
	}

	if cfg.AutoCommit && autoCommit != nil {
		if err := autoCommit(); err == nil {
			// Committed: the tree is clean again and the clock restarts on
			// the next real change.
			g.mu.Lock()
			g.dirtySince = time.Time{}
			g.lastNudge = time.Time{}
			g.mu.Unlock()
			return
		}
		// A failed auto-commit falls through to the nudge: the person should
		// hear that BOTH the diff is old and the automation could not save it.
	}
	if onNudge != nil {
		onNudge(dirtyFor)
	}
}

// String describes the watcher for logs.
func (g *GitWatchService) String() string {
	cfg := g.Config()
	if !cfg.Enabled {
		return "git watch: tắt"
	}
	mode := "nhắc"
	if cfg.AutoCommit {
		mode = "tự commit"
	}
	return fmt.Sprintf("git watch: %s sau %dh workspace bẩn", mode, cfg.ThresholdHours)
}
