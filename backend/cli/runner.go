package cli

import (
	"context"
	"sync/atomic"
	"time"

	"claude_suite/backend/models"
)

// taskTimeoutNanos is the per-execution CLI timeout (default 10m), configurable
// at runtime via SetTaskTimeout. Stored as int64 nanoseconds for atomic access.
var taskTimeoutNanos int64 = int64(10 * time.Minute)

// SetTaskTimeout sets the per-task CLI timeout. Values < 1 minute are clamped.
func SetTaskTimeout(d time.Duration) {
	if d < time.Minute {
		d = time.Minute
	}
	atomic.StoreInt64(&taskTimeoutNanos, int64(d))
}

// TaskTimeout returns the current per-task CLI timeout.
func TaskTimeout() time.Duration {
	return time.Duration(atomic.LoadInt64(&taskTimeoutNanos))
}

// RunResult contains output and metrics from a CLI execution
type RunResult struct {
	Success     bool    `json:"success"`
	Output      string  `json:"output"`
	Error       string  `json:"error"`
	SessionID   string  `json:"session_id"`
	TokensUsed  int64   `json:"tokens_used"`
	CostUSD     float64 `json:"cost_usd"`
	DurationSec float64 `json:"duration_sec"`
}

// LogCallback functions report real-time output line-by-line
type LogCallback func(msg string, level string)

// CLIRunner defines standard interface for executing AI models
type CLIRunner interface {
	RunAgent(agent *models.Agent, prompt string, onLog LogCallback, cwd string) *RunResult
	// RunAgentCtx runs an agent bound to a cancellable context so a single task
	// can be stopped mid-execution (kills the underlying CLI process).
	RunAgentCtx(ctx context.Context, agent *models.Agent, prompt string, onLog LogCallback, cwd string) *RunResult
	RunOnce(prompt string, model string, system string, onLog LogCallback, cwd string) *RunResult
	RunSession(prompt string, model string, system string, sessionID string, onLog LogCallback, cwd string) *RunResult
}
