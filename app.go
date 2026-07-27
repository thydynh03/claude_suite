package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"agent_center/backend/sysproc"

	"agent_center/backend/claims"
	"agent_center/backend/cli"
	"agent_center/backend/core"
	"agent_center/backend/database"
	"agent_center/backend/defaults"
	"agent_center/backend/logger"
	"agent_center/backend/modelcatalog"
	"agent_center/backend/models"
	"agent_center/backend/orchestrator"
	"agent_center/backend/paths"
	"agent_center/backend/pipeline"
	"agent_center/backend/secrets"
	"agent_center/backend/services"
	"agent_center/backend/services/projectmap"
	"agent_center/backend/textutil"
	"agent_center/backend/version"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"agent_center/backend/provider"
)

// App struct
type App struct {
	ctx             context.Context
	agentRepo       *database.AgentRepository
	taskRepo        *database.TaskRepository
	memoryRepo      *database.MemoryRepository
	attemptRepo     *database.AttemptRepository
	lessonRepo      *database.LessonRepository
	regressionRepo  *database.RegressionRepository
	learner         *services.Learner
	modelCatalog    *modelcatalog.Catalog
	cliRunner       cli.CLIRunner
	contextMgr      *services.ContextManager
	gitService      *services.GitService
	updaterService  *services.UpdaterService
	webhookService  *services.WebhookService
	exporterService *services.ExporterService
	schedulerSvc    *services.SchedulerService
	browserService  *services.BrowserAgentService
	orchestrator    *orchestrator.Orchestrator
	projectMapper   *projectmap.Mapper
	planBuilder     *pipeline.PlanBuilder
	pipelineEngine  *pipeline.PipelineEngine
	oauthListener   *services.OAuthListenerService
	claimsHost      *services.ClaimsHostService
	mcpStore        *services.MCPStore
	gitWatch        *services.GitWatchService

	tunnelCmd    *exec.Cmd
	tunnelCancel context.CancelFunc
	tunnelMu     sync.Mutex

	workspaceConfig    models.WorkspaceConfig
	integrationsConfig models.IntegrationsConfig
	uiConfig           models.UIConfig
	memoryConfig       models.MemoryConfig

	// dbInitErr is why the database could not be opened, if it could not.
	// Every repository is holding a nil handle in that case, so startup must
	// say so and stop rather than panic on the first query.
	dbInitErr error

	// Startup speaks before the frontend can listen; these hold what it said
	// until domReady.
	noticeMu       sync.Mutex
	domIsReady     bool
	pendingNotices []map[string]string

	// antiKeysLoadFailed blocks saves that would overwrite an account file the
	// app could not read: an empty pool is not a newer version of it.
	antiKeysLoadFailed bool
}

// The desktop app must offer every shared capability. Adding a method to
// core.Facade breaks this line until it is implemented here too, which is what
// stops a capability landing in one frontend and quietly missing from the other.
var _ core.Facade = (*App)(nil)

// currentOnboardingVersion bumps when the tour content changes materially, so an
// updated build can show the refreshed tour once again.
const currentOnboardingVersion = "1"

// memoryCheapModel runs the background memory work (summaries, distillation) —
// the same cheap model the commit-message generator uses.
const memoryCheapModel = "claude-haiku-4-5"

