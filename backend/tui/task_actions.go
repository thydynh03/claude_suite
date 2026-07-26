package tui

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"claude_suite/backend/claims"
	"claude_suite/backend/cli"
	"claude_suite/backend/core"
	"claude_suite/backend/database"
	"claude_suite/backend/defaults"
	"claude_suite/backend/models"
	"claude_suite/backend/orchestrator"
	"claude_suite/backend/pipeline"
	"claude_suite/backend/provider"
	"claude_suite/backend/services"
	"claude_suite/backend/services/projectmap"
)

// The terminal UI must offer the same shared capabilities as the desktop app.
// See core.Facade for why this is a compile-time assertion rather than a
// convention: the two drifted to different method names for eight git and export
// capabilities, and nothing failed until someone compared them by hand.
var _ core.Facade = (*RepositoryTaskActions)(nil)

// tuiCheapModel runs the TUI's background memory work — same model choice as
// the desktop app's memoryCheapModel.
const tuiCheapModel = "claude-haiku-4-5"

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
	mapper    *projectmap.Mapper
	lessons   *database.LessonRepository
	regress   *database.RegressionRepository
	learner   *services.Learner
	orch      *orchestrator.Orchestrator
	git       *services.GitService
	webhook   *services.WebhookService
	browser   *services.BrowserAgentService
	events    chan RuntimeEvent
	workspace string
	dataDir   string
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
	dsn := sqliteURI(absolute, "mode=rw")
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
	attemptRepo := database.NewAttemptRepository(db)
	wsRepo := database.NewWorkspaceRepository(db)
	obsRepo := database.NewObservationRepository(db)
	lessonRepo := database.NewLessonRepository(db)
	regressionRepo := database.NewRegressionRepository(db)
	gitSvc := services.NewGitService()
	mapper := projectmap.NewMapper(database.NewCodeGraphRepository(db), wsRepo)
	mapper.SetSummarizer(runner, tuiCheapModel)
	contextManager.AttachRepos(taskRepo, attemptRepo)
	contextManager.AttachProjectMapper(mapper)
	contextManager.AttachMemoryRepos(lessonRepo, regressionRepo)
	learner := services.NewLearner(lessonRepo, obsRepo, wsRepo, attemptRepo, regressionRepo, gitSvc, runner, tuiCheapModel)
	orch := orchestrator.NewOrchestrator(
		agentRepo, taskRepo, database.NewMemoryRepository(db),
		runner, contextManager, gitSvc, services.NewBrowserAgentService(),
	)
	orch.SetAttemptRepo(attemptRepo)
	orch.SetMemoryStores(wsRepo, obsRepo)
	orch.SetProjectMapper(mapper)
	orch.SetLearner(learner)
	learner.Start()
	orch.SetWorkspaceDir(workspace)
	// Same stored knobs as the desktop app — one config file, two frontends.
	memCfg := services.LoadMemoryConfig(filepath.Dir(absolute))
	orch.SetPackBudget(memCfg.ContextPackMaxChars)
	orch.SetSessionResume(memCfg.SessionResume)
	mapper.SetAutoSummarize(memCfg.AutoSummarize)
	learner.SetPromotionEnabled(memCfg.LessonPromotion)
	// Seed role definitions beside the database the same way the desktop app
	// does at startup: the orchestrator reads <dataDir>/roles on every task run
	// and silently skips missing files, so a TUI-only install would otherwise
	// run agents without any role context.
	if _, err := defaults.SeedRoles(filepath.Join(filepath.Dir(absolute), "roles")); err != nil {
		fmt.Printf("Could not seed agent roles: %v\n", err)
	}
	// A crash or forced exit leaves tasks marked "running", and the dispatcher
	// skips those forever. The Wails frontend recovers them on startup; write-mode
	// TUI sessions have to do the same or those tasks are stuck permanently.
	staleTasks, err := taskRepo.ResetRunningTasks()
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("recover tasks left running: %w", err)
	}
	planBuilder := pipeline.NewPlanBuilder(runner)
	planBuilder.AttachBoard(taskRepo)
	planBuilder.AttachRegressions(regressionRepo)
	actions := &RepositoryTaskActions{
		db: db, repo: taskRepo, agents: agentRepo,
		plan: planBuilder, pipe: pipeline.NewPipelineEngine(agentRepo, runner),
		exporter: services.NewExporterService(),
		runner:   runner, context: contextManager, mapper: mapper,
		lessons: lessonRepo, regress: regressionRepo, learner: learner,
		orch: orch, git: gitSvc, webhook: services.NewWebhookService(taskRepo),
		browser: services.NewBrowserAgentService(),
		events:  make(chan RuntimeEvent, 128), workspace: workspace,
		dataDir: filepath.Dir(absolute),
	}
	// Named rather than three positional callbacks: swapping two of them compiles
	// and produces a UI that quietly stops updating.
	core.AttachEventSink(orch, core.EventFuncs{
		OnLog: func(message, level string) {
			actions.publish(RuntimeEvent{Kind: "log", Message: message, Level: level})
		},
		OnApproval: func(taskID, agentName, taskTitle string) {
			actions.publish(RuntimeEvent{Kind: "approval", TaskID: taskID, Agent: agentName, Task: taskTitle})
		},
		OnBoard: func() { actions.publish(RuntimeEvent{Kind: "board"}) },
	})
	if staleTasks > 0 {
		// The channel is buffered, so this survives until the UI starts draining it.
		actions.publish(RuntimeEvent{
			Kind:    "log",
			Level:   "WARN",
			Message: fmt.Sprintf("Recovered %d task(s) left running by a previous session.", staleTasks),
		})
	}
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

