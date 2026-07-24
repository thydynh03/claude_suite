package tui

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"claude_suite/backend/cli"
	"claude_suite/backend/database"
	"claude_suite/backend/models"
	"claude_suite/backend/orchestrator"
	"claude_suite/backend/pipeline"
	"claude_suite/backend/services"
)

// RepositoryTaskActions adapts the shared task repository to the TUI. Opening
// write mode is explicit; it never creates a missing database or runs schema
// migrations.
type RepositoryTaskActions struct {
	db        *sql.DB
	repo      *database.TaskRepository
	plan      *pipeline.PlanBuilder
	exporter  *services.ExporterService
	runner    cli.CLIRunner
	context   *services.ContextManager
	orch      *orchestrator.Orchestrator
	events    chan RuntimeEvent
	workspace string
}

func OpenRepositoryTaskActions(path, workspace string) (*RepositoryTaskActions, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("open write database: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("database path is a directory: %s", absolute)
	}
	dsn := (&url.URL{Scheme: "file", Path: absolute, RawQuery: "mode=rw"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database read/write: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect database read/write: %w", err)
	}
	runner := cli.NewClaudeCLI()
	contextManager := services.NewContextManager()
	taskRepo := database.NewTaskRepository(db)
	orch := orchestrator.NewOrchestrator(
		database.NewAgentRepository(db), taskRepo, database.NewMemoryRepository(db),
		runner, contextManager, services.NewGitService(), services.NewBrowserAgentService(),
	)
	orch.SetWorkspaceDir(workspace)
	actions := &RepositoryTaskActions{
		db: db, repo: database.NewTaskRepository(db),
		plan: pipeline.NewPlanBuilder(runner), exporter: services.NewExporterService(),
		runner: runner, context: contextManager,
		orch: orch, events: make(chan RuntimeEvent, 128), workspace: workspace,
	}
	orch.SetEventHandlers(
		func(message, level string) {
			actions.publish(RuntimeEvent{Kind: "log", Message: message, Level: level})
		},
		func(agentName, taskTitle string) {
			actions.publish(RuntimeEvent{Kind: "approval", Agent: agentName, Task: taskTitle})
		},
		func() { actions.publish(RuntimeEvent{Kind: "board"}) },
	)
	return actions, nil
}

func (a *RepositoryTaskActions) CreateTask(task models.Task) error {
	if err := a.repo.Create(&task); err != nil {
		return err
	}
	return a.checkpoint()
}

func (a *RepositoryTaskActions) UpdateTaskStatus(taskID, status string) error {
	if err := a.repo.UpdateStatus(taskID, status, "", ""); err != nil {
		return err
	}
	return a.checkpoint()
}

func (a *RepositoryTaskActions) DeleteTask(taskID string) error {
	if err := a.repo.Delete(taskID); err != nil {
		return err
	}
	return a.checkpoint()
}

func (a *RepositoryTaskActions) DeleteDoneTasks() error {
	if err := a.repo.DeleteDone(); err != nil {
		return err
	}
	return a.checkpoint()
}

func (a *RepositoryTaskActions) ClearAllTasks() error {
	if err := a.repo.DeleteAll(); err != nil {
		return err
	}
	return a.checkpoint()
}

func (a *RepositoryTaskActions) DecomposePlan(requirement string) error {
	tasks, err := a.plan.Decompose(requirement, a.workspace)
	if err != nil {
		return err
	}
	for index := range tasks {
		if err := a.repo.Create(&tasks[index]); err != nil {
			return err
		}
	}
	return a.checkpoint()
}

func (a *RepositoryTaskActions) ExportReport() (string, error) {
	tasks, err := a.repo.GetAll()
	if err != nil {
		return "", err
	}
	markdown, _, err := a.exporter.ExportKanbanReport(tasks, a.workspace)
	return markdown, err
}

func (a *RepositoryTaskActions) RunQuickCLI(prompt string) (string, error) {
	fullPrompt := prompt
	if a.workspace != "" {
		if contextPrompt := a.context.BuildContextPrompt(a.workspace, nil); contextPrompt != "" {
			fullPrompt = contextPrompt + "\n\n" + prompt
		}
	}
	result := a.runner.RunOnce(fullPrompt, "claude-sonnet-4-5", "", func(message, level string) {
		a.publish(RuntimeEvent{Kind: "log", Message: message, Level: level})
	}, a.workspace)
	if result == nil {
		return "", fmt.Errorf("quick CLI returned no result")
	}
	if !result.Success {
		return result.Output, fmt.Errorf("%s", emptyError(result.Error, "quick CLI execution failed"))
	}
	return result.Output, nil
}

func emptyError(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func (a *RepositoryTaskActions) StartOrchestrator() { a.orch.Start() }
func (a *RepositoryTaskActions) StopOrchestrator()  { a.orch.Stop() }
func (a *RepositoryTaskActions) IsOrchestratorRunning() bool {
	return a.orch.IsRunning()
}
func (a *RepositoryTaskActions) ResolveApproval(approved bool) {
	a.orch.ResolveApproval(approved)
}
func (a *RepositoryTaskActions) Events() <-chan RuntimeEvent { return a.events }

func (a *RepositoryTaskActions) publish(event RuntimeEvent) {
	select {
	case a.events <- event:
	default:
	}
}

func (a *RepositoryTaskActions) checkpoint() error {
	_, err := a.db.Exec("PRAGMA wal_checkpoint(PASSIVE)")
	return err
}

func (a *RepositoryTaskActions) Close() error {
	if a == nil || a.db == nil {
		return nil
	}
	a.orch.Stop()
	return a.db.Close()
}