// NewApp creates a new App application struct
func NewApp() *App {
	db, err := database.InitDB()
	if err != nil {
		// Printing to stdout is invisible in a -H windowsgui build. Every
		// repository below is then built on a nil handle and the first query in
		// startup panics — the whole app died with no window and no message
		// anywhere. The error is kept and startup turns it into a dialog.
		logger.Error(fmt.Sprintf("InitDB failed: %v", err))
	}

	agentRepo := database.NewAgentRepository(db)
	taskRepo := database.NewTaskRepository(db)
	memoryRepo := database.NewMemoryRepository(db)
	attemptRepo := database.NewAttemptRepository(db)
	cliRunner := cli.NewClaudeCLI()
	contextMgr := services.NewContextManager()
	gitSvc := services.NewGitService()
	updaterSvc := services.NewUpdaterService()
	webhookSvc := services.NewWebhookService(taskRepo)
	exporterSvc := services.NewExporterService()
	schedulerSvc := services.NewSchedulerService()
	browserSvc := services.NewBrowserAgentService()
	claimsHostSvc := services.NewClaimsHostService()

	wsRepo := database.NewWorkspaceRepository(db)
	obsRepo := database.NewObservationRepository(db)
	graphRepo := database.NewCodeGraphRepository(db)
	lessonRepo := database.NewLessonRepository(db)
	regressionRepo := database.NewRegressionRepository(db)
	mapper := projectmap.NewMapper(graphRepo, wsRepo)
	mapper.SetSummarizer(cliRunner, memoryCheapModel)
	mapper.SetAutoSummarize(true)

	contextMgr.AttachRepos(taskRepo, attemptRepo)
	contextMgr.AttachProjectMapper(mapper)
	contextMgr.AttachMemoryRepos(lessonRepo, regressionRepo)

	learner := services.NewLearner(lessonRepo, obsRepo, wsRepo, attemptRepo, regressionRepo, gitSvc, cliRunner, memoryCheapModel)

	catalog := modelcatalog.New(filepath.Dir(database.GetDBPath()))

	orch := orchestrator.NewOrchestrator(agentRepo, taskRepo, memoryRepo, cliRunner, contextMgr, gitSvc, browserSvc)
	orch.SetAttemptRepo(attemptRepo)
	orch.SetMemoryStores(wsRepo, obsRepo)
	orch.SetProjectMapper(mapper)
	orch.SetLearner(learner)
	orch.SetModelCatalog(catalog)
	planBuilder := pipeline.NewPlanBuilder(cliRunner)
	planBuilder.AttachBoard(taskRepo)
	planBuilder.AttachRegressions(regressionRepo)
	pipelineEng := pipeline.NewPipelineEngine(agentRepo, cliRunner)

	app := &App{
		agentRepo:       agentRepo,
		taskRepo:        taskRepo,
		memoryRepo:      memoryRepo,
		attemptRepo:     attemptRepo,
		lessonRepo:      lessonRepo,
		regressionRepo:  regressionRepo,
		learner:         learner,
		modelCatalog:    catalog,
		cliRunner:       cliRunner,
		contextMgr:      contextMgr,
		gitService:      gitSvc,
		updaterService:  updaterSvc,
		webhookService:  webhookSvc,
		exporterService: exporterSvc,
		schedulerSvc:    schedulerSvc,
		browserService:  browserSvc,
		orchestrator:    orch,
		projectMapper:   mapper,
		planBuilder:     planBuilder,
		pipelineEngine:  pipelineEng,
		oauthListener:   services.NewOAuthListenerService(),
		claimsHost:      claimsHostSvc,
		dbInitErr:       err,
	}

	app.schedulerSvc.SetTriggerCallback(app.runScheduledJob)

	// The runner asks for a live access token right before it spends one.
	cli.EnsureFreshToken = app.refreshOneAccountToken

	// WaitForQuota jobs hold until the anti pool has a usable account; the
	// pool is the only party that measured anything, so it answers. Claude
	// runs are never held — this app has no quota data about them.
	app.schedulerSvc.SetQuotaGate(func(jobProvider string) (bool, string) {
		if !provider.IsAnti(provider.ResolveProvider(jobProvider, "")) {
			return true, ""
		}
		return cli.GlobalAntiPool.QuotaReady(cli.QuotaCooldown)
	})

	return app
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Without a database there is nothing to start: every repository holds a
	// nil handle, and the first query below would panic the process behind a
	// window that never appeared. Say what happened, in a dialog the user can
	// actually read, and stop.
	if a.dbInitErr != nil {
		msg := fmt.Sprintf(
			"Không mở được cơ sở dữ liệu:\n\n%v\n\nThư mục dữ liệu: %s\n\n"+
				"Thường do thư mục không tạo được, ổ đĩa đầy, hoặc file .db bị phần mềm diệt virus khoá.",
			a.dbInitErr, paths.DataDir(),
		)
		logger.Error("startup aborted: " + msg)
		_, _ = wailsRuntime.MessageDialog(ctx, wailsRuntime.MessageDialogOptions{
			Type:    wailsRuntime.ErrorDialog,
			Title:   "Agent Center không khởi động được",
			Message: msg,
		})
		wailsRuntime.Quit(ctx)
		return
	}

	a.orchestrator.SetContext(ctx)
	a.pipelineEngine.SetContext(ctx)
	a.schedulerSvc.SetContext(ctx)
	// Scheduled jobs live in a file beside the database, so closing the app no
	// longer throws away every schedule the user set up.
	if err := a.schedulerSvc.UseStore(filepath.Join(filepath.Dir(database.GetDBPath()), "scheduled_jobs.json")); err != nil {
		fmt.Printf("Scheduler store warning: %v\n", err)
	}
	a.schedulerSvc.Start()

	// Git watch: AutoSnapshot guards the diff during each task, this guards
	// what comes after — a night of agent work sitting as one loose diff.
	a.gitWatch = services.NewGitWatchService(filepath.Dir(database.GetDBPath()))
	a.gitWatch.SetHooks(
		func() (bool, error) {
			// A workspace with agents mid-task is BUSY, not abandoned: the
			// dirty clock must not accrue through an overnight plan run, or
			// auto-commit writes a half-finished agent edit into history and
			// breaks the AutoSnapshot invariant every diff reader relies on
			// (the uncommitted diff IS the current task's edits).
			if a.runningTaskCount() > 0 {
				return false, nil
			}
			ws := a.workspaceConfig.LastWorkspaceFolder
			if ws == "" {
				return false, nil
			}
			st, err := a.gitService.GetStatus(ws)
			if err != nil {
				return false, err
			}
			isRepo, _ := st["is_repo"].(bool)
			clean, _ := st["clean"].(bool)
			return isRepo && !clean, nil
		},
		func(dirtyFor time.Duration) {
			// dirtyFor == 0 is the watcher saying it cannot inspect the
			// workspace at all (no git, folder gone). Saying so beats going
			// quietly inert, which is indistinguishable from "nothing to do".
			if dirtyFor <= 0 {
				a.emitLog("⚠️ Canh chừng Git không đọc được workspace (chưa cài git, hoặc thư mục đã đổi) — tính năng này đang tạm ngưng.", "WARN")
				return
			}
			a.emitLog(fmt.Sprintf("⏰ Workspace đã bẩn %.1f giờ chưa commit — vào Git panel hoặc bật tự commit trong Lịch tự động.", dirtyFor.Hours()), "WARN")
		},
		func() error {
			if err := a.runScheduledCommit(); err != nil {
				return err
			}
			a.emitLog("✅ Git watch: đã tự commit phần thay đổi để lâu với message do AI soạn.", "SUCCESS")
			return nil
		},
	)
	a.gitWatch.Start()

	// Loaded here rather than in the constructor: the data directory is only
	// settled once the runtime has started.
	a.mcpStore = services.NewMCPStore(filepath.Dir(database.GetDBPath()))

	// The day's spending is kept on disk too: a cap a restart clears is not a cap.
	if err := a.orchestrator.UseBudgetStore(filepath.Join(filepath.Dir(database.GetDBPath()), "budget.json")); err != nil {
		fmt.Printf("Budget store warning: %v\n", err)
	}

	// Seed role definitions now, not when the Roles page is first opened: the
	// orchestrator reads this directory on every task run and silently skips
	// missing files, so a fresh install that never visited the Roles page ran
	// every agent with only the dispatcher's one-line system string.
	_ = a.getRolesDir()

	// The learner runs for the app's lifetime, deliberately not tied to the
	// orchestrator's Start/Stop: Stop lets running tasks finish, and their
	// final observations still deserve a distillation pass.
	a.learner.SetLogger(func(msg, level string) {
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, "log_entry", map[string]string{
				"message": msg, "level": level, "time": time.Now().Format("15:04:05"),
			})
		}
	})
	a.learner.Start()

	// Memory subsystem knobs are user-configurable (Settings → Memory).
	a.memoryConfig = services.LoadMemoryConfig(filepath.Dir(database.GetDBPath()))
	a.applyMemoryConfig()

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

	// Adjudication progress goes to the same log stream as everything else, so a
	// session running on this machine is visible without a separate console.
	a.claimsHost.SetEventHandler(func(sessionID, message string) {
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, "claims_event", map[string]string{
				"session_id": sessionID,
				"message":    message,
				"time":       time.Now().Format("15:04:05"),
			})
		}
	})

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
	if workspace == "" {
		// With no workspace the guard below permits everything and a relative
		// path resolves against the process's working directory — Downloads,
		// for a portable copy. Refusing names the actual problem.
		return "", fmt.Errorf("chưa chọn workspace — hãy chọn thư mục dự án trước khi mở file")
	}
	fullPath := relPath
	if !filepath.IsAbs(relPath) {
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
	workspace := a.workspaceConfig.LastWorkspaceFolder
	if workspace == "" {
		// Saving into the exe's directory is how a portable copy ends up
		// writing stray files into Downloads — or failing with a bare
		// "Access is denied" from Program Files that never mentions why.
		return fmt.Errorf("chưa chọn workspace — hãy chọn thư mục dự án trước khi lưu file")
	}
	fullPath := relPath
	if !filepath.IsAbs(relPath) {
		fullPath = filepath.Join(workspace, relPath)
	}
	// Writing needs this more than reading did. ReadFileContent has been
	// containment-checked for a while; this one was not, so a symlink inside the
	// workspace was enough to overwrite any file the user could write to.
	if err := services.EnsureWithinWorkspace(workspace, fullPath); err != nil {
		return err
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

// StartOrchestrator reports whether this call started it. Returning true
// unconditionally made a second press look like a fresh start.
func (a *App) StartOrchestrator() bool {
	return a.orchestrator.Start()
}

func (a *App) StopOrchestrator() bool {
	return a.orchestrator.Stop()
}

// GetDispatchReadiness tells the board why nothing is running: no work, work
// waiting on unfinished tasks, or work waiting on tasks that failed and will
// never finish. Pressing Execute Plan used to look identical in all three cases.
func (a *App) GetDispatchReadiness() (database.DispatchReadiness, error) {
	return a.taskRepo.DispatchReadiness()
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
	// Context only when the user actually attached files. Building it on a
	// bare workspace produced a header-only block that framed the whole
	// message as context — the model then reported "no question or task in
	// your message" to a perfectly clear question.
	ctxPrompt := ""
	if len(localFiles) > 0 {
		ctxPrompt = a.contextMgr.BuildContextPrompt(a.workspaceConfig.LastWorkspaceFolder, localFiles)
	}

	fullPrompt := prompt
	if ctxPrompt != "" {
		// The label separates data from task, so the question cannot read as
		// one more line of context.
		fullPrompt = fmt.Sprintf("%s\n--- YÊU CẦU CỦA NGƯỜI DÙNG (trả lời phần này) ---\n%s", ctxPrompt, prompt)
	}

	var runner cli.CLIRunner
	providerKey := provider.ResolveProvider("", model)
	if provider.IsAnti(providerKey) {
		runner = cli.NewAntigravityCLI()
	} else {
		runner = a.cliRunner
	}

	// Stream the run instead of discarding it. onLog was nil here, so a prompt
	// that takes three minutes produced nothing at all until it finished and then
	// dumped the whole answer at once — indistinguishable from a button that does
	// nothing. The runners already surface tool calls through this callback; the
	// orchestrator has used it all along.
	onLog := func(message, level string) {
		a.emitLog(message, level)
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, "quick_cli_log", map[string]string{
				"message": message,
				"level":   level,
				"time":    time.Now().Format("15:04:05"),
			})
		}
	}

	sessionID := cli.GetGlobalSession(providerKey)
	var result *cli.RunResult
	if sessionID != "" {
		result = runner.RunSession(fullPrompt, model, system, sessionID, onLog, a.workspaceConfig.LastWorkspaceFolder)
	} else {
		result = runner.RunOnce(fullPrompt, model, system, onLog, a.workspaceConfig.LastWorkspaceFolder)
	}

	if result != nil && result.SessionID != "" {
		cli.SetGlobalSession(providerKey, result.SessionID)
	}
	// Surface (and learn) the model the CLI actually ran — the answer to
	// "agent này đang dùng model nào" as a fact, not a configured wish.
	if result != nil && result.ModelUsed != "" {
		onLog("🤖 Model thực tế: "+result.ModelUsed, "INFO")
		a.modelCatalog.RecordObserved(result.ModelUsed)
	}
	return result, nil
}

// ── Model catalog ───────────────────────────────────────────────────────

// GetAvailableModels returns the merged model catalog: aliases (always-newest),
// builtin, observed-in-runs, and API-discovered Gemini models.
func (a *App) GetAvailableModels() []modelcatalog.Model {
	return a.modelCatalog.List()
}

// RefreshGeminiModels fetches the live Gemini model list using the key pool's
// API keys. Returns how many models the API reported.
func (a *App) RefreshGeminiModels() (int, error) {
	var keys []string
	for _, k := range cli.GlobalAntiPool.GetKeys() {
		if k.APIKey != "" {
			keys = append(keys, k.APIKey)
		}
	}
	return a.modelCatalog.RefreshGemini(keys)
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
	return a.persistDecomposedPlan(tasks)
}

func (a *App) DecomposePlanWithProvider(requirement string, provider string, model string) ([]models.Task, error) {
	tasks, err := a.planBuilder.DecomposeWithProvider(requirement, a.workspaceConfig.LastWorkspaceFolder, provider, model)
	if err != nil {
		return nil, err
	}
	return a.persistDecomposedPlan(tasks)
}

// persistDecomposedPlan writes a decomposed plan to the board. Create errors
// used to be discarded: a SQLITE_BUSY during the loop dropped tasks from the
// plan while the UI announced it queued, and with no board_updated emitted
// the board did not even refresh to show what little had landed.
func (a *App) persistDecomposedPlan(tasks []models.Task) ([]models.Task, error) {
	for i := range tasks {
		if err := a.taskRepo.Create(&tasks[i]); err != nil {
			a.emitBoardUpdated()
			return nil, fmt.Errorf("lưu task %q thất bại: %w", tasks[i].Title, err)
		}
	}
	a.emitBoardUpdated()
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
	// The service binds synchronously precisely so a taken port surfaces
	// here; discarding the error re-created the "UI shows listening while
	// nothing is bound" bug the service layer fixed — and drifted from the
	// TUI, which reports the failure.
	if err := a.webhookService.Start(port); err != nil {
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, "log_entry", map[string]string{
				"message": "❌ Webhook không mở được: " + err.Error(),
				"level":   "ERROR",
				"time":    time.Now().Format("15:04:05"),
			})
		}
		return false
	}
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

