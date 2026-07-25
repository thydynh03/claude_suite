package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"claude_suite/backend/cli"
	"claude_suite/backend/database"
	"claude_suite/backend/defaults"
	"claude_suite/backend/models"
	"claude_suite/backend/orchestrator"
	"claude_suite/backend/pipeline"
	"claude_suite/backend/services"
	"claude_suite/backend/textutil"
	"claude_suite/backend/version"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
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
	browserService  *services.BrowserAgentService
	orchestrator    *orchestrator.Orchestrator
	planBuilder     *pipeline.PlanBuilder
	pipelineEngine  *pipeline.PipelineEngine
	oauthListener   *services.OAuthListenerService

	workspaceConfig    models.WorkspaceConfig
	integrationsConfig models.IntegrationsConfig
	uiConfig           models.UIConfig
}

// currentOnboardingVersion bumps when the tour content changes materially, so an
// updated build can show the refreshed tour once again.
const currentOnboardingVersion = "1"

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
	browserSvc := services.NewBrowserAgentService()

	orch := orchestrator.NewOrchestrator(agentRepo, taskRepo, memoryRepo, cliRunner, contextMgr, gitSvc, browserSvc)
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
		browserService:  browserSvc,
		orchestrator:    orch,
		planBuilder:     planBuilder,
		pipelineEngine:  pipelineEng,
		oauthListener:   services.NewOAuthListenerService(),
	}

	app.schedulerSvc.SetTriggerCallback(func(job services.JobRequest) error {
		model := job.Model
		if model == "" {
			// A job that did not pick one follows the provider it did pick, so a
			// Gemini-scheduled run does not quietly execute on Claude.
			model = defaultModelFor(job.Provider)
		}
		result, err := app.RunQuickCLI(job.Prompt, model, "", nil)
		if err != nil {
			return err
		}
		if result != nil && !result.Success {
			return fmt.Errorf("%s", result.Error)
		}
		return nil
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
	// Scheduled jobs live in a file beside the database, so closing the app no
	// longer throws away every schedule the user set up.
	if err := a.schedulerSvc.UseStore(filepath.Join(filepath.Dir(database.GetDBPath()), "scheduled_jobs.json")); err != nil {
		fmt.Printf("Scheduler store warning: %v\n", err)
	}
	a.schedulerSvc.Start()

	a.loadWorkspaceConfig()
	a.loadIntegrationsConfig()
	a.loadUIConfig()
	a.loadAntiKeys()
	if a.workspaceConfig.LastWorkspaceFolder != "" {
		a.orchestrator.SetWorkspaceDir(a.workspaceConfig.LastWorkspaceFolder)
	}

	// Recover tasks left "running" by a previous session that was killed mid-flight.
	if n, err := a.taskRepo.ResetRunningTasks(); err == nil && n > 0 {
		wailsRuntime.EventsEmit(ctx, "log_entry", map[string]string{
			"message": fmt.Sprintf("♻️ Đã khôi phục %d task đang chạy dở về Backlog sau khi khởi động lại.", n),
			"level":   "WARN",
			"time":    time.Now().Format("15:04:05"),
		})
	}

	// Browser Agent asks the user through the live log instead of dying on a
	// blocked step, so it needs a way to push forms onto the event bus.
	a.browserService.SetEventEmitter(func(event string, payload map[string]interface{}) {
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, event, payload)
		}
	})
}

// ── Workspace ──────────────────────────────────────────────────────────

