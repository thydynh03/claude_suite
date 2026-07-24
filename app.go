package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"claude_suite/backend/cli"
	"claude_suite/backend/database"
	"claude_suite/backend/models"
	"claude_suite/backend/orchestrator"
	"claude_suite/backend/pipeline"
	"claude_suite/backend/services"
	"claude_suite/backend/version"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx             context.Context
	agentRepo       *database.AgentRepository
	taskRepo        *database.TaskRepository
	memoryRepo      *database.MemoryRepository
	cliRunner       cli.CLIRunner
	contextMgr      *services.ContextManager
	gitService      *services.GitService
	updaterService  *services.UpdaterService
	webhookService  *services.WebhookService
	exporterService *services.ExporterService
	schedulerSvc    *services.SchedulerService
	orchestrator    *orchestrator.Orchestrator
	planBuilder     *pipeline.PlanBuilder
	pipelineEngine  *pipeline.PipelineEngine

	workspaceConfig models.WorkspaceConfig
}

// NewApp creates a new App application struct
func NewApp() *App {
	db, err := database.InitDB()
	if err != nil {
		fmt.Printf("Error initializing DB: %v\n", err)
	}

	agentRepo := database.NewAgentRepository(db)
	taskRepo := database.NewTaskRepository(db)
	memoryRepo := database.NewMemoryRepository(db)
	cliRunner := cli.NewClaudeCLI()
	contextMgr := services.NewContextManager()
	gitSvc := services.NewGitService()
	updaterSvc := services.NewUpdaterService()
	webhookSvc := services.NewWebhookService(taskRepo)
	exporterSvc := services.NewExporterService()
	schedulerSvc := services.NewSchedulerService()

	orch := orchestrator.NewOrchestrator(agentRepo, taskRepo, memoryRepo, cliRunner, contextMgr, gitSvc)
	planBuilder := pipeline.NewPlanBuilder(cliRunner)
	pipelineEng := pipeline.NewPipelineEngine(agentRepo, cliRunner)

	app := &App{
		agentRepo:       agentRepo,
		taskRepo:        taskRepo,
		memoryRepo:      memoryRepo,
		cliRunner:       cliRunner,
		contextMgr:      contextMgr,
		gitService:      gitSvc,
		updaterService:  updaterSvc,
		webhookService:  webhookSvc,
		exporterService: exporterSvc,
		schedulerSvc:    schedulerSvc,
		orchestrator:    orch,
		planBuilder:     planBuilder,
		pipelineEngine:  pipelineEng,
	}

	app.schedulerSvc.SetTriggerCallback(func(prompt string) {
		app.RunQuickCLI(prompt, "claude-sonnet-4-5", "", nil)
	})

	return app
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.orchestrator.SetContext(ctx)
	a.pipelineEngine.SetContext(ctx)
	a.schedulerSvc.SetContext(ctx)
	a.schedulerSvc.Start()

	a.loadWorkspaceConfig()
	if a.workspaceConfig.LastWorkspaceFolder != "" {
		a.orchestrator.SetWorkspaceDir(a.workspaceConfig.LastWorkspaceFolder)
	}
}

// ── Workspace ──────────────────────────────────────────────────────────

func (a *App) SelectWorkspaceFolder() (string, error) {
	folder, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Chọn thư mục Workspace làm việc",
	})
	if err != nil || folder == "" {
		return "", err
	}

	a.workspaceConfig.LastWorkspaceFolder = folder
	a.addRecentWorkspace(folder)
	a.saveWorkspaceConfig()
	a.orchestrator.SetWorkspaceDir(folder)

	return folder, nil
}

func (a *App) GetWorkspaceConfig() models.WorkspaceConfig {
	return a.workspaceConfig
}

func (a *App) ScanWorkspaceFiles() ([]string, error) {
	if a.workspaceConfig.LastWorkspaceFolder == "" {
		return []string{}, nil
	}
	return a.contextMgr.ScanWorkspace(a.workspaceConfig.LastWorkspaceFolder)
}

// ── Agents CRUD ────────────────────────────────────────────────────────

func (a *App) GetAgents() ([]models.Agent, error) {
	return a.agentRepo.GetAll()
}

func (a *App) SaveAgent(agent models.Agent) error {
	if agent.AgentID == "" {
		return a.agentRepo.Create(&agent)
	}
	return a.agentRepo.Update(&agent)
}

func (a *App) DeleteAgent(agentID string) error {
	return a.agentRepo.Delete(agentID)
}

func (a *App) ResetAgentsToDefaults() error {
	return a.agentRepo.ResetToDefaults()
}

// ── Tasks CRUD ─────────────────────────────────────────────────────────

func (a *App) GetTasks() ([]models.Task, error) {
	return a.taskRepo.GetAll()
}

func (a *App) CreateTask(task models.Task) error {
	return a.taskRepo.Create(&task)
}

func (a *App) UpdateTaskStatus(taskID string, status string) error {
	return a.taskRepo.UpdateStatus(taskID, status, "", "")
}

func (a *App) DeleteTask(taskID string) error {
	return a.taskRepo.Delete(taskID)
}

func (a *App) DeleteDoneTasks() error {
	return a.taskRepo.DeleteDone()
}

func (a *App) ClearAllTasks() error {
	return a.taskRepo.DeleteAll()
}

// ── Orchestrator & Execution ───────────────────────────────────────────

func (a *App) StartOrchestrator() bool {
	a.orchestrator.Start()
	return true
}