// updateBlockedByRunningTasks refuses to let the updater exit the process
// while sub-agents are mid-task. The agents are separate processes that
// survive os.Exit as orphans, still editing the workspace; on relaunch
// ResetRunningTasks would then dispatch a second agent onto the same
// workspace beside the orphan.
// runningTaskCount reports how many tasks are mid-run right now — the shared
// "are agents working?" answer for anything that must not act over their
// heads (the updater's exit, the git watch's commit).
func (a *App) runningTaskCount() int {
	tasks, err := a.taskRepo.GetAll()
	if err != nil {
		return 0
	}
	running := 0
	for _, t := range tasks {
		if t.Status == "running" {
			running++
		}
	}
	return running
}

func (a *App) updateBlockedByRunningTasks() string {
	if running := a.runningTaskCount(); running > 0 {
		return fmt.Sprintf("Đang có %d task chạy — cập nhật sẽ đóng app và bỏ rơi agent giữa chừng. Dừng hoặc chờ các task xong rồi cập nhật.", running)
	}
	return ""
}

func (a *App) DownloadAndUpdate(downloadUrl string) UpdateResponse {
	if msg := a.updateBlockedByRunningTasks(); msg != "" {
		return UpdateResponse{Success: false, Error: msg}
	}
	err := a.updaterService.DownloadAndInstall(downloadUrl, nil)
	if err != nil {
		return UpdateResponse{Success: false, Error: err.Error()}
	}
	return UpdateResponse{Success: true}
}

func (a *App) DownloadAndInstallUpdate(url string) error {
	if msg := a.updateBlockedByRunningTasks(); msg != "" {
		return fmt.Errorf("%s", msg)
	}
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
	if strings.TrimSpace(diff) == "" {
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
		// A parse failure here means the file exists but is damaged; the app
		// then starts with no workspace, which reads as "my settings vanished".
		if err := json.Unmarshal(data, &a.workspaceConfig); err != nil {
			logger.Error(fmt.Sprintf("workspace_config.json hỏng: %v", err))
			a.emitLog("⚠️ workspace_config.json không đọc được — app khởi động không có workspace, hãy chọn lại thư mục dự án.", "WARN")
		}
	}
}

func (a *App) saveWorkspaceConfig() {
	dbDir := filepath.Dir(database.GetDBPath())
	cfgPath := filepath.Join(dbDir, "workspace_config.json")
	data, _ := json.MarshalIndent(a.workspaceConfig, "", "  ")
	a.saveConfigFile(cfgPath, data, "workspace")
}

// saveConfigFile writes a config atomically and says so when it cannot.
//
// These writes were `_ = os.WriteFile`: picking a workspace reported success,
// and the choice was simply gone on the next launch when the write had failed.
// Non-atomic also meant a crash mid-write left truncated JSON that the loader
// silently read as "no config" — the same silence, one launch later.
func (a *App) saveConfigFile(path string, data []byte, what string) {
	if err := services.WriteFileAtomic(path, data, 0o600); err != nil {
		logger.Error(fmt.Sprintf("save %s config: %v", what, err))
		a.emitLog(fmt.Sprintf("⚠️ Không lưu được cấu hình %s (%v) — thay đổi này sẽ mất khi khởi động lại.", what, err), "ERROR")
	}
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
	a.saveConfigFile(cfgPath, data, "tích hợp")
}

// ── Anti Keys Config ───────────────────────────────────────────────────

func (a *App) loadAntiKeys() {
	dbDir := filepath.Dir(database.GetDBPath())
	cfgPath := filepath.Join(dbDir, "anti_accounts.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return
	}
	plaintext, wasPlain, err := secrets.Open(data)
	if err != nil {
		// A sealed file this user cannot open — copied from another machine
		// or account. Refusing beats silently starting with an empty pool
		// and overwriting the file on the next save: this is the branch that
		// case actually takes, so it is the one that most needs the guard.
		a.antiKeysLoadFailed = true
		a.emitLog("❌ Không giải mã được anti_accounts.json (file thuộc máy/tài khoản khác?): "+err.Error(), "ERROR")
		return
	}
	var keys []cli.AntiAccountKey
	if err := json.Unmarshal(plaintext, &keys); err != nil {
		// Damaged JSON used to be swallowed entirely: the pool came up empty
		// with no explanation, and the first account the user added atomically
		// overwrote the still-recoverable file, taking every refresh token with
		// it. Say so, and leave the file alone.
		a.antiKeysLoadFailed = true
		a.emitLog("❌ anti_accounts.json không đọc được ("+err.Error()+") — danh sách tài khoản đang trống và file được giữ nguyên để bạn khôi phục.", "ERROR")
		return
	}
	cli.GlobalAntiPool.SetKeys(keys)
	if wasPlain && len(keys) > 0 {
		// A legacy plain-text install: reseal it now, so the refresh
		// tokens stop sitting on disk in the clear.
		a.saveAntiKeys()
	}
}

func (a *App) saveAntiKeys() {
	dbDir := filepath.Dir(database.GetDBPath())
	cfgPath := filepath.Join(dbDir, "anti_accounts.json")

	// The file could not be read this session, so what is in memory is not a
	// newer version of it — it is nothing. Rather than refuse every save for
	// the rest of the session (which silently blocked adding accounts, signing
	// in, every toggle), the unreadable file is moved aside once and named, so
	// nothing is destroyed and the user is not stuck.
	if a.antiKeysLoadFailed {
		backup := fmt.Sprintf("%s.unreadable-%d", cfgPath, time.Now().Unix())
		if err := os.Rename(cfgPath, backup); err != nil && !os.IsNotExist(err) {
			a.emitLog("⚠️ Chưa lưu được tài khoản: file cũ không đọc được và cũng không đổi tên được ("+err.Error()+"). Hãy tự sao lưu rồi xoá "+cfgPath, "ERROR")
			return
		}
		a.antiKeysLoadFailed = false
		a.emitLog("⚠️ File anti_accounts.json cũ không đọc được — đã giữ lại tại "+backup+" và tạo file mới. Không mất dữ liệu cũ.", "WARN")
	}
	data, _ := json.MarshalIndent(cli.GlobalAntiPool.GetKeys(), "", "  ")
	// DPAPI-sealed to this Windows user: the file holds OAuth refresh
	// tokens, and 0600 alone does nothing against anything running as the
	// user, a copied disk, or a synced backup.
	sealed, err := secrets.Seal(data)
	if err != nil {
		// Losing the pool is worse than storing it the way every version
		// until now did — keep the plain write, but say so.
		a.emitLog("⚠️ Không niêm phong được anti_accounts.json, lưu dạng thường: "+err.Error(), "WARN")
		sealed = data
	}
	// 0600 stays: defence in depth, and the non-Windows builds seal as
	// identity.
	if err := services.WriteFileAtomic(cfgPath, sealed, 0o600); err != nil {
		a.emitLog("❌ Không lưu được anti_accounts.json: "+err.Error(), "ERROR")
	}
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
	a.saveConfigFile(cfgPath, data, "giao diện")
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
	if provider.IsAnti(provider.ResolveProvider("", model)) {
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

	// A frame per step, so the user can watch a headless run instead of trusting
	// the log. Cheap enough at one capture per step; a CDP screencast would be
	// smoother but is a different mechanism with its own failure modes.
	onFrame := func(step int, dataURL string) {
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, "browser_agent_frame", map[string]interface{}{
				"step":  step,
				"image": dataURL,
			})
		}
	}

	return a.browserService.RunAutonomousBrowserTask(targetURL, prompt, systemPrompt, model, takeScreenshot, headless, useRealProfile, maxSteps, runner, onLog, onFrame)
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

// WarmupResult reports what warming the pool actually achieved, per account.
type WarmupResult struct {
	Total     int      `json:"total"`
	Refreshed int      `json:"refreshed"`
	Failed    int      `json:"failed"`
	Skipped   int      `json:"skipped"`
	Errors    []string `json:"errors"`
}

// WarmupAntiAccountKeys refreshes every account's access token and reports the
// outcome.
//
// It used to only clear rate-limit marks in memory and return nothing, so the
// button called "Làm Nóng" neither touched the network nor told anyone what
// happened. An account whose refresh token had been revoked stayed marked
// "active" until the next task failed on it.
// SetAntiAccountTier labels an account FREE/PRO/ULTRA. Tiers are not fetched from
// anywhere, so this is the only way one becomes true; accounts start UNKNOWN
// rather than claiming a tier nobody checked.
// ── MCP servers ────────────────────────────────────────────────────────

// GetMCPCatalogue lists the servers offered without a web search.
func (a *App) GetMCPCatalogue() []services.MCPServer {
	return services.MCPCatalogue()
}

// mcp returns the store, creating it if startup has not run yet. A binding
// called before startup would otherwise dereference a nil pointer and take the
// app down — the data directory is resolvable at any point, so there is no
// reason for the window to exist.
func (a *App) mcp() *services.MCPStore {
	if a.mcpStore == nil {
		a.mcpStore = services.NewMCPStore(filepath.Dir(database.GetDBPath()))
	}
	return a.mcpStore
}

func (a *App) GetMCPServers() []services.MCPServer {
	return a.mcp().List()
}

func (a *App) AddMCPServer(server services.MCPServer) (services.MCPServer, error) {
	return a.mcp().Add(server)
}

func (a *App) RemoveMCPServer(id string) error {
	return a.mcp().Remove(id)
}

func (a *App) SetMCPServerEnabled(id string, enabled bool) error {
	return a.mcp().SetEnabled(id, enabled)
}

// TestMCPServer checks a configured server can be started before a task depends
// on it. A misconfigured server used to fail silently: agents simply ran without
// the tools the user believed they had.
func (a *App) TestMCPServer(id string) (string, error) {
	return a.mcp().TestServer(id)
}