func (a *RepositoryTaskActions) DecomposePlanWithProvider(requirement, providerName, model string) error {
	providerHint := provider.RunnerKey(provider.ResolveProvider(providerName, model))
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

func (a *RepositoryTaskActions) ExportKanbanReport() (string, error) {
	tasks, err := a.repo.GetAll()
	if err != nil {
		return "", err
	}
	markdown, _, err := a.exporter.ExportKanbanReport(tasks, a.workspace)
	return markdown, err
}

func (a *RepositoryTaskActions) RunQuickCLI(prompt, providerName, model string, files []string) (string, error) {
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
	if provider.IsAnti(provider.ResolveProvider(providerName, model)) {
		runner = cli.NewAntigravityCLI()
	}
	log := func(message, level string) {
		a.publish(RuntimeEvent{Kind: "log", Message: message, Level: level})
	}
	sessionID := cli.GetGlobalSession(providerName)
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
		cli.SetGlobalSession(providerName, result.SessionID)
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

// ── Project Map (name parity with the desktop app) ──────────────────────

func (a *RepositoryTaskActions) RebuildProjectMap() (*projectmap.BuildReport, error) {
	if a.workspace == "" {
		return nil, fmt.Errorf("no workspace selected")
	}
	return a.mapper.FullBuild(a.workspace)
}

func (a *RepositoryTaskActions) GetProjectMap() (*models.ProjectMapStats, error) {
	if a.workspace == "" {
		return nil, fmt.Errorf("no workspace selected")
	}
	return a.mapper.Stats(a.workspace)
}

func (a *RepositoryTaskActions) GetProjectMapStaleness() (models.StalenessReport, error) {
	if a.workspace == "" {
		return models.StalenessReport{Status: "unknown"}, nil
	}
	return a.mapper.Staleness(a.workspace), nil
}

func (a *RepositoryTaskActions) RefreshProjectSummaries() (int, error) {
	if a.workspace == "" {
		return 0, fmt.Errorf("no workspace selected")
	}
	return a.mapper.RefreshSummaries(a.workspace, 3)
}

// ── Memory (name parity with the desktop app) ───────────────────────────

func (a *RepositoryTaskActions) workspaceID() (string, error) {
	if a.workspace == "" {
		return "", fmt.Errorf("no workspace selected")
	}
	return database.WorkspaceIDFor(a.workspace, projectmap.WorkspaceRemoteIdentity(a.workspace)), nil
}

func (a *RepositoryTaskActions) GetMemoryLessons(status string) ([]models.MemoryLesson, error) {
	wsID, err := a.workspaceID()
	if err != nil {
		return nil, err
	}
	lessons, err := a.lessons.ListByStatus(wsID, status, 200)
	if err != nil {
		return nil, err
	}
	if lessons == nil {
		lessons = []models.MemoryLesson{}
	}
	return lessons, nil
}

func (a *RepositoryTaskActions) ReviewMemoryLesson(lessonID, decision string) error {
	status, err := database.ReviewDecisionStatus(decision)
	if err != nil {
		return err
	}
	return a.lessons.SetStatus(lessonID, status)
}

func (a *RepositoryTaskActions) GetRegressions() ([]models.Regression, error) {
	wsID, err := a.workspaceID()
	if err != nil {
		return nil, err
	}
	regs, err := a.regress.List(wsID, 200)
	if err != nil {
		return nil, err
	}
	if regs == nil {
		regs = []models.Regression{}
	}
	return regs, nil
}

func (a *RepositoryTaskActions) ApproveRegressionGuard(regressionID, checkName string, command []string) error {
	if a.workspace == "" {
		return fmt.Errorf("no workspace selected")
	}
	reg, err := a.regress.Get(regressionID)
	if err != nil {
		return err
	}
	if reg == nil {
		return fmt.Errorf("regression %s not found", regressionID)
	}
	if err := claims.AppendCheck(a.workspace, claims.Check{
		Name:        checkName,
		Description: "Guard cho regression: " + reg.Title,
		Command:     command,
		TimeoutSec:  120,
	}); err != nil {
		return err
	}
	return a.regress.SetGuard(regressionID, checkName, "approved")
}

func (a *RepositoryTaskActions) GetMemoryConfig() models.MemoryConfig {
	return services.LoadMemoryConfig(a.dataDir)
}

func (a *RepositoryTaskActions) SaveMemoryConfig(cfg models.MemoryConfig) error {
	if cfg.ContextPackMaxChars < 0 {
		cfg.ContextPackMaxChars = 0
	}
	if err := services.SaveMemoryConfig(a.dataDir, cfg); err != nil {
		return err
	}
	a.orch.SetPackBudget(cfg.ContextPackMaxChars)
	a.orch.SetSessionResume(cfg.SessionResume)
	a.mapper.SetAutoSummarize(cfg.AutoSummarize)
	a.learner.SetPromotionEnabled(cfg.LessonPromotion)
	return nil
}

func (a *RepositoryTaskActions) GetProjectGraph() (*models.ModuleGraph, error) {
	if a.workspace == "" {
		return nil, fmt.Errorf("no workspace selected")
	}
	return a.mapper.ModuleGraph(a.workspace)
}

func (a *RepositoryTaskActions) GetContextPackPreview(taskID string) (string, error) {
	task, err := a.repo.GetByID(taskID)
	if err != nil {
		return "", err
	}
	if task == nil {
		return "", fmt.Errorf("task %s not found", taskID)
	}
	wsID := ""
	if a.workspace != "" {
		wsID = database.WorkspaceIDFor(a.workspace, projectmap.WorkspaceRemoteIdentity(a.workspace))
	}
	pack := a.context.BuildTaskPack(a.workspace, wsID, task, a.orch.GetPackBudget())
	if pack.Text == "" {
		return "(pack rỗng — task này chưa có context nào để tiêm)", nil
	}
	return pack.Text, nil
}

func (a *RepositoryTaskActions) ReadFileContent(relPath string) (string, error) {
	fullPath := relPath
	if !filepath.IsAbs(relPath) && a.workspace != "" {
		fullPath = filepath.Join(a.workspace, relPath)
	}
	// Shared with the Wails adapter so the two frontends cannot drift apart on
	// what counts as inside the workspace.
	if err := services.EnsureWithinWorkspace(a.workspace, fullPath); err != nil {
		return "", err
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

// Both report the state afterwards rather than nothing, so a caller can tell a
// start that took from one that did not without a second round trip.
func (a *RepositoryTaskActions) StartOrchestrator() bool {
	a.orch.Start()
	return a.orch.IsRunning()
}

func (a *RepositoryTaskActions) StopOrchestrator() bool {
	a.orch.Stop()
	return a.orch.IsRunning()
}
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
func (a *RepositoryTaskActions) ResolveApproval(taskID string, approved bool) {
	a.orch.ResolveApproval(taskID, approved)
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

func (a *RepositoryTaskActions) GetGitStatus() (map[string]interface{}, error) {
	return a.git.GetStatus(a.workspace)
}

func (a *RepositoryTaskActions) GetGitBranches() (*services.GitBranchInfo, error) {
	return a.git.GetBranches(a.workspace)
}

func (a *RepositoryTaskActions) GetGitLog(limit int) ([]services.GitCommitInfo, error) {
	return a.git.GetLog(a.workspace, limit)
}

func (a *RepositoryTaskActions) CreateGitBranch(name string) error {
	return a.git.CreateBranch(a.workspace, name)
}

func (a *RepositoryTaskActions) CheckoutGitBranch(name string) error {
	return a.git.CheckoutBranch(a.workspace, name)
}

func (a *RepositoryTaskActions) CreateGitCommit(message string) error {
	return a.git.CreateCommit(a.workspace, message)
}

func (a *RepositoryTaskActions) RevertGitCommit(hash string) error {
	return a.git.RevertCommit(a.workspace, hash)
}

func (a *RepositoryTaskActions) ToggleWebhook(port int) bool {
	if a.webhook.IsRunning() {
		_ = a.webhook.Stop()
		return false
	}
	if err := a.webhook.Start(port); err != nil {
		a.publish(RuntimeEvent{
			Kind:    "log",
			Level:   "ERROR",
			Message: fmt.Sprintf("Webhook failed to start on port %d: %v", port, err),
		})
		return false
	}
	return true
}

func (a *RepositoryTaskActions) IsWebhookRunning() bool {
	return a.webhook.IsRunning()
}

func (a *RepositoryTaskActions) RunBrowserTask(url string, screenshot bool) (BrowserResult, error) {
	result, err := a.browser.RunBrowserTask(url, "", screenshot, true)
	if result == nil {
		// err is only non-nil here by convention of the browser service; do not
		// dereference it blindly, or a future (nil, nil) return panics the TUI.
		message := "browser task failed"
		if err != nil {
			message = emptyError(err.Error(), message)
		}
		return BrowserResult{Success: false, Error: message}, err
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
	if err := os.WriteFile(path, data, 0600); err != nil {
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
	if a.learner != nil {
		a.learner.Stop()
	}
	return a.db.Close()
}
