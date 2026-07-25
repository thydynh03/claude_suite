package tui

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	agents    *database.AgentRepository
	plan      *pipeline.PlanBuilder
	pipe      *pipeline.PipelineEngine
	exporter  *services.ExporterService
	runner    cli.CLIRunner
	context   *services.ContextManager
	orch      *orchestrator.Orchestrator
	git       *services.GitService
	webhook   *services.WebhookService
	browser   *services.BrowserAgentService
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
	agentRepo := database.NewAgentRepository(db)
	orch := orchestrator.NewOrchestrator(
		agentRepo, taskRepo, database.NewMemoryRepository(db),
		runner, contextManager, services.NewGitService(), services.NewBrowserAgentService(),
	)
	orch.SetWorkspaceDir(workspace)
	actions := &RepositoryTaskActions{
		db: db, repo: taskRepo, agents: agentRepo,
		plan: pipeline.NewPlanBuilder(runner), pipe: pipeline.NewPipelineEngine(agentRepo, runner),
		exporter: services.NewExporterService(),
		runner:   runner, context: contextManager,
		orch: orch, git: services.NewGitService(), webhook: services.NewWebhookService(taskRepo),
		browser: services.NewBrowserAgentService(),
		events:  make(chan RuntimeEvent, 128), workspace: workspace,
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

func (a *RepositoryTaskActions) AssignTask(taskID, assignedTo string) error {
	if err := a.repo.AssignTask(taskID, assignedTo); err != nil {
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

func (a *RepositoryTaskActions) DecomposePlanWithProvider(requirement, provider, model string) error {
	providerHint := "claude_cli"
	if provider == "anti" {
		providerHint = "anti_cli"
	}
	if model == "" {
		model = "claude-opus-4-8"
	}
	tasks, err := a.plan.DecomposeWithProvider(requirement, a.workspace, providerHint, model)
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

func (a *RepositoryTaskActions) RunQuickCLI(prompt, provider, model string, files []string) (string, error) {
	fullPrompt := prompt
	if a.workspace != "" {
		if contextPrompt := a.context.BuildContextPrompt(a.workspace, files); contextPrompt != "" {
			fullPrompt = contextPrompt + "\n\n" + prompt
		}
	}
	if model == "" {
		model = "claude-sonnet-4-5"
	}
	runner := a.runner
	if provider == "anti" {
		runner = cli.NewAntigravityCLI()
	}
	log := func(message, level string) {
		a.publish(RuntimeEvent{Kind: "log", Message: message, Level: level})
	}
	sessionID := cli.GetGlobalSession(provider)
	var result *cli.RunResult
	if sessionID != "" {
		result = runner.RunSession(fullPrompt, model, "", sessionID, log, a.workspace)
	} else {
		result = runner.RunOnce(fullPrompt, model, "", log, a.workspace)
	}
	if result == nil {
		return "", fmt.Errorf("quick CLI returned no result")
	}
	if result.SessionID != "" {
		cli.SetGlobalSession(provider, result.SessionID)
	}
	if !result.Success {
		return result.Output, fmt.Errorf("%s", emptyError(result.Error, "quick CLI execution failed"))
	}
	return result.Output, nil
}

func (a *RepositoryTaskActions) GetActiveSession(provider string) string {
	return cli.GetGlobalSession(provider)
}

func (a *RepositoryTaskActions) ClearActiveSession(provider string) error {
	cli.ClearGlobalSession(provider)
	return nil
}

func (a *RepositoryTaskActions) ScanWorkspaceFiles() ([]string, error) {
	if a.workspace == "" {
		return []string{}, nil
	}
	return a.context.ScanWorkspace(a.workspace)
}

func (a *RepositoryTaskActions) ReadFileContent(relPath string) (string, error) {
	fullPath := relPath
	if !filepath.IsAbs(relPath) && a.workspace != "" {
		fullPath = filepath.Join(a.workspace, relPath)
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
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
func (a *RepositoryTaskActions) SetAutoApproveAll(enabled bool) bool {
	a.orch.SetAutoApproveAll(enabled)
	return enabled
}
func (a *RepositoryTaskActions) GetAutoApproveAll() bool {
	return a.orch.GetAutoApproveAll()
}
func (a *RepositoryTaskActions) ResolveApproval(approved bool) {
	a.orch.ResolveApproval(approved)
}

func (a *RepositoryTaskActions) SaveAgent(agent models.Agent) error {
	if agent.AgentID == "" {
		return a.agents.Create(&agent)
	}
	return a.agents.Update(&agent)
}

func (a *RepositoryTaskActions) DeleteAgent(agentID string) error {
	return a.agents.Delete(agentID)
}

func (a *RepositoryTaskActions) ResetAgentsToDefaults() error {
	return a.agents.ResetToDefaults()
}

func (a *RepositoryTaskActions) RunPipeline() error {
	_, err := a.pipe.RunPipeline(a.pipe.GetDefaultSteps(), a.workspace)
	if err == nil {
		a.publish(RuntimeEvent{Kind: "board"})
	}
	return err
}

func (a *RepositoryTaskActions) GitStatus() (map[string]interface{}, error) {
	return a.git.GetStatus(a.workspace)
}

func (a *RepositoryTaskActions) GitBranches() (*services.GitBranchInfo, error) {
	return a.git.GetBranches(a.workspace)
}

func (a *RepositoryTaskActions) GitLog(limit int) ([]services.GitCommitInfo, error) {
	return a.git.GetLog(a.workspace, limit)
}

func (a *RepositoryTaskActions) GitCreateBranch(name string) error {
	return a.git.CreateBranch(a.workspace, name)
}

func (a *RepositoryTaskActions) GitCheckoutBranch(name string) error {
	return a.git.CheckoutBranch(a.workspace, name)
}

func (a *RepositoryTaskActions) GitCommit(message string) error {
	return a.git.CreateCommit(a.workspace, message)
}

func (a *RepositoryTaskActions) GitRevert(hash string) error {
	return a.git.RevertCommit(a.workspace, hash)
}

func (a *RepositoryTaskActions) ToggleWebhook(port int) bool {
	if a.webhook.IsRunning() {
		_ = a.webhook.Stop()
		return false
	}
	_ = a.webhook.Start(port)
	return true
}

func (a *RepositoryTaskActions) IsWebhookRunning() bool {
	return a.webhook.IsRunning()
}

func (a *RepositoryTaskActions) RunBrowserTask(url string, screenshot bool) (BrowserResult, error) {
	result, err := a.browser.RunBrowserTask(url, screenshot)
	if result == nil {
		return BrowserResult{Success: false, Error: emptyError(err.Error(), "browser task failed")}, err
	}
	out := BrowserResult{
		URL: result.URL, Title: result.Title, TextContent: result.TextContent,
		Logs: result.Logs, Success: result.Success, Error: result.Error,
	}
	if result.ScreenshotBase64 != "" {
		if path, saveErr := saveScreenshot(result.ScreenshotBase64); saveErr == nil {
			out.ScreenshotPath = path
		}
	}
	return out, err
}

func saveScreenshot(dataURI string) (string, error) {
	raw := strings.TrimPrefix(dataURI, "data:image/png;base64,")
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(os.TempDir(), "claude-suite-screenshots")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("screenshot-%d.png", time.Now().UnixNano()))
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return path, nil
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