func (a *App) SetAntiAccountTier(id, tier string) bool {
	ok := cli.GlobalAntiPool.SetTier(id, tier)
	if ok {
		a.saveAntiKeys()
	}
	return ok
}

func (a *App) WarmupAntiAccountKeys() WarmupResult {
	// Clearing the rate-limit marks first is still right: an account parked for a
	// 429 an hour ago deserves another try. Accounts the user switched off stay
	// off — that toggle is a decision, not a transient state.
	cli.GlobalAntiPool.WarmupKeys()

	clientID, clientSecret := oauthCreds()
	pending := cli.GlobalAntiPool.AccountsNeedingRefresh()

	result := WarmupResult{Total: len(cli.GlobalAntiPool.GetKeys())}
	result.Skipped = result.Total - len(pending)

	if len(pending) > 0 && (clientID == "" || clientSecret == "") {
		result.Failed = len(pending)
		result.Errors = append(result.Errors,
			"Chưa có GCP OAuth Credentials nên không làm mới được token nào. Đặt Client ID/Secret ở phần cấu hình phía trên.")
		a.saveAntiKeys()
		return result
	}

	for id, refreshToken := range pending {
		token, err := services.RefreshGoogleAccessToken(context.Background(), clientID, clientSecret, refreshToken)
		if err != nil {
			result.Failed++
			// Same rule as RefreshAccountTokens: a network failure must not
			// disable an account Google never even judged.
			if errors.Is(err, services.ErrGoogleRefused) {
				cli.GlobalAntiPool.DisableKey(id, err.Error())
				a.emitLog(fmt.Sprintf("⚠️ Tài khoản %s bị Google từ chối, đã tắt: %v", accountLabel(id), err), "WARN")
			} else {
				a.emitLog(fmt.Sprintf("⚠️ Tài khoản %s chưa làm mới được token (lỗi mạng?), sẽ thử lại: %v", accountLabel(id), err), "WARN")
			}
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", accountLabel(id), err))
			continue
		}
		cli.GlobalAntiPool.SetAccessToken(id, token.AccessToken, token.ExpiresAt)
		result.Refreshed++
	}

	a.saveAntiKeys()
	return result
}

// accountLabel prefers the email over the internal id: an error naming
// "k-3f2a" tells the user nothing they can act on.
func accountLabel(id string) string {
	for _, k := range cli.GlobalAntiPool.GetKeys() {
		if k.ID == id {
			if k.Email != "" {
				return k.Email
			}
			if k.Name != "" {
				return k.Name
			}
			break
		}
	}
	return id
}

// oauthCreds resolves GCP OAuth credentials from (1) environment variables,
// (2) a local gcp_oauth.json config next to the DB, else empty. Secrets must NOT
// be committed to source — set agent_center_GCP_CLIENT_ID / _CLIENT_SECRET or
// drop a gcp_oauth.json ({"client_id":"...","client_secret":"..."}) in the data dir.
func oauthCreds() (string, string) {
	id := os.Getenv("agent_center_GCP_CLIENT_ID")
	secret := os.Getenv("agent_center_GCP_CLIENT_SECRET")
	if id != "" && secret != "" {
		return id, secret
	}

	cfgPath := filepath.Join(filepath.Dir(database.GetDBPath()), "gcp_oauth.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return id, secret
	}
	// PowerShell's Out-File and Set-Content write a BOM by default, and
	// encoding/json rejects one outright: the file existed, parsed as nothing,
	// and the app told the user to create the file they were looking at.
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))

	// Google Cloud Console hands out client_secret_*.json with the fields
	// nested under "installed" (or "web"). Reading only the flat shape meant a
	// user who dropped in the file they actually downloaded got zero values
	// from a successful parse.
	var fileCreds struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		Installed    struct {
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
		} `json:"installed"`
		Web struct {
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
		} `json:"web"`
	}
	if err := json.Unmarshal(data, &fileCreds); err != nil {
		logger.Error(fmt.Sprintf("gcp_oauth.json không đọc được: %v", err))
		return id, secret
	}
	for _, pair := range [][2]string{
		{fileCreds.ClientID, fileCreds.ClientSecret},
		{fileCreds.Installed.ClientID, fileCreds.Installed.ClientSecret},
		{fileCreds.Web.ClientID, fileCreds.Web.ClientSecret},
	} {
		if id == "" {
			id = pair[0]
		}
		if secret == "" {
			secret = pair[1]
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
				"message": "⚠️ Thiếu GCP OAuth credentials. Đặt biến môi trường agent_center_GCP_CLIENT_ID / _CLIENT_SECRET hoặc tạo file gcp_oauth.json trong thư mục dữ liệu.",
				"level":   "ERROR",
				"time":    time.Now().Format("15:04:05"),
			})
		}
		return ""
	}

	boundPort := 8045
	state := ""
	if a.oauthListener != nil {
		var err error
		boundPort, state, err = a.oauthListener.StartOAuthListener(clientID, clientSecret, func(email, accessToken, refreshToken string) {
			if refreshToken != "" {
				cli.GlobalAntiPool.AddRefreshAccount(email, refreshToken)
				a.saveAntiKeys()
			} else {
				// Fallback if user didn't grant offline access or no refresh token was returned
				cli.GlobalAntiPool.AddOAuthKey("Google ("+email+")", accessToken)
			}
			a.emitAgentsUpdated() // Update UI

			if a.ctx != nil {
				wailsRuntime.EventsEmit(a.ctx, "oauth_success", map[string]string{
					"email": email,
					"token": accessToken,
				})
			}
		})
		if err != nil {
			// A dead listener means Google's redirect lands on nothing. Say
			// so instead of opening a browser flow that cannot complete.
			if a.ctx != nil {
				wailsRuntime.EventsEmit(a.ctx, "log_entry", map[string]string{
					"message": "❌ Không mở được cổng đăng nhập Google: " + err.Error(),
					"level":   "ERROR",
					"time":    time.Now().Format("15:04:05"),
				})
			}
			return ""
		}
	}

	redirectURI := fmt.Sprintf("http://localhost:%d/auth/callback", boundPort)
	// access_type=offline + prompt=consent so Google returns a refresh_token;
	// state ties the callback to this click — the listener rejects anything else.
	authURL := "https://accounts.google.com/o/oauth2/v2/auth?client_id=" + clientID + "&redirect_uri=" + url.QueryEscape(redirectURI) + "&response_type=code&scope=https://www.googleapis.com/auth/cloud-platform%20https://www.googleapis.com/auth/userinfo.email&access_type=offline&prompt=consent&state=" + state
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

// ── Project Map ─────────────────────────────────────────────────────────

// RebuildProjectMap scans the current workspace and rebuilds its code graph.
// Deterministic only — zero LLM tokens.
func (a *App) RebuildProjectMap() (*projectmap.BuildReport, error) {
	ws := a.workspaceConfig.LastWorkspaceFolder
	if ws == "" {
		return nil, fmt.Errorf("chưa chọn workspace")
	}
	return a.projectMapper.FullBuild(ws)
}

// GetProjectMap reports the stored map's size and freshness for the UI.
func (a *App) GetProjectMap() (*models.ProjectMapStats, error) {
	ws := a.workspaceConfig.LastWorkspaceFolder
	if ws == "" {
		return nil, fmt.Errorf("chưa chọn workspace")
	}
	return a.projectMapper.Stats(ws)
}

// GetProjectMapStaleness says how far the map lags the working tree.
func (a *App) GetProjectMapStaleness() (models.StalenessReport, error) {
	ws := a.workspaceConfig.LastWorkspaceFolder
	if ws == "" {
		return models.StalenessReport{Status: "unknown"}, nil
	}
	return a.projectMapper.Staleness(ws), nil
}

// RefreshProjectSummaries enriches stale file summaries with the cheap model,
// up to 3 batches per click. Returns how many files were refreshed.
func (a *App) RefreshProjectSummaries() (int, error) {
	ws := a.workspaceConfig.LastWorkspaceFolder
	if ws == "" {
		return 0, fmt.Errorf("chưa chọn workspace")
	}
	return a.projectMapper.RefreshSummaries(ws, 3)
}

// ── Memory (lessons, regressions, pack preview) ─────────────────────────

// currentWorkspaceID resolves the identity of the selected workspace through
// the same resolver the mapper and the capture path use.
func (a *App) currentWorkspaceID() (string, error) {
	ws := a.workspaceConfig.LastWorkspaceFolder
	if ws == "" {
		return "", fmt.Errorf("chưa chọn workspace")
	}
	return database.WorkspaceIDFor(ws, projectmap.WorkspaceRemoteIdentity(ws)), nil
}

// GetMemoryLessons lists the workspace's lessons by status (pending|active|archived).
func (a *App) GetMemoryLessons(status string) ([]models.MemoryLesson, error) {
	wsID, err := a.currentWorkspaceID()
	if err != nil {
		return nil, err
	}
	lessons, err := a.lessonRepo.ListByStatus(wsID, status, 200)
	if err != nil {
		return nil, err
	}
	if lessons == nil {
		lessons = []models.MemoryLesson{}
	}
	return lessons, nil
}

// ReviewMemoryLesson applies a human decision to a lesson: approve|reject|archive.
func (a *App) ReviewMemoryLesson(lessonID, decision string) error {
	status, err := database.ReviewDecisionStatus(decision)
	if err != nil {
		return err
	}
	return a.lessonRepo.SetStatus(lessonID, status)
}