func (a *App) SelectWorkspaceFolder() (string, error) {
	folder, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
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

func (a *App) ReadFileContent(relPath string) (string, error) {
	workspace := a.workspaceConfig.LastWorkspaceFolder
	fullPath := relPath
	if !filepath.IsAbs(relPath) && workspace != "" {
		fullPath = filepath.Join(workspace, relPath)
	}
	// Same containment rule as the TUI adapter: filepath.Join cleans "..", but a
	// symlink inside the workspace can still point outside it. Having the check
	// in one frontend and not the other is worse than not having it at all,
	// because the asymmetry reads as protection that is not there.
	if err := services.EnsureWithinWorkspace(workspace, fullPath); err != nil {
		return "", err
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (a *App) SaveFileContent(relPath string, content string) error {
	fullPath := relPath
	if !filepath.IsAbs(relPath) && a.workspaceConfig.LastWorkspaceFolder != "" {
		fullPath = filepath.Join(a.workspaceConfig.LastWorkspaceFolder, relPath)
	}
	return os.WriteFile(fullPath, []byte(content), 0644)
}

// ── Agents CRUD ────────────────────────────────────────────────────────

func (a *App) GetAgents() ([]models.Agent, error) {
	return a.agentRepo.GetAll()
}

func (a *App) emitAgentsUpdated() {
	if a.ctx != nil {
		wailsRuntime.EventsEmit(a.ctx, "agent_updated", nil)
	}
}

func (a *App) SaveAgent(agent models.Agent) error {
	var err error
	if agent.AgentID == "" {
		err = a.agentRepo.Create(&agent)
	} else {
		err = a.agentRepo.Update(&agent)
	}
	if err == nil {
		a.emitAgentsUpdated()
	}
	return err
}

func (a *App) DeleteAgent(agentID string) error {
	err := a.agentRepo.Delete(agentID)
	if err == nil {
		a.emitAgentsUpdated()
	}
	return err
}

func (a *App) ResetAgentsToDefaults() error {
	err := a.agentRepo.ResetToDefaults()
	if err == nil {
		a.emitAgentsUpdated()
	}
	return err
}

func (a *App) emitBoardUpdated() {
	if a.ctx != nil {
		wailsRuntime.EventsEmit(a.ctx, "board_updated", nil)
	}
}

func (a *App) GetTasks() ([]models.Task, error) {
	return a.taskRepo.GetAll()
}

func (a *App) CreateTask(task models.Task) error {
	err := a.taskRepo.Create(&task)
	if err == nil {
		a.emitBoardUpdated()
	}
	return err
}

func (a *App) UpdateTaskStatus(taskID string, status string) error {
	err := a.taskRepo.UpdateStatus(taskID, status, "", "")
	if err == nil {
		a.emitBoardUpdated()
	}
	return err
}

func (a *App) AssignTask(taskID string, assignedTo string) error {
	err := a.taskRepo.AssignTask(taskID, assignedTo)
	if err == nil {
		a.emitBoardUpdated()
	}
	return err
}

func (a *App) DeleteTask(taskID string) error {
	err := a.taskRepo.Delete(taskID)
	if err == nil {
		a.emitBoardUpdated()
	}
	return err
}

func (a *App) DeleteDoneTasks() error {
	err := a.taskRepo.DeleteDone()
	if err == nil {
		a.emitBoardUpdated()
	}
	return err
}

func (a *App) ClearAllTasks() error {
	err := a.taskRepo.DeleteAll()
	if err == nil {
		a.emitBoardUpdated()
	}
	return err
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

func (a *App) Ping() bool {
	return true
}

func (a *App) ResolveApproval(taskID string, approved bool) {
	a.orchestrator.ResolveApproval(taskID, approved)
}

func (a *App) StopTask(taskID string) bool {
	return a.orchestrator.StopTask(taskID)
}

func (a *App) RetryTask(taskID string) error {
	return a.orchestrator.RetryTask(taskID)
}

func (a *App) SetMaxConcurrency(n int) int {
	a.orchestrator.SetMaxConcurrency(n)
	return a.orchestrator.GetMaxConcurrency()
}

func (a *App) GetMaxConcurrency() int {
	return a.orchestrator.GetMaxConcurrency()
}

func (a *App) SetVerifyBuild(enabled bool) bool {
	a.orchestrator.SetVerifyBuild(enabled)
	return enabled
}

func (a *App) GetVerifyBuild() bool {
	return a.orchestrator.GetVerifyBuild()
}

func (a *App) SetTaskTimeoutMinutes(minutes int) int {
	cli.SetTaskTimeout(time.Duration(minutes) * time.Minute)
	return int(cli.TaskTimeout() / time.Minute)
}

func (a *App) GetTaskTimeoutMinutes() int {
	return int(cli.TaskTimeout() / time.Minute)
}

func (a *App) SetAutoApproveAll(enabled bool) bool {
	a.orchestrator.SetAutoApproveAll(enabled)
	return enabled
}

func (a *App) GetAutoApproveAll() bool {
	return a.orchestrator.GetAutoApproveAll()
}

func (a *App) SetShowCLIConsole(show bool) bool {
	cli.ShowCLIConsole = show
	return cli.ShowCLIConsole
}

func (a *App) GetShowCLIConsole() bool {
	return cli.ShowCLIConsole
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

	var runner cli.CLIRunner
	modelLower := strings.ToLower(model)
	providerKey := "claude"
	if strings.Contains(modelLower, "gemini") || strings.Contains(modelLower, "thinking") {
		runner = cli.NewAntigravityCLI()
		providerKey = "anti"
	} else {
		runner = a.cliRunner
	}

	sessionID := cli.GetGlobalSession(providerKey)
	var result *cli.RunResult
	if sessionID != "" {
		result = runner.RunSession(fullPrompt, model, system, sessionID, nil, a.workspaceConfig.LastWorkspaceFolder)
	} else {
		result = runner.RunOnce(fullPrompt, model, system, nil, a.workspaceConfig.LastWorkspaceFolder)
	}

	if result != nil && result.SessionID != "" {
		cli.SetGlobalSession(providerKey, result.SessionID)
	}
	return result, nil
}

func (a *App) GetActiveSession(provider string) string {
	return cli.GetGlobalSession(provider)
}

func (a *App) ClearActiveSession(provider string) error {
	cli.ClearGlobalSession(provider)
	return nil
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

func (a *App) DecomposePlanWithProvider(requirement string, provider string, model string) ([]models.Task, error) {
	tasks, err := a.planBuilder.DecomposeWithProvider(requirement, a.workspaceConfig.LastWorkspaceFolder, provider, model)
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
	return version.GetVersion()
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
			wailsRuntime.EventsEmit(a.ctx, "update_progress", map[string]int64{
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

// ── Git Version Control ───────────────────────────────────────────────

func (a *App) GetGitStatus() (map[string]interface{}, error) {
	return a.gitService.GetStatus(a.workspaceConfig.LastWorkspaceFolder)
}

func (a *App) GetWorkspaceDiff() (string, error) {
	return a.gitService.GetWorkspaceDiff(a.workspaceConfig.LastWorkspaceFolder)
}

func (a *App) RunGitCommand(args []string) (string, error) {
	return a.gitService.RunCommand(a.workspaceConfig.LastWorkspaceFolder, args)
}

// GenerateCommitMessage asks the AI to write a Conventional Commits message from
// the current workspace diff.
func (a *App) GenerateCommitMessage() (string, error) {
	diff, err := a.gitService.GetWorkspaceDiff(a.workspaceConfig.LastWorkspaceFolder)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(diff) == "" || strings.Contains(diff, "Không có thay đổi") {
		return "", fmt.Errorf("không có thay đổi để tạo commit message")
	}
	diff = textutil.Truncate(diff, 12000, "\n... [diff truncated]")
	system := "You are a senior engineer writing a Conventional Commits message. Reply with ONLY the commit message: a concise type(scope): subject line under 72 chars, optionally a blank line and short bullet body. No code fences, no explanation."
	prompt := "Write a commit message for this diff:\n\n" + diff
	result := a.cliRunner.RunOnce(prompt, "claude-haiku-4-5", system, nil, a.workspaceConfig.LastWorkspaceFolder)
	if result == nil || !result.Success {
		msg := "unknown error"
		if result != nil {
			msg = result.Error
		}
		return "", fmt.Errorf("AI lỗi khi tạo commit message: %s", msg)
	}
	return strings.TrimSpace(result.Output), nil
}

func (a *App) GetGitBranches() (*services.GitBranchInfo, error) {
	return a.gitService.GetBranches(a.workspaceConfig.LastWorkspaceFolder)
}

func (a *App) GetGitLog(limit int) ([]services.GitCommitInfo, error) {
	return a.gitService.GetLog(a.workspaceConfig.LastWorkspaceFolder, limit)
}

func (a *App) CreateGitBranch(name string) error {
	return a.gitService.CreateBranch(a.workspaceConfig.LastWorkspaceFolder, name)
}

func (a *App) CheckoutGitBranch(name string) error {
	return a.gitService.CheckoutBranch(a.workspaceConfig.LastWorkspaceFolder, name)
}

func (a *App) CreateGitCommit(message string) error {
	return a.gitService.CreateCommit(a.workspaceConfig.LastWorkspaceFolder, message)
}

func (a *App) RevertGitCommit(hash string) error {
	return a.gitService.RevertCommit(a.workspaceConfig.LastWorkspaceFolder, hash)
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

// ── Integrations Config (outbound webhook + MCP) ───────────────────────

func (a *App) loadIntegrationsConfig() {
	dbDir := filepath.Dir(database.GetDBPath())
	cfgPath := filepath.Join(dbDir, "integrations_config.json")
	data, err := os.ReadFile(cfgPath)
	if err == nil {
		_ = json.Unmarshal(data, &a.integrationsConfig)
	}
	a.orchestrator.SetNotifierURL(a.integrationsConfig.OutboundWebhookURL)
}

func (a *App) saveIntegrationsConfig() {
	dbDir := filepath.Dir(database.GetDBPath())
	cfgPath := filepath.Join(dbDir, "integrations_config.json")
	data, _ := json.MarshalIndent(a.integrationsConfig, "", "  ")
	_ = os.WriteFile(cfgPath, data, 0644)
}

// ── Anti Keys Config ───────────────────────────────────────────────────

func (a *App) loadAntiKeys() {
	dbDir := filepath.Dir(database.GetDBPath())
	cfgPath := filepath.Join(dbDir, "anti_accounts.json")
	data, err := os.ReadFile(cfgPath)
	if err == nil {
		var keys []cli.AntiAccountKey
		if json.Unmarshal(data, &keys) == nil {
			cli.GlobalAntiPool.SetKeys(keys)
		}
	}
}

func (a *App) saveAntiKeys() {
	dbDir := filepath.Dir(database.GetDBPath())
	cfgPath := filepath.Join(dbDir, "anti_accounts.json")
	data, _ := json.MarshalIndent(cli.GlobalAntiPool.GetKeys(), "", "  ")
	_ = os.WriteFile(cfgPath, data, 0644)
}

// ── First-run Onboarding State ─────────────────────────────────────────

func (a *App) loadUIConfig() {
	dbDir := filepath.Dir(database.GetDBPath())
	cfgPath := filepath.Join(dbDir, "ui_config.json")
	if data, err := os.ReadFile(cfgPath); err == nil {
		_ = json.Unmarshal(data, &a.uiConfig)
	}
}

func (a *App) saveUIConfig() {
	dbDir := filepath.Dir(database.GetDBPath())
	cfgPath := filepath.Join(dbDir, "ui_config.json")
	data, _ := json.MarshalIndent(a.uiConfig, "", "  ")
	_ = os.WriteFile(cfgPath, data, 0644)
}

// ShouldShowOnboarding reports whether the welcome tour should open — true on a
// fresh install, or when the tour content version has changed since it was seen.
func (a *App) ShouldShowOnboarding() bool {
	return !a.uiConfig.OnboardingSeen || a.uiConfig.OnboardingVersion != currentOnboardingVersion
}

// SetOnboardingSeen records that the user finished (or skipped) the tour.
func (a *App) SetOnboardingSeen(seen bool) bool {
	a.uiConfig.OnboardingSeen = seen
	a.uiConfig.OnboardingVersion = currentOnboardingVersion
	a.saveUIConfig()
	return seen
}

func (a *App) GetIntegrationsConfig() models.IntegrationsConfig {
	return a.integrationsConfig
}

func (a *App) SaveIntegrationsConfig(cfg models.IntegrationsConfig) models.IntegrationsConfig {
	a.integrationsConfig = cfg
	a.saveIntegrationsConfig()
	a.orchestrator.SetNotifierURL(cfg.OutboundWebhookURL)
	return a.integrationsConfig
}

func (a *App) addRecentWorkspace(dir string) {
	if dir == "" {
		return
	}
	// Most-recent-first, de-duplicated, capped at 8 entries.
	newRecent := []string{dir}
	for _, prev := range a.workspaceConfig.RecentWorkspaces {
		if prev != dir {
			newRecent = append(newRecent, prev)
		}
	}
	if len(newRecent) > 8 {
		newRecent = newRecent[:8]
	}
	a.workspaceConfig.RecentWorkspaces = newRecent
}

func (a *App) StopBrowserTask() {
	a.browserService.StopBrowserTask()
}

// ResolveBrowserAsk delivers a live-log form answer back to the waiting agent.
func (a *App) ResolveBrowserAsk(id string, answer string) bool {
	return a.browserService.ResolveAsk(id, answer)
}

// OpenAgentChromeWindow opens the agent's profile as a plain browser with no debug
// port, which is the only way Google will let the user sign in — it refuses any
// browser it can tell is driven over CDP. The session written here is reused by
// later agent runs.
func (a *App) OpenAgentChromeWindow(targetURL string) map[string]interface{} {
	logs := []string{}
	if err := services.OpenAgentChromeForLogin(targetURL, func(msg string) {
		logs = append(logs, msg)
	}); err != nil {
		logs = append(logs, fmt.Sprintf("❌ %v", err))
		return map[string]interface{}{"success": false, "logs": logs, "error": err.Error()}
	}
	return map[string]interface{}{
		"success": true,
		"logs":    logs,
		"profile": services.AgentUserDataDir(),
	}
}

// CheckChromeDebugMode reports whether the agent's own Chrome profile is running
// and attachable over CDP.
func (a *App) CheckChromeDebugMode() map[string]interface{} {
	ready, port, profileDir := services.ChromeAgentStatus()
	msg := fmt.Sprintf("⏸️ Chrome Agent chưa chạy. Nó sẽ tự mở khi bạn chạy Agent (profile: %s).", profileDir)
	if ready {
		msg = fmt.Sprintf("✅ Chrome Agent đang chạy trên port %d — các phiên đã đăng nhập trong profile này được dùng lại.", port)
	}
	return map[string]interface{}{"ready": ready, "message": msg, "profile": profileDir, "port": port}
}

func (a *App) RunBrowserTask(targetURL string, prompt string, roleFile string, model string, takeScreenshot bool, headless bool, useRealProfile bool, maxSteps int) (*services.BrowserActionResult, error) {
	systemPrompt := ""
	if roleFile != "" {
		systemPrompt, _ = a.GetRoleContent(roleFile)
	}

	var runner cli.CLIRunner
	modelLower := strings.ToLower(model)
	if strings.Contains(modelLower, "gemini") || strings.Contains(modelLower, "thinking") {
		runner = cli.NewAntigravityCLI()
	} else {
		runner = a.cliRunner
	}

	onLog := func(msg string) {
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, "browser_agent_log", map[string]string{
				"log":  msg,
				"time": time.Now().Format("15:04:05"),
			})
		}
	}

	return a.browserService.RunAutonomousBrowserTask(targetURL, prompt, systemPrompt, model, takeScreenshot, headless, useRealProfile, maxSteps, runner, onLog)
}

// ── Anti CLI Multi-Account / Key Pool ──────────────────────────────────

func (a *App) GetAntiAccountKeys() []cli.AntiAccountKey {
	return cli.GlobalAntiPool.GetKeys()
}

func (a *App) AddAntiAccountKey(name, apiKey string) {
	cli.GlobalAntiPool.AddKey(name, apiKey)
	a.saveAntiKeys()
}

func (a *App) AddAntiOAuthAccountKey(name, oauthToken string) {
	cli.GlobalAntiPool.AddOAuthKey(name, oauthToken)
	a.saveAntiKeys()
}

func (a *App) DeleteAntiAccountKey(id string) bool {
	res := cli.GlobalAntiPool.DeleteKey(id)
	a.saveAntiKeys()
	return res
}

func (a *App) SetCurrentAntiAccountKey(id string) bool {
	res := cli.GlobalAntiPool.SetCurrentKey(id)
	a.saveAntiKeys()
	return res
}

func (a *App) ToggleAntiAccountKeyStatus(id string) string {
	res := cli.GlobalAntiPool.ToggleKeyStatus(id)
	a.saveAntiKeys()
	return res
}

func (a *App) WarmupAntiAccountKeys() {
	cli.GlobalAntiPool.WarmupKeys()
	a.saveAntiKeys()
}

// oauthCreds resolves GCP OAuth credentials from (1) environment variables,
// (2) a local gcp_oauth.json config next to the DB, else empty. Secrets must NOT
// be committed to source — set CLAUDE_SUITE_GCP_CLIENT_ID / _CLIENT_SECRET or
// drop a gcp_oauth.json ({"client_id":"...","client_secret":"..."}) in the data dir.
func oauthCreds() (string, string) {
	id := os.Getenv("CLAUDE_SUITE_GCP_CLIENT_ID")
	secret := os.Getenv("CLAUDE_SUITE_GCP_CLIENT_SECRET")
	if id != "" && secret != "" {
		return id, secret
	}

	cfgPath := filepath.Join(filepath.Dir(database.GetDBPath()), "gcp_oauth.json")
	if data, err := os.ReadFile(cfgPath); err == nil {
		var fileCreds struct {
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
		}
		if json.Unmarshal(data, &fileCreds) == nil {
			if id == "" {
				id = fileCreds.ClientID
			}
			if secret == "" {
				secret = fileCreds.ClientSecret
			}
		}
	}
	return id, secret
}

func (a *App) OpenGoogleOAuthLogin(customClientID string) string {
	defID, clientSecret := oauthCreds()

	clientID := customClientID
	if clientID == "" {
		clientID = defID
	}

	if clientID == "" || clientSecret == "" {
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, "log_entry", map[string]string{
				"message": "⚠️ Thiếu GCP OAuth credentials. Đặt biến môi trường CLAUDE_SUITE_GCP_CLIENT_ID / _CLIENT_SECRET hoặc tạo file gcp_oauth.json trong thư mục dữ liệu.",
				"level":   "ERROR",
				"time":    time.Now().Format("15:04:05"),
			})
		}
		return ""
	}

	if a.oauthListener != nil {
		_ = a.oauthListener.StartOAuthListener(clientID, clientSecret, func(email, token string) {
			if a.ctx != nil {
				wailsRuntime.EventsEmit(a.ctx, "oauth_success", map[string]string{
					"email": email,
					"token": token,
				})
			}
		})
	}

	authURL := "https://accounts.google.com/o/oauth2/v2/auth?client_id=" + clientID + "&redirect_uri=http://localhost:8045/auth/callback&response_type=code&scope=https://www.googleapis.com/auth/cloud-platform%20https://www.googleapis.com/auth/userinfo.email"
	if a.ctx != nil {
		wailsRuntime.BrowserOpenURL(a.ctx, authURL)
	}
	return authURL
}

func (a *App) OpenURLInBrowser(targetURL string) {
	if a.ctx != nil {
		wailsRuntime.BrowserOpenURL(a.ctx, targetURL)
	}
}

// ── Agent Roles (Markdown Repository) ───────────────────────────────────

func (a *App) getRolesDir() string {
	dir := filepath.Join(filepath.Dir(database.GetDBPath()), "roles")
	// Seeded from the binary's own copies, so a downloaded build ships the same
	// role definitions as a checkout. Existing files are left alone, and roles
	// added in a later release appear without wiping the user's edits.
	if _, err := defaults.SeedRoles(dir); err != nil {
		fmt.Printf("Could not seed agent roles in %s: %v\n", dir, err)
	}
	return dir
}

func (a *App) ListRoles() ([]string, error) {
	dir := a.getRolesDir()
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var roles []string
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".md") {
			roles = append(roles, f.Name())
		}
	}
	return roles, nil
}

func (a *App) GetRoleContent(filename string) (string, error) {
	dir := a.getRolesDir()
	path := filepath.Join(dir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (a *App) SaveRoleContent(filename string, content string) error {
	dir := a.getRolesDir()
	if !strings.HasSuffix(filename, ".md") {
		filename += ".md"
	}
	path := filepath.Join(dir, filename)
	return os.WriteFile(path, []byte(content), 0644)
}

func (a *App) DeleteRole(filename string) error {
	dir := a.getRolesDir()
	path := filepath.Join(dir, filename)
	return os.Remove(path)
}

type SystemMetrics struct {
	AllocMemoryMB   uint64 `json:"alloc_memory_mb"`
	SysMemoryMB     uint64 `json:"sys_memory_mb"`
	NumGoroutine    int    `json:"num_goroutine"`
	NumCPU          int    `json:"num_cpu"`
	ActiveKeysCount int    `json:"active_keys_count"`
}

func (a *App) GetSystemMetrics() SystemMetrics {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	keys := cli.GlobalAntiPool.GetKeys()

	return SystemMetrics{
		AllocMemoryMB:   m.Alloc / 1024 / 1024,
		SysMemoryMB:     m.Sys / 1024 / 1024,
		NumGoroutine:    runtime.NumGoroutine(),
		NumCPU:          runtime.NumCPU(),
		ActiveKeysCount: len(keys),
	}
}

func (a *App) GetGCPOAuthCredentials() map[string]string {
	id, secret := oauthCreds()
	return map[string]string{
		"client_id":     id,
		"client_secret": secret,
	}
}

func (a *App) SaveGCPOAuthCredentials(clientID, clientSecret string) error {
	cfgPath := filepath.Join(filepath.Dir(database.GetDBPath()), "gcp_oauth.json")
	fileCreds := map[string]string{
		"client_id":     clientID,
		"client_secret": clientSecret,
	}
	data, err := json.MarshalIndent(fileCreds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, data, 0644)
}

// defaultModelFor picks a model for a scheduled job that named a provider but no
// model. RunQuickCLI routes on the model name, so returning the wrong family
// here would send a Gemini job to Claude.
func defaultModelFor(provider string) string {
	if provider == "anti" || provider == "anti_cli" {
		return "gemini-3.6-flash-high"
	}
	return "claude-sonnet-4-5"
}

// ScheduleJob schedules a prompt against a specific provider and model.
func (a *App) ScheduleJob(prompt, targetTime string, repeat bool, provider, model string) (string, error) {
	return a.schedulerSvc.ScheduleJob(prompt, targetTime, repeat, provider, model)
}

// SetScheduledJobEnabled pauses or resumes a repeating job.
func (a *App) SetScheduledJobEnabled(id string, enabled bool) bool {
	return a.schedulerSvc.SetJobEnabled(id, enabled)
}
