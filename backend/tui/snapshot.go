package tui

import (
	"claude_suite/backend/models"
	"claude_suite/backend/services"
)

// Snapshot is the read model consumed by the terminal frontend.
type Snapshot struct {
	Agents       []models.Agent
	Tasks        []models.Task
	TaskCounts   map[string]int
	AgentCounts  map[string]int
	RecentMemory []models.MemoryItem
	Workspace    string
}

// Loader is the integration seam for the shared read-only database backend.
type Loader func(path string) (Snapshot, error)

type RuntimeEvent struct {
	Kind, Message, Level, Agent, Task, TaskID string
}

// BrowserResult mirrors services.BrowserActionResult but swaps the raw
// base64 screenshot for a saved file path, since terminals cannot render
// inline images.
type BrowserResult struct {
	URL            string
	Title          string
	TextContent    string
	ScreenshotPath string
	Logs           []string
	Success        bool
	Error          string
}

// TaskActions is the frontend-neutral mutation contract shared by the Kanban
// UI and the application core. Implementations own persistence and validation;
// the terminal UI never writes SQL directly.
type TaskActions interface {
	CreateTask(task models.Task) error
	UpdateTaskStatus(taskID, status string) error
	AssignTask(taskID, assignedTo string) error
	DeleteTask(taskID string) error
	DeleteDoneTasks() error
	ClearAllTasks() error
	DecomposePlanWithProvider(requirement, provider, model string) error
	ExportReport() (string, error)
	RunQuickCLI(prompt, provider, model string, files []string) (string, error)
	GetActiveSession(provider string) string
	ClearActiveSession(provider string) error
	ScanWorkspaceFiles() ([]string, error)
	ReadFileContent(relPath string) (string, error)
	StartOrchestrator()
	StopOrchestrator()
	IsOrchestratorRunning() bool
	SetAutoApproveAll(enabled bool) bool
	GetAutoApproveAll() bool
	ResolveApproval(taskID string, approved bool)
	SaveAgent(agent models.Agent) error
	DeleteAgent(agentID string) error
	ResetAgentsToDefaults() error
	RunPipeline() error
	GitStatus() (map[string]interface{}, error)
	GitBranches() (*services.GitBranchInfo, error)
	GitLog(limit int) ([]services.GitCommitInfo, error)
	GitCreateBranch(name string) error
	GitCheckoutBranch(name string) error
	GitCommit(message string) error
	GitRevert(hash string) error
	ToggleWebhook(port int) bool
	IsWebhookRunning() bool
	RunBrowserTask(url string, screenshot bool) (BrowserResult, error)
	Events() <-chan RuntimeEvent
}