// GetRegressions lists the workspace's recorded fixed bugs.
func (a *App) GetRegressions() ([]models.Regression, error) {
	wsID, err := a.currentWorkspaceID()
	if err != nil {
		return nil, err
	}
	regs, err := a.regressionRepo.List(wsID, 200)
	if err != nil {
		return nil, err
	}
	if regs == nil {
		regs = []models.Regression{}
	}
	return regs, nil
}

// ApproveRegressionGuard writes a human-approved falsifier into the
// workspace's .agent-center/checks.json and links it to the regression. The
// UI shows the exact argv before this is called — never a summary.
func (a *App) ApproveRegressionGuard(regressionID, checkName string, command []string) error {
	ws := a.workspaceConfig.LastWorkspaceFolder
	if ws == "" {
		return fmt.Errorf("chưa chọn workspace")
	}
	reg, err := a.regressionRepo.Get(regressionID)
	if err != nil {
		return err
	}
	if reg == nil {
		return fmt.Errorf("không tìm thấy regression %s", regressionID)
	}
	if err := claims.AppendCheck(ws, claims.Check{
		Name:        checkName,
		Description: "Guard cho regression: " + reg.Title,
		Command:     command,
		TimeoutSec:  120,
	}); err != nil {
		return err
	}
	return a.regressionRepo.SetGuard(regressionID, checkName, "approved")
}

// applyMemoryConfig pushes the stored knobs into the running services — the
// single place config becomes behavior, called at startup and on save.
func (a *App) applyMemoryConfig() {
	a.orchestrator.SetPackBudget(a.memoryConfig.ContextPackMaxChars)
	a.orchestrator.SetSessionResume(a.memoryConfig.SessionResume)
	a.projectMapper.SetAutoSummarize(a.memoryConfig.AutoSummarize)
	a.learner.SetPromotionEnabled(a.memoryConfig.LessonPromotion)
}

// GetMemoryConfig returns the memory subsystem's settings.
func (a *App) GetMemoryConfig() models.MemoryConfig {
	return a.memoryConfig
}

// SaveMemoryConfig persists and applies the memory settings.
func (a *App) SaveMemoryConfig(cfg models.MemoryConfig) error {
	if cfg.ContextPackMaxChars < 0 {
		cfg.ContextPackMaxChars = 0
	}
	if err := services.SaveMemoryConfig(filepath.Dir(database.GetDBPath()), cfg); err != nil {
		return err
	}
	a.memoryConfig = cfg
	a.applyMemoryConfig()
	return nil
}

// GetProjectGraph returns the directory-level view of the map for the UI.
func (a *App) GetProjectGraph() (*models.ModuleGraph, error) {
	ws := a.workspaceConfig.LastWorkspaceFolder
	if ws == "" {
		return nil, fmt.Errorf("chưa chọn workspace")
	}
	return a.projectMapper.ModuleGraph(ws)
}

// GetContextPackPreview renders exactly what a task would receive as its
// context pack right now — the demonstrability tool.
func (a *App) GetContextPackPreview(taskID string) (string, error) {
	ws := a.workspaceConfig.LastWorkspaceFolder
	task, err := a.taskRepo.GetByID(taskID)
	if err != nil {
		return "", err
	}
	if task == nil {
		return "", fmt.Errorf("không tìm thấy task %s", taskID)
	}
	wsID := ""
	if ws != "" {
		wsID = database.WorkspaceIDFor(ws, projectmap.WorkspaceRemoteIdentity(ws))
	}
	pack := a.contextMgr.BuildTaskPack(ws, wsID, task, a.orchestrator.GetPackBudget())
	if pack.Text == "" {
		return "(pack rỗng — task này chưa có context nào để tiêm)", nil
	}
	return pack.Text, nil
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
	clientID = strings.TrimSpace(clientID)
	clientSecret = strings.TrimSpace(clientSecret)

	// Rejected here rather than at the OAuth call. A blank or obviously wrong
	// value used to save happily and then fail much later, during a login, with
	// an error from Google that says nothing about where the value came from.
	if clientID == "" || clientSecret == "" {
		return fmt.Errorf("cần cả Client ID và Client Secret")
	}
	if !strings.HasSuffix(clientID, ".apps.googleusercontent.com") {
		return fmt.Errorf("Client ID phải có dạng ...apps.googleusercontent.com — bạn dán nhầm giá trị khác?")
	}

	cfgPath := filepath.Join(filepath.Dir(database.GetDBPath()), "gcp_oauth.json")
	fileCreds := map[string]string{
		"client_id":     clientID,
		"client_secret": clientSecret,
	}
	data, err := json.MarshalIndent(fileCreds, "", "  ")
	if err != nil {
		return err
	}
	// 0600, not 0644: this is a secret, and the data directory is inside the
	// user profile where other accounts on a shared machine could read it.
	return os.WriteFile(cfgPath, data, 0600)
}

