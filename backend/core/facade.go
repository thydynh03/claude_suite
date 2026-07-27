// Package core holds what every frontend of Agent Center must agree on.
//
// There are two frontends over one backend: the Wails desktop app in app.go and
// the terminal UI in backend/tui. They are separate adapters by design — one
// serialises across a JavaScript boundary, the other returns errors and pushes
// events down a channel — but the capabilities behind them are meant to be the
// same set.
//
// Keeping them the same by convention did not work. Eight git and export
// capabilities drifted to different names on each side (GetGitStatus against
// GitStatus, CreateGitBranch against GitCreateBranch), and RunQuickCLI picked
// its provider from the model string on one side and from an argument on the
// other. Nothing failed; the two simply meant different things, and a reader had
// to compare them by hand to notice.
//
// Facade replaces that convention with a compile error.
package core

import (
	"agent_center/backend/models"
	"agent_center/backend/services"
)

// Facade is the capability set both frontends must offer.
//
// Adding a method here breaks the build of any adapter that has not implemented
// it, which is the entire point: a capability added to the desktop app and
// forgotten in the terminal UI stops being something a reviewer has to catch.
//
// An adapter may have methods beyond this — the desktop app has configuration,
// onboarding and OAuth that the terminal UI has no use for. Only the shared part
// is constrained.
type Facade interface {
	// ── Tasks ──
	CreateTask(task models.Task) error
	UpdateTaskStatus(taskID string, status string) error
	AssignTask(taskID string, assignedTo string) error
	DeleteTask(taskID string) error
	DeleteDoneTasks() error
	ClearAllTasks() error

	// ── Agents ──
	SaveAgent(agent models.Agent) error
	DeleteAgent(agentID string) error
	ResetAgentsToDefaults() error

	// ── Orchestrator ──
	// Start and Stop report the state afterwards, so a caller can tell a start
	// that took from one that did not.
	StartOrchestrator() bool
	StopOrchestrator() bool
	IsOrchestratorRunning() bool
	SetAutoApproveAll(enabled bool) bool
	GetAutoApproveAll() bool
	// ResolveApproval carries the task id on purpose. Without it the only
	// available call resolved every waiting approval at once, so answering the
	// prompt on screen also decided the ones the user could not see.
	ResolveApproval(taskID string, approved bool)

	// ── Workspace ──
	ScanWorkspaceFiles() ([]string, error)
	ReadFileContent(relPath string) (string, error)

	// ── Git ──
	GetGitStatus() (map[string]interface{}, error)
	GetGitBranches() (*services.GitBranchInfo, error)
	GetGitLog(limit int) ([]services.GitCommitInfo, error)
	CreateGitBranch(name string) error
	CheckoutGitBranch(name string) error
	CreateGitCommit(message string) error
	RevertGitCommit(hash string) error

	// ── Sessions and integrations ──
	GetActiveSession(provider string) string
	ClearActiveSession(provider string) error
	ToggleWebhook(port int) bool
	IsWebhookRunning() bool
	ExportKanbanReport() (string, error)
}

// Deliberately outside the Facade, with the reason, so that "why is this not
// shared" does not have to be rediscovered:
//
//   - RunBrowserTask. The desktop app drives an autonomous multi-step agent that
//     needs somewhere to render each step and a way to ask the user mid-run. The
//     terminal UI takes a URL and a screenshot flag. docs/ARCHITECTURE_DECISIONS.md
//     records this as intentional: the TUI is a keyboard inspector, and porting
//     the agent is a feature rather than a consistency fix.
//
//   - RunQuickCLI, RunPipeline, DecomposePlanWithProvider. These return rich
//     values across the Wails boundary and plain errors in the terminal, because
//     one renders a result panel and the other streams into a log. The logic they
//     share is provider selection, and that lives in ResolveProvider below rather
//     than being reimplemented on each side.
//
//   - StartOrchestrator, StopOrchestrator. Same capability, and both now report
//     whether the orchestrator is running afterwards, so they are in the Facade.
