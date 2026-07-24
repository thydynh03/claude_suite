package tui

import "claude_suite/backend/models"

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
	Kind, Message, Level, Agent, Task string
}

// TaskActions is the frontend-neutral mutation contract shared by the Kanban
// UI and the application core. Implementations own persistence and validation;
// the terminal UI never writes SQL directly.
type TaskActions interface {
	CreateTask(task models.Task) error
	UpdateTaskStatus(taskID, status string) error
	DeleteTask(taskID string) error
	DeleteDoneTasks() error
	ClearAllTasks() error
	DecomposePlan(requirement string) error
	ExportReport() (string, error)
	RunQuickCLI(prompt string) (string, error)
	StartOrchestrator()
	StopOrchestrator()
	IsOrchestratorRunning() bool
	ResolveApproval(approved bool)
	Events() <-chan RuntimeEvent
}