// ImportGCPCredentialsFile reads the client_secret_*.json that Google Cloud
// Console hands out and stores the credentials from it.
//
// Typing a client secret by hand is where this goes wrong: it is long, it is
// pasted from a browser, and a truncated one fails at login with an error that
// blames the login. The file is the artefact Google actually gives you.
func (a *App) ImportGCPCredentialsFile() (string, error) {
	path, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title:   "Chọn tệp client_secret_....json tải từ Google Cloud Console",
		Filters: []wailsRuntime.FileFilter{{DisplayName: "JSON", Pattern: "*.json"}},
	})
	if err != nil || path == "" {
		return "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	// Console wraps the credentials in "installed" for a Desktop app and in "web"
	// for a web client. Only the desktop shape can complete the loopback flow this
	// app uses, so a web client is named as the problem rather than half-working.
	var wrapper struct {
		Installed *struct {
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
		} `json:"installed"`
		Web *struct {
			ClientID string `json:"client_id"`
		} `json:"web"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return "", fmt.Errorf("tệp không phải JSON hợp lệ: %w", err)
	}

	if wrapper.Installed == nil {
		if wrapper.Web != nil {
			return "", fmt.Errorf("đây là OAuth client loại Web. App cần loại Desktop app — tạo lại client và chọn Desktop app")
		}
		return "", fmt.Errorf("không tìm thấy phần \"installed\" trong tệp — đây có phải client_secret tải từ Google Cloud Console không?")
	}

	if err := a.SaveGCPOAuthCredentials(wrapper.Installed.ClientID, wrapper.Installed.ClientSecret); err != nil {
		return "", err
	}
	return wrapper.Installed.ClientID, nil
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

// runScheduledJob executes a due job. Each kind delegates to a capability the
// app already has, so a schedule can do anything a person could do from the UI
// rather than only sending a prompt.
func (a *App) runScheduledJob(job services.JobRequest) error {
	model := job.Model
	if model == "" {
		// Follow the provider the job picked, so a Gemini schedule does not
		// quietly execute on Claude — RunQuickCLI routes on the model name.
		model = defaultModelFor(job.Provider)
	}

	switch job.Kind {
	case services.JobKindPlan:
		return a.runScheduledPlan(job, model)
	case services.JobKindE2E:
		return a.runScheduledE2E(job)
	case services.JobKindGitCommit:
		return a.runScheduledCommit()
	case services.JobKindDigest:
		return a.runScheduledDigest()
	default: // JobKindPrompt, and anything scheduled before kinds existed
		result, err := a.RunQuickCLI(job.Prompt, model, "", nil)
		if err != nil {
			return err
		}
		if result != nil && !result.Success {
			return fmt.Errorf("%s", result.Error)
		}
		return nil
	}
}

// runScheduledPlan turns the prompt into tasks and lets the orchestrator work
// through them, which is the overnight case: describe it before bed, review the
// diff in the morning.
func (a *App) runScheduledPlan(job services.JobRequest, model string) error {
	provider := "claude_cli"
	if job.Provider == "anti" || job.Provider == "anti_cli" {
		provider = "anti_cli"
	}

	tasks, err := a.planBuilder.DecomposeWithProvider(job.Prompt, a.workspaceConfig.LastWorkspaceFolder, provider, model)
	if err != nil {
		return fmt.Errorf("decompose: %w", err)
	}
	for i := range tasks {
		if err := a.taskRepo.Create(&tasks[i]); err != nil {
			return fmt.Errorf("create task: %w", err)
		}
	}

	a.emitBoardUpdated()
	a.orchestrator.Start()
	return nil
}

// runScheduledE2E queues a browser test. It goes through the board rather than
// calling the browser directly so a failure lands in the existing repair loop:
// the orchestrator spawns a fix task and re-runs the test.
func (a *App) runScheduledE2E(job services.JobRequest) error {
	prompt := job.Prompt
	if strings.TrimSpace(prompt) == "" {
		prompt = "Kiểm thử giao diện web và báo lỗi console nếu có."
	}

	task := models.Task{
		Title:    "[E2E] " + firstLine(prompt),
		Prompt:   prompt,
		Priority: "high",
		Status:   "backlog",
	}
	if err := a.taskRepo.Create(&task); err != nil {
		return fmt.Errorf("create E2E task: %w", err)
	}

	a.emitBoardUpdated()
	a.orchestrator.Start()
	return nil
}

// runScheduledCommit writes a commit for whatever is uncommitted, so a night of
// agent work does not sit as a loose diff.
func (a *App) runScheduledCommit() error {
	workspace := a.workspaceConfig.LastWorkspaceFolder
	if workspace == "" {
		return fmt.Errorf("chưa chọn workspace")
	}

	diff, err := a.gitService.GetWorkspaceDiff(workspace)
	if err != nil {
		return fmt.Errorf("read diff: %w", err)
	}
	if strings.TrimSpace(diff) == "" {
		return nil // nothing to commit is a success, not a failure
	}

	message, err := a.GenerateCommitMessage()
	if err != nil {
		return fmt.Errorf("write commit message: %w", err)
	}
	if _, err := a.gitService.RunCommand(workspace, []string{"add", "."}); err != nil {
		return fmt.Errorf("stage changes: %w", err)
	}
	return a.gitService.CreateCommit(workspace, message)
}

// runScheduledDigest sends the day's summary to the configured webhook. With no
// webhook set there is nowhere for it to go, and saying so beats a silent no-op.
func (a *App) runScheduledDigest() error {
	if a.integrationsConfig.OutboundWebhookURL == "" {
		return fmt.Errorf("chưa cấu hình webhook để gửi tổng kết")
	}

	tasks, err := a.taskRepo.GetAll()
	if err != nil {
		return fmt.Errorf("read tasks: %w", err)
	}
	agents, err := a.agentRepo.GetAll()
	if err != nil {
		return fmt.Errorf("read agents: %w", err)
	}

	summary := services.BuildDigest(services.DigestInput{
		Tasks:  tasks,
		Agents: agents,
		Since:  time.Now().Add(-24 * time.Hour),
	})
	a.orchestrator.NotifyDigest(summary)
	return nil
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	return textutil.Truncate(strings.TrimSpace(s), 60, "…")
}

// ScheduleKind schedules a job of a given kind (prompt, plan, e2e, git-commit,
// digest).
func (a *App) ScheduleKind(kind, prompt, targetTime string, repeat bool, provider, model string, waitForQuota bool) (string, error) {
	// One atomic creation: the flag lands with the row. Patching it on after
	// insert left a gap the one-second scan ticker could fire through.
	return a.schedulerSvc.ScheduleKindJob(kind, prompt, targetTime, repeat, provider, model, waitForQuota)
}

// ── Git watch ──────────────────────────────────────────────────────────

func (a *App) GetGitWatchConfig() services.GitWatchConfig {
	return a.gitWatch.Config()
}

func (a *App) SaveGitWatchConfig(cfg services.GitWatchConfig) error {
	if err := a.gitWatch.SaveConfig(cfg); err != nil {
		return err
	}
	a.emitLog("💾 "+a.gitWatch.String(), "INFO")
	return nil
}

// GetJobKinds lists the kinds the scheduler accepts, for the UI to offer.
func (a *App) GetJobKinds() []string {
	return services.KnownJobKinds
}

// SetDailyBudgetUSD caps what the agents may spend in a day. Zero removes the cap.
func (a *App) SetDailyBudgetUSD(limit float64) float64 {
	a.orchestrator.SetDailyBudgetUSD(limit)
	_, _, applied := a.orchestrator.BudgetSnapshot()
	return applied
}

// BudgetStatus reports the day, what has been spent so far, and the ceiling.
type BudgetStatus struct {
	Day      string  `json:"day"`
	SpentUSD float64 `json:"spent_usd"`
	LimitUSD float64 `json:"limit_usd"`
}

func (a *App) GetBudgetStatus() BudgetStatus {
	day, spent, limit := a.orchestrator.BudgetSnapshot()
	return BudgetStatus{Day: day, SpentUSD: spent, LimitUSD: limit}
}

// ImportGoogleAccounts loads a file of {email, refresh_token} entries into the
// account pool.
//
// A refresh token is the part worth keeping: an access token expires in about an
// hour, so a pool built from access tokens quietly dies and rotating onto it
// after a 429 achieves nothing. Each account mints its own access token on
// demand, and again whenever that one expires.
//
// The file stays where it is; only the tokens are read. Keep it outside the
// repository — anything here is one `git add .` away from being published.
func (a *App) ImportGoogleAccounts(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("không đọc được tệp: %w", err)
	}

	accounts, err := services.ParseGoogleAccountsFile(data)
	if err != nil {
		return "", err
	}
	for _, account := range accounts {
		cli.GlobalAntiPool.AddRefreshAccount(account.Email, account.RefreshToken)
	}
	a.saveAntiKeys()

	refreshed, failed := a.RefreshAccountTokens()
	a.emitAgentsUpdated()
	return fmt.Sprintf("Đã nhập %d tài khoản (%d sẵn sàng, %d lỗi).", len(accounts), refreshed, failed), nil
}

// SelectGoogleAccountsFile asks for the file, then imports it.
func (a *App) SelectGoogleAccountsFile() (string, error) {
	path, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title:   "Chọn tệp tài khoản Google (JSON)",
		Filters: []wailsRuntime.FileFilter{{DisplayName: "JSON", Pattern: "*.json"}},
	})
	if err != nil || path == "" {
		return "", err
	}
	return a.ImportGoogleAccounts(path)
}

// RefreshAccountTokens mints an access token for every account whose one is
// missing or expired, and reports how many worked. An account Google refuses —
// a revoked token, most often — is disabled rather than retried every minute.
func (a *App) RefreshAccountTokens() (refreshed int, failed int) {
	clientID, clientSecret := oauthCreds()
	pending := cli.GlobalAntiPool.AccountsNeedingRefresh()
	if len(pending) == 0 {
		return 0, 0
	}

	// Missing OAuth creds is OUR configuration state, not Google's verdict on
	// the tokens. Disabling on it wiped every imported account before Google
	// was ever contacted — persisted, and nothing re-enables disabled
	// accounts. WarmupAntiAccountKeys already guards this identically.
	if clientID == "" || clientSecret == "" {
		a.emitLog("⚠️ Chưa cấu hình GCP OAuth (agent_center_GCP_CLIENT_ID / _CLIENT_SECRET hoặc gcp_oauth.json) — bỏ qua làm mới token, không tài khoản nào bị vô hiệu hoá.", "WARN")
		return 0, len(pending)
	}

	for id, refreshToken := range pending {
		token, err := services.RefreshGoogleAccessToken(context.Background(), clientID, clientSecret, refreshToken)
		if err != nil {
			failed++
			// Only Google's own refusal disables an account. A transport error
			// is the user's network — importing an accounts file while offline
			// used to disable every account in it, permanently, since nothing
			// re-enables them.
			if errors.Is(err, services.ErrGoogleRefused) {
				cli.GlobalAntiPool.DisableKey(id, err.Error())
				a.emitLog(fmt.Sprintf("⚠️ Tài khoản %s bị Google từ chối, đã tắt: %v", id, err), "WARN")
			} else {
				a.emitLog(fmt.Sprintf("⚠️ Tài khoản %s chưa làm mới được token (lỗi mạng?), sẽ thử lại: %v", id, err), "WARN")
			}
			continue
		}
		cli.GlobalAntiPool.SetAccessToken(id, token.AccessToken, token.ExpiresAt)
		refreshed++
	}

	if refreshed > 0 || failed > 0 {
		a.saveAntiKeys()
	}
	return refreshed, failed
}

// quotaCategory buckets a Code Assist model id into the three groups the table
// colours by. Unrecognised ids fall to "gemini" because that is what this
// endpoint is mostly listing; the display name is shown verbatim either way.
func quotaCategory(modelID string) string {
	id := strings.ToLower(modelID)
	switch {
	case strings.Contains(id, "claude"):
		return "claude"
	case strings.Contains(id, "gpt"), strings.Contains(id, "oss"):
		return "gpt"
	default:
		return "gemini"
	}
}

// fetchQuotaFor reads one account's plan and per-model quota from Google.
//
// The access token is minted first when the stored one has expired: an hour-old
// token would come back 401, which reads as "Google refused this account" and is
// not what happened.
func (a *App) fetchQuotaFor(accountID string) (services.AccountPlan, error) {
	acc := cli.GlobalAntiPool.AccountByID(accountID)
	if acc == nil {
		return services.AccountPlan{}, fmt.Errorf("không tìm thấy tài khoản %s", accountID)
	}
	// A bare API key is not a Google sign-in and has no plan to read. Saying so
	// beats sending it and reporting whatever 401 comes back.
	if acc.RefreshToken == "" && acc.OAuthToken == "" {
		return services.AccountPlan{}, fmt.Errorf("tài khoản này là API key, không phải đăng nhập Google — không có gói để đọc")
	}

	if acc.OAuthToken == "" || (!acc.TokenExpiresAt.IsZero() && time.Now().After(acc.TokenExpiresAt)) {
		if err := a.refreshOneAccountToken(accountID); err != nil {
			return services.AccountPlan{}, err
		}
		acc = cli.GlobalAntiPool.AccountByID(accountID)
		if acc == nil {
			return services.AccountPlan{}, fmt.Errorf("tài khoản %s biến mất khi đang làm mới token", accountID)
		}
	}

	return services.FetchAccountPlan(context.Background(), acc.OAuthToken)
}

// RefreshAntiAccountQuota reads gói (FREE/PRO/ULTRA) and per-model quota for one
// account from Google's Code Assist endpoints, and records it against the
// account with tier_source "google".
//
// This is the first thing in the app that genuinely asks Google about quota. The
// table it fills used to be a compile-time constant — the same 53% for every
// account — and backend/cli/quota_honesty_test.go exists to keep that from
// coming back. Nothing here writes a number Google did not send: a partial
// answer becomes a blank with a reason attached, never a plausible-looking bar.
func (a *App) RefreshAntiAccountQuota(accountID string) map[string]interface{} {
	plan, err := a.fetchQuotaFor(accountID)

	quotas := make([]cli.ModelQuota, 0, len(plan.Models))
	for _, m := range plan.Models {
		frac := m.RemainingFraction
		if !m.HasQuota {
			frac = -1
		}
		var resetIn time.Duration
		if !m.ResetTime.IsZero() {
			resetIn = time.Until(m.ResetTime)
		}
		quotas = append(quotas, cli.QuotaFromFraction(m.DisplayName, quotaCategory(m.ID), frac, m.IsExhausted, resetIn))
	}

	reason := ""
	if err != nil {
		reason = err.Error()
	}
	cli.GlobalAntiPool.ApplyQuota(accountID, plan.Tier, plan.RawTier, plan.ProjectID, quotas, reason)
	a.saveAntiKeys()

	if err != nil {
		a.emitLog(fmt.Sprintf("⚠️ Chưa đọc được hạn mức của %s: %v", accountLabel(accountID), err), "WARN")
		return map[string]interface{}{"ok": false, "error": reason, "tier": plan.Tier}
	}
	a.emitLog(fmt.Sprintf("✅ Đã đọc gói %s và hạn mức %d model của %s.", plan.Tier, len(quotas), accountLabel(accountID)), "SUCCESS")
	return map[string]interface{}{"ok": true, "tier": plan.Tier, "raw_tier": plan.RawTier, "models": len(quotas)}
}

// RefreshAllAntiAccountQuota does the same for every account in the pool,
// sequentially — eleven accounts hitting one internal endpoint at once is how a
// pool gets itself rate-limited while asking about rate limits.
func (a *App) RefreshAllAntiAccountQuota() map[string]interface{} {
	keys := cli.GlobalAntiPool.GetKeys()
	ok, failed := 0, 0
	for _, k := range keys {
		res := a.RefreshAntiAccountQuota(k.ID)
		if v, _ := res["ok"].(bool); v {
			ok++
		} else {
			failed++
		}
	}
	a.emitLog(fmt.Sprintf("Đọc hạn mức xong: %d tài khoản có dữ liệu, %d không.", ok, failed), "INFO")
	return map[string]interface{}{"ok": ok, "failed": failed, "total": len(keys)}
}

// refreshOneAccountToken mints a fresh access token for a single account, on
// the run path, so an expired token never reaches the CLI. Disabling follows
// the same rule as everywhere else: only Google's refusal counts.
func (a *App) refreshOneAccountToken(accountID string) error {
	acc := cli.GlobalAntiPool.AccountByID(accountID)
	if acc == nil {
		return fmt.Errorf("không tìm thấy tài khoản %s", accountID)
	}
	if acc.RefreshToken == "" {
		return fmt.Errorf("tài khoản %s không có refresh token", accountLabel(accountID))
	}
	clientID, clientSecret := oauthCreds()
	if clientID == "" || clientSecret == "" {
		return fmt.Errorf("chưa cấu hình GCP OAuth credentials")
	}
	token, err := services.RefreshGoogleAccessToken(context.Background(), clientID, clientSecret, acc.RefreshToken)
	if err != nil {
		if errors.Is(err, services.ErrGoogleRefused) {
			cli.GlobalAntiPool.DisableKey(accountID, err.Error())
			a.saveAntiKeys()
		}
		return err
	}
	cli.GlobalAntiPool.SetAccessToken(accountID, token.AccessToken, token.ExpiresAt)
	a.saveAntiKeys()
	return nil
}

func (a *App) emitLog(message, level string) {
	if a.ctx == nil {
		return
	}
	entry := map[string]string{
		"message": message,
		"level":   level,
		"time":    time.Now().Format("15:04:05"),
	}

	// Every line goes to the log file, not just the queued ones: the event bus
	// keeps nothing, so an error the user scrolled past is gone the moment the
	// app closes.
	if level == "ERROR" {
		logger.Error(message)
	} else {
		logger.Info(level + ": " + message)
	}

	// Wails drops an event nobody is listening for, and the frontend registers
	// its listeners as it mounts — so everything startup had to say (a data
	// file that could not be decrypted, tasks recovered from a killed session)
	// used to vanish. Those lines are held until the DOM is ready.
	a.noticeMu.Lock()
	if !a.domIsReady {
		// Bounded: a broken WebView2 runtime means OnDomReady never fires, and
		// the schedulers keep logging for as long as the app is open.
		if len(a.pendingNotices) < 500 {
			a.pendingNotices = append(a.pendingNotices, entry)
		}
		a.noticeMu.Unlock()
		return
	}
	a.noticeMu.Unlock()

	wailsRuntime.EventsEmit(a.ctx, "log_entry", entry)
}

// domReady flushes what startup said before anyone could hear it.
//
// The flush is delayed a beat on purpose. OnDomReady is DOMContentLoaded; the
// frontend registers its listeners in onMount, and Wails drops an event with
// no listener — so flushing at the instant DOM-ready fires can still be
// flushing into nothing on a slow first paint. App.svelte registers before its
// awaits for the same reason; this is the second half of the same fix.
func (a *App) domReady(ctx context.Context) {
	time.Sleep(300 * time.Millisecond)

	a.noticeMu.Lock()
	a.domIsReady = true
	pending := a.pendingNotices
	a.pendingNotices = nil
	a.noticeMu.Unlock()

	for _, entry := range pending {
		wailsRuntime.EventsEmit(ctx, "log_entry", entry)
	}

	// A previous update that could not swap the exe relaunched the old version
	// and said nothing, so the same update was offered again forever.
	if a.updaterService != nil && a.updaterService.TakeFailedSwapNotice() {
		a.emitLog("⚠️ Lần cập nhật trước không thay được file chương trình (file đang bị khoá — thường do phần mềm diệt virus hoặc thư mục đang đồng bộ). App vẫn chạy bản cũ; hãy thử cập nhật lại.", "WARN")
	}
}

// ── Claim adjudication ─────────────────────────────────────────────────
//
// Two members' agents reaching opposite conclusions is settled by running the
// check each of them named, not by letting them argue. These methods drive that
// from the desktop app; agents on other machines take part through
// cmd/agent-center-claim.

// ClaimsHostStatus reports whether agents can currently connect.
func (a *App) ClaimsHostStatus() map[string]interface{} {
	// A running host keeps the workspace it was started with, so the checks
	// listed here must come from THAT folder: reading the currently selected
	// one let the page advertise checks the adjudicator would look for
	// somewhere else after a workspace switch.
	workspace := a.workspaceConfig.LastWorkspaceFolder
	if a.claimsHost.IsRunning() {
		workspace = a.claimsHost.Workspace()
	}
	if workspace == "" {
		// Discovery against an empty path stats relative names in whatever
		// directory the exe was launched from — Downloads, for a portable copy
		// — and the page then lists falsifiers belonging to a folder the user
		// never chose. Do not look at all.
		return map[string]interface{}{
			"running":  a.claimsHost.IsRunning(),
			"addr":     a.claimsHost.Addr(),
			"checks":   []string{},
			"warning":  "Chưa chọn workspace — hãy chọn thư mục dự án trước, catalogue check và falsifier đều đọc từ đó.",
			"sessions": a.claimsHost.SessionIDs(),
		}
	}
	checks, err := a.claimsHost.CatalogueNames(workspace)
	warning := ""
	if err != nil {
		warning = err.Error()
	} else if len(checks) == 0 {
		// Worth saying plainly: with no checks every claim becomes an opinion and
		// nothing can ever block, which looks like the feature is broken. Checks
		// are discovered from the workspace under review, not from this
		// repository, so an empty list usually means the wrong folder is selected.
		warning = "Không tìm thấy check nào trong workspace này (không có go.mod, cũng không có package.json với script test/lint/check/build). Mọi claim sẽ thành ý kiến và không có gì chặn được merge."
	}
	return map[string]interface{}{
		"running":  a.claimsHost.IsRunning(),
		"addr":     a.claimsHost.Addr(),
		"checks":   checks,
		"warning":  warning,
		"sessions": a.claimsHost.SessionIDs(),
	}
}

// StartClaimsHost begins accepting agents on port.
func (a *App) StartClaimsHost(port int) map[string]interface{} {
	workspace := a.workspaceConfig.LastWorkspaceFolder
	if workspace == "" {
		return map[string]interface{}{"success": false, "error": "Chọn workspace trước: catalogue check và falsifier đều chạy trong đó."}
	}
	if err := a.claimsHost.Start(port, workspace); err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	return map[string]interface{}{"success": true, "addr": a.claimsHost.Addr()}
}

// StopClaimsHost stops accepting agents.
func (a *App) StopClaimsHost() error {
	return a.claimsHost.Stop()
}

// OpenClaimSession starts a session and returns what a teammate needs to join.
func (a *App) OpenClaimSession(subject string, ttlMinutes int) map[string]interface{} {
	if ttlMinutes <= 0 {
		ttlMinutes = 60
	}
	id, token, err := a.claimsHost.Open(subject, ttlMinutes)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}

	// One address per case the user might be in, rather than one address that is
	// only right for the case they are probably not in. See claims.JoinTargets.
	targets := claims.JoinTargets(a.claimsHost.Addr())
	host := ""
	if len(targets) > 0 {
		host = targets[0].Host
	}

	commands := make([]map[string]string, 0, len(targets))
	for _, t := range targets {
		commands = append(commands, map[string]string{
			"scope":   t.Scope,
			"label":   t.Label,
			"host":    t.Host,
			"command": claims.JoinCommand(t.Host, id, token),
			// For the teammate who never installed the app: a one-line
			// PowerShell bootstrap, an MCP registration that needs no
			// download at all, and a paste-into-any-AI prompt carrying the
			// whole decision tree (has app → use it; no app → ask to
			// download, else MCP).
			"bootstrap": claims.BootstrapJoinCommand(t.Host, id, token),
			"mcp":       claims.MCPJoinCommand(t.Host, id, token),
			"prompt":    claims.AgentJoinPrompt(t.Host, id, token, subject),
		})
	}

	return map[string]interface{}{
		"success": true,
		"id":      id,
		"token":   token,
		"host":    host,
		"targets": commands,
		// Kept for the local case so existing callers keep working; "targets" is
		// what the UI should show, because it says which address is for whom.
		"join_command": claims.JoinCommand(host, id, token),
		// Whether the tool this command names is actually installed. Without
		// this the UI hands out a line that fails with "not found" and no cause.
		"tool_installed": claimToolInstalled(),
	}
}

// BuildJoinCommands builds the join hand-outs for a host this machine cannot
// discover by itself — a tunnel URL (cloudflared/ngrok), a domain behind a
// reverse proxy, or a router port-forward. This is the door for a teammate
// who shares neither the LAN nor a tailnet: they only need to reach the URL.
func (a *App) BuildJoinCommands(rawHost, sessionID, token, subject string) map[string]string {
	host, err := claims.NormalizeJoinHost(rawHost)
	if err != nil {
		return map[string]string{"error": err.Error()}
	}
	return map[string]string{
		"host":      host,
		"command":   claims.JoinCommand(host, sessionID, token),
		"bootstrap": claims.BootstrapJoinCommand(host, sessionID, token),
		"mcp":       claims.MCPJoinCommand(host, sessionID, token),
		"prompt":    claims.AgentJoinPrompt(host, sessionID, token, subject),
	}
}

// CreateChecksFile writes a starter .agent-center/checks.json for the current
// workspace, seeded with whatever the project's own tooling implies.
//
// Without a catalogue every claim is an opinion and nothing can block a merge —
// the UI said so, but had no way to help. This gives the user a file to edit
// rather than a format to look up.
func (a *App) CreateChecksFile() (string, error) {
	workspace := a.workspaceConfig.LastWorkspaceFolder
	if workspace == "" {
		return "", fmt.Errorf("chưa chọn workspace")
	}

	dir := filepath.Join(workspace, ".agent-center")
	path := filepath.Join(dir, "checks.json")
	// Never overwrite: the existing file is the user's, and it may be the only
	// copy of checks they wrote by hand.
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("%s đã tồn tại — mở tệp đó để sửa", path)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	discovered := claims.Discover(workspace)
	if len(discovered.Checks) == 0 {
		// A template beats an empty file: it shows the shape without the user
		// having to find documentation for it.
		discovered.Checks = []claims.Check{{
			Name:        "my-test",
			Description: "Đổi thành lệnh kiểm thử của dự án bạn",
			Command:     []string{"echo", "thay-bang-lenh-that"},
			TimeoutSec:  600,
		}}
	}

	data, err := json.MarshalIndent(discovered, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return path, nil
}

// SayInClaimSession posts a chat message from the person running the app, so the
// arbiter is a participant in the discussion rather than a spectator to it.
// It goes through Host.Say so the message is also pushed to every connected
// agent — written straight into the session it sat unread until the next
// broadcast, which for a quiet session was never.
func (a *App) SayInClaimSession(sessionID, author, text string) error {
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("tin nhắn trống")
	}
	if author == "" {
		author = "trọng tài"
	}
	if err := a.claimsHost.Say(sessionID, author, text); err != nil {
		return fmt.Errorf("không gửi được vào phiên %q: %w", sessionID, err)
	}
	return nil
}

// GetClaimChat returns the discussion in a session.
func (a *App) GetClaimChat(sessionID string) []claims.ChatMessage {
	session, ok := a.claimsHost.Session(sessionID)
	if !ok {
		return nil
	}
	return session.Chat()
}

// GetClaimSession returns a session's current state for the UI.
func (a *App) GetClaimSession(sessionID string) map[string]interface{} {
	session, ok := a.claimsHost.Session(sessionID)
	if !ok {
		return map[string]interface{}{"found": false}
	}
	// Everything visible: the UI is the operator's view, not an agent's, so the
	// blind-collect restriction does not apply to it.
	return map[string]interface{}{
		"found":        true,
		"id":           session.ID,
		"subject":      session.Subject,
		"phase":        string(session.Phase()),
		"claims":       session.VisibleTo(""),
		"warnings":     session.Warnings(),
		"round":        session.DebateRound(),
		"maxRound":     claims.MaxDebateRounds,
		"opinions":     session.OpenOpinions(),
		"remarks":      session.Remarks(),
		"participants": session.ParticipantNames(),
	}
}

// OpenClaimDebate opens a discussion round for the claims no check could settle.
//
// Deliberately a separate, explicit step rather than something the session slides
// into. Discussion is the weakest part of this protocol — agents drift toward
// whichever side sounds most certain — so it happens only when someone asks for
// it, only for opinions, and only twice.
func (a *App) OpenClaimDebate(sessionID string) error {
	return a.claimsHost.OpenDebateRound(sessionID)
}

// ForceAdjudicateClaims ends a collect window waiting on an agent that will not
// report finished.
func (a *App) ForceAdjudicateClaims(sessionID string) error {
	return a.claimsHost.ForceAdjudicate(sessionID)
}

// FinishClaimSession closes a session and returns its outcome.
func (a *App) FinishClaimSession(sessionID string) (*claims.Outcome, error) {
	return a.claimsHost.Finish(sessionID)
}

// claimToolCommand is how the join command should invoke the claim CLI.
//
// The installer puts agent-center-claim.exe beside the app, which is not on
// PATH, so a bare name only works for someone who ran `go install` from source.
// The whole point of the copied line is that a teammate's agent can run it
// without being told to install anything first, so it names the full path when
// the tool is there and falls back to the bare name when it is not — which is
// the development case, where PATH does have it.
// claimToolInstalled reports whether agent-center-claim can be found — beside
// the app, as the installer places it, or on PATH.
//
// The command handed to a teammate names the tool without a path, so this is what
// decides whether that command can work at all. The previous version returned the
// absolute path of *this* machine and embedded it in a line meant to be run on
// another one; it worked only while both sides used the same install directory.
func claimToolInstalled() bool {
	names := []string{"agent-center-claim", "agent-center-claim.exe"}

	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for _, name := range names {
			if info, statErr := os.Stat(filepath.Join(dir, name)); statErr == nil && !info.IsDir() {
				return true
			}
		}
	}

	for _, name := range names {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	return false
}

// StartTunnelProcess runs a shell/tunnel command (like cloudflared or ngrok) in background
// and streams its log output line by line via Wails event 'tunnel_output'.
// If a public URL (trycloudflare, ngrok, loca.lt) is detected in the log,
// it emits 'tunnel_url_detected'.
func (a *App) StartTunnelProcess(cmdStr string) (string, error) {
	cmdStr = strings.TrimSpace(cmdStr)
	if cmdStr == "" {
		return "", fmt.Errorf("Lệnh rỗng")
	}

	a.tunnelMu.Lock()
	if a.tunnelCmd != nil && a.tunnelCmd.Process != nil {
		_ = a.tunnelCmd.Process.Kill()
		if a.tunnelCancel != nil {
			a.tunnelCancel()
		}
		a.tunnelCmd = nil
		a.tunnelCancel = nil
	}
	a.tunnelMu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = sysproc.CommandContext(ctx, "powershell.exe", "-NoProfile", "-Command", cmdStr)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", cmdStr)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return "", fmt.Errorf("không mở được stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return "", fmt.Errorf("không mở được stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return "", fmt.Errorf("không khởi chạy được lệnh: %w", err)
	}

	a.tunnelMu.Lock()
	a.tunnelCmd = cmd
	a.tunnelCancel = cancel
	a.tunnelMu.Unlock()

	scanPipe := func(r io.Reader) {
		scanner := bufio.NewScanner(r)
		urlRegex := regexp.MustCompile(`https://[a-zA-Z0-9-]+\.(trycloudflare\.com|ngrok-free\.app|ngrok\.io|loca\.lt|lvh\.me)`)
		for scanner.Scan() {
			line := scanner.Text()
			if a.ctx != nil {
				wailsRuntime.EventsEmit(a.ctx, "tunnel_output", map[string]string{
					"line": line,
				})
				if match := urlRegex.FindString(line); match != "" {
					wailsRuntime.EventsEmit(a.ctx, "tunnel_url_detected", map[string]string{
						"url": match,
					})
				}
			}
		}
	}

	go scanPipe(stdout)
	go scanPipe(stderr)

	go func() {
		_ = cmd.Wait()
		a.tunnelMu.Lock()
		if a.tunnelCmd == cmd {
			a.tunnelCmd = nil
			a.tunnelCancel = nil
		}
		a.tunnelMu.Unlock()
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, "tunnel_stopped", map[string]string{})
		}
	}()

	return "Đã khởi chạy lệnh tunnel", nil
}

func (a *App) StopTunnelProcess() bool {
	a.tunnelMu.Lock()
	defer a.tunnelMu.Unlock()
	if a.tunnelCmd != nil && a.tunnelCmd.Process != nil {
		_ = a.tunnelCmd.Process.Kill()
		if a.tunnelCancel != nil {
			a.tunnelCancel()
		}
		a.tunnelCmd = nil
		a.tunnelCancel = nil
		return true
	}
	return false
}

func (a *App) IsTunnelRunning() bool {
	a.tunnelMu.Lock()
	defer a.tunnelMu.Unlock()
	return a.tunnelCmd != nil && a.tunnelCmd.Process != nil
}