func (a *App) StopOrchestrator() bool {
	a.orchestrator.Stop()
	return true
}

func (a *App) IsOrchestratorRunning() bool {
	return a.orchestrator.IsRunning()
}

func (a *App) ResolveApproval(approved bool) {
	a.orchestrator.ResolveApproval(approved)
}

func (a *App) RunQuickCLI(prompt string, model string, system string, localFiles []string) (*cli.RunResult, error) {
	ctxPrompt := ""
	if len(localFiles) > 0 || a.workspaceConfig.LastWorkspaceFolder != "" {
		ctxPrompt = a.contextMgr.BuildContextPrompt(a.workspaceConfig.LastWorkspaceFolder, localFiles)
	}

	fullPrompt := prompt
	if ctxPrompt != "" {
		fullPrompt = fmt.Sprintf("%s\n\n%s", ctxPrompt, prompt)
	}

	result := a.cliRunner.RunOnce(fullPrompt, model, system, nil, a.workspaceConfig.LastWorkspaceFolder)
	return result, nil
}

// ── Plan Decomposer & Pipeline ─────────────────────────────────────────

func (a *App) DecomposePlan(requirement string) ([]models.Task, error) {
	tasks, err := a.planBuilder.Decompose(requirement, a.workspaceConfig.LastWorkspaceFolder)
	if err != nil {
		return nil, err
	}

	for i := range tasks {
		_ = a.taskRepo.Create(&tasks[i])
	}

	return a.taskRepo.GetAll()
}

func (a *App) GetPipelineSteps() []models.PipelineStep {
	return a.pipelineEngine.GetDefaultSteps()
}

func (a *App) RunPipeline(steps []models.PipelineStep) ([]models.PipelineStep, error) {
	return a.pipelineEngine.RunPipeline(steps, a.workspaceConfig.LastWorkspaceFolder)
}

// ── Webhook & Services ─────────────────────────────────────────────────

func (a *App) ToggleWebhook(port int) bool {
	if a.webhookService.IsRunning() {
		_ = a.webhookService.Stop()
		return false
	}
	_ = a.webhookService.Start(port)
	return true
}

func (a *App) IsWebhookRunning() bool {
	return a.webhookService.IsRunning()
}

func (a *App) GetAppVersion() string {
	return version.CurrentVersion
}

func (a *App) CheckForUpdates() (*services.UpdateInfo, error) {
	return a.updaterService.CheckForUpdates()
}

type UpdateResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

func (a *App) DownloadAndUpdate(downloadUrl string) UpdateResponse {
	err := a.updaterService.DownloadAndInstall(downloadUrl, nil)
	if err != nil {
		return UpdateResponse{Success: false, Error: err.Error()}
	}
	return UpdateResponse{Success: true}
}

func (a *App) DownloadAndInstallUpdate(url string) error {
	return a.updaterService.DownloadAndInstall(url, func(downloaded, total int64) {
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "update_progress", map[string]int64{
				"downloaded": downloaded,
				"total":      total,
			})
		}
	})
}

func (a *App) PerformAutoUpdate() (*cli.RunResult, error) {
	info, err := a.updaterService.CheckForUpdates()
	if err != nil {
		return &cli.RunResult{Success: false, Error: err.Error()}, nil
	}
	if !info.HasUpdate {
		return &cli.RunResult{Success: true, Output: "Already up to date."}, nil
	}
	err = a.DownloadAndInstallUpdate(info.DownloadURL)
	if err != nil {
		return &cli.RunResult{Success: false, Error: err.Error()}, nil
	}
	return &cli.RunResult{Success: true, Output: "Updated successfully."}, nil
}

func (a *App) ExportKanbanReport() (string, error) {
	tasks, err := a.taskRepo.GetAll()
	if err != nil {
		return "", err
	}
	md, _, err := a.exporterService.ExportKanbanReport(tasks, a.workspaceConfig.LastWorkspaceFolder)
	return md, err
}

// ── Scheduler ──────────────────────────────────────────────────────────

func (a *App) SchedulePrompt(prompt string, targetTimeStr string, repeat bool) (string, error) {
	return a.schedulerSvc.SchedulePrompt(prompt, targetTimeStr, repeat)
}

func (a *App) CancelScheduledJob(id string) {
	a.schedulerSvc.CancelJob(id)
}

func (a *App) GetScheduledJobs() []services.ScheduledJob {
	return a.schedulerSvc.GetJobs()
}

// ── Config Persistence Helpers ─────────────────────────────────────────

func (a *App) loadWorkspaceConfig() {
	dbDir := filepath.Dir(database.GetDBPath())
	cfgPath := filepath.Join(dbDir, "workspace_config.json")
	data, err := os.ReadFile(cfgPath)
	if err == nil {
		_ = json.Unmarshal(data, &a.workspaceConfig)
	}
}

func (a *App) saveWorkspaceConfig() {
	dbDir := filepath.Dir(database.GetDBPath())
	cfgPath := filepath.Join(dbDir, "workspace_config.json")
	data, _ := json.MarshalIndent(a.workspaceConfig, "", "  ")
	_ = os.WriteFile(cfgPath, data, 0644)
}

func (a *App) addRecentWorkspace(dir string) {
	var newRecent []string
	newRecent = append(newRecent, dir)

	for _, r := range a.workspaceConfig.RecentWorkspaces {
		if r != dir && len(newRecent) < 5 {
			newRecent = append(newRecent, r)
		}
	}
	a.workspaceConfig.RecentWorkspaces = newRecent
}
