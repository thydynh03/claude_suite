package cli

import "claude_suite/backend/models"

// RunResult contains output and metrics from a CLI execution
type RunResult struct {
	Success     bool    `json:"success"`
	Output      string  `json:"output"`
	Error       string  `json:"error"`
	SessionID   string  `json:"session_id"`
	TokensUsed  int64   `json:"tokens_used"`
	DurationSec float64 `json:"duration_sec"`
}

// LogCallback functions report real-time output line-by-line
type LogCallback func(msg string, level string)

// CLIRunner defines standard interface for executing AI models
type CLIRunner interface {
	RunAgent(agent *models.Agent, prompt string, onLog LogCallback, cwd string) *RunResult
	RunOnce(prompt string, model string, system string, onLog LogCallback, cwd string) *RunResult
}
