package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"claude_suite/backend/cli"
	"claude_suite/backend/database"
	"claude_suite/backend/models"
	"claude_suite/backend/services"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// defaultMaxConcurrency is how many sub-agents may execute tasks in parallel.
const defaultMaxConcurrency = 3

type Orchestrator struct {
	ctx          context.Context
	agentRepo    *database.AgentRepository
	taskRepo     *database.TaskRepository
	memoryRepo   *database.MemoryRepository
	cliRunner    cli.CLIRunner
	contextMgr   *services.ContextManager
	gitService   *services.GitService
	browserSvc   *services.BrowserAgentService
	verifySvc    *services.VerifyService
	notifierSvc  *services.NotifierService
	dispatcher   *AgentDispatcher
	fallback     *FallbackHandler
	workspaceDir string
	verifyBuild  bool

	running        bool
	autoApproveAll bool
	stopCh         chan struct{}
	mu             sync.Mutex
	onLog          func(message, level string)
	onApproval     func(taskID, agentName, taskTitle string)
	onBoard        func()

	approvalMu    sync.Mutex
	approvalChans map[string]chan bool // taskID -> approval channel (per-task)

	maxConcurrency int
	runMu          sync.Mutex
	activeRuns     map[string]context.CancelFunc // taskID -> cancel func for in-flight tasks
	wg             sync.WaitGroup
}

// SetEventHandlers connects a frontend-neutral event sink. Wails runtime
// events remain available; terminal and other frontends can subscribe without
// importing Wails bindings.
func (o *Orchestrator) SetEventHandlers(
	onLog func(message, level string),
	onApproval func(taskID, agentName, taskTitle string),
	onBoard func(),
) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.onLog, o.onApproval, o.onBoard = onLog, onApproval, onBoard
}

func NewOrchestrator(
	agentRepo *database.AgentRepository,
	taskRepo *database.TaskRepository,
	memoryRepo *database.MemoryRepository,
	cliRunner cli.CLIRunner,
	ctxMgr *services.ContextManager,
	gitSvc *services.GitService,
	browserSvc *services.BrowserAgentService,
) *Orchestrator {
	return &Orchestrator{
		agentRepo:      agentRepo,
		taskRepo:       taskRepo,
		memoryRepo:     memoryRepo,
		cliRunner:      cliRunner,
		contextMgr:     ctxMgr,
		gitService:     gitSvc,
		browserSvc:     browserSvc,
		verifySvc:      services.NewVerifyService(),
		notifierSvc:    services.NewNotifierService(),
		verifyBuild:    true,
		dispatcher:     NewAgentDispatcher(),
		fallback:       NewFallbackHandler(),
		approvalChans:  make(map[string]chan bool),
		maxConcurrency: defaultMaxConcurrency,
		activeRuns:     make(map[string]context.CancelFunc),
	}
}

func (o *Orchestrator) SetContext(ctx context.Context) {
	o.ctx = ctx
}

func (o *Orchestrator) SetWorkspaceDir(dir string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.workspaceDir = dir
}

// GetWorkspaceDir returns the current workspace directory under lock — used by
// task goroutines, which run concurrently with SetWorkspaceDir on the main thread.
func (o *Orchestrator) GetWorkspaceDir() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.workspaceDir
}

func (o *Orchestrator) SetMaxConcurrency(n int) {
	if n < 1 {
		n = 1
	}
	o.runMu.Lock()
	o.maxConcurrency = n
	o.runMu.Unlock()
	o.emitLog(fmt.Sprintf("⚙️ Số agent chạy song song tối đa: %d", n), "INFO")
}

func (o *Orchestrator) GetMaxConcurrency() int {
	o.runMu.Lock()
	defer o.runMu.Unlock()
	return o.maxConcurrency
}

func (o *Orchestrator) SetVerifyBuild(enabled bool) {
	o.mu.Lock()
	o.verifyBuild = enabled
	o.mu.Unlock()
	o.emitLog(fmt.Sprintf("🔧 Build verify trước khi Done: %v", enabled), "INFO")
}

func (o *Orchestrator) GetVerifyBuild() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.verifyBuild
}

func (o *Orchestrator) SetNotifierURL(url string) {
	o.notifierSvc.SetWebhookURL(url)
}

func (o *Orchestrator) GetNotifierURL() string {
	return o.notifierSvc.GetWebhookURL()
}

func (o *Orchestrator) Start() {
	o.mu.Lock()
	if o.running {
		o.mu.Unlock()
		return
	}
	o.running = true
	o.stopCh = make(chan struct{})
	// Hand this run's channel to the loop rather than letting it read the field.
	// A later Start replaces the field, and an old loop still reading it is a data
	// race — one the race detector catches on the Stop-then-Start path a user
	// takes with two button presses.
	stop := o.stopCh
	o.mu.Unlock()

	o.emitLog(fmt.Sprintf("🚀 Orchestrator ACTIVE: Quét Backlog & phân công song song (tối đa %d agent).", o.GetMaxConcurrency()), "INFO")
	go o.loop(stop)
}

// stopChannel returns the channel for the current run under the lock. A caller
// holds on to what it is given: a task that started under one run should keep
// watching that run's channel even if the orchestrator is started again.
func (o *Orchestrator) stopChannel() <-chan struct{} {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.stopCh
}

func (o *Orchestrator) Stop() {
	o.mu.Lock()
	if !o.running {
		o.mu.Unlock()
		return
	}
	o.running = false
	close(o.stopCh)
	o.mu.Unlock()

	o.emitLog("🛑 Orchestrator INACTIVE: Đã tạm dừng quét Backlog (task đang chạy vẫn tiếp tục).", "WARN")
}

func (o *Orchestrator) IsRunning() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.running
}

func (o *Orchestrator) ResolveApproval(taskID string, approved bool) {
	o.approvalMu.Lock()
	defer o.approvalMu.Unlock()
	if taskID == "" {
		// Backward-compatible: resolve all pending approvals with this decision.
		for _, ch := range o.approvalChans {
			select {
			case ch <- approved:
			default:
			}
		}
		return
	}
	if ch, ok := o.approvalChans[taskID]; ok {
		select {
		case ch <- approved:
		default:
		}
	}
}

func (o *Orchestrator) SetAutoApproveAll(enabled bool) {
	o.mu.Lock()
	o.autoApproveAll = enabled
	o.mu.Unlock()
	if enabled {
		o.emitLog("⚡ Đã bật chế độ 'Luôn Luôn Cho Phép' (Auto-Approve All Tasks).", "SUCCESS")
	}
}

func (o *Orchestrator) GetAutoApproveAll() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.autoApproveAll
}

func (o *Orchestrator) loop(stop <-chan struct{}) {
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			o.dispatchAvailable()
		}
	}
}

// baseCtx returns the app context if available, else Background — parent for task runs.
func (o *Orchestrator) baseCtx() context.Context {
	if o.ctx != nil {
		return o.ctx
	}
	return context.Background()
}

// dispatchAvailable launches as many dependency-satisfied tasks as there are
// free concurrency slots, each in its own goroutine (true parallel execution).
func (o *Orchestrator) dispatchAvailable() {
	for {
		o.runMu.Lock()
		slots := o.maxConcurrency - len(o.activeRuns)
		o.runMu.Unlock()
		if slots <= 0 {
			return
		}

		task, err := o.taskRepo.NextDispatchable()
		if err != nil || task == nil {
			return
		}

		o.runMu.Lock()
		if _, busy := o.activeRuns[task.TaskID]; busy {
			// Already in-flight (DB not yet reflecting running); stop to avoid a busy loop.
			o.runMu.Unlock()
			return
		}
		o.runMu.Unlock()

		agent := o.prepareAgentForTask(task)
		if agent == nil {
			return
		}

		// Mark running synchronously so the next NextDispatchable won't repick it.
		_ = o.taskRepo.AssignTask(task.TaskID, agent.Name)
		_ = o.taskRepo.UpdateStatus(task.TaskID, "running", "", "")
		agent.Status = "running"
		agent.LastTask = task.Title
		_ = o.agentRepo.SetStatus(agent.AgentID, "running", task.Title, "")
		o.emitBoard()
		o.emitAgents()

		ctx, cancel := context.WithCancel(o.baseCtx())
		o.runMu.Lock()
		o.activeRuns[task.TaskID] = cancel
		o.runMu.Unlock()

		o.wg.Add(1)
		go func(t models.Task, ag models.Agent) {
			defer o.wg.Done()
			o.runTask(ctx, &t, &ag)
			o.runMu.Lock()
			delete(o.activeRuns, t.TaskID)
			o.runMu.Unlock()
			cancel()
		}(*task, *agent)
	}
}

// prepareAgentForTask resolves (and if necessary bootstraps/creates) the agent
// that should handle a task. Returns nil if no suitable agent can be determined.
func (o *Orchestrator) prepareAgentForTask(task *models.Task) *models.Agent {
	o.emitLog(fmt.Sprintf("📋 Quét Backlog: Task '%s'. Đang phân tích Agent phù hợp...", task.Title), "INFO")

	agents, err := o.agentRepo.GetAll()
	if err != nil || len(agents) == 0 {
		o.emitLog("⚠️ Không tìm thấy Agent trong Database. Tự động tạo Agent mặc định...", "WARN")
		_ = o.agentRepo.ResetToDefaults()
		agents, _ = o.agentRepo.GetAll()
	}

	agent := o.dispatcher.FindMatchingAgent(task, agents)
	if agent == nil {
		o.emitLog(fmt.Sprintf("⚠️ Không tìm thấy Agent phù hợp cho task '%s'", task.Title), "WARN")
		return nil
	}

	if agent.AgentID == "" {
		_ = o.agentRepo.Create(agent)
		o.emitLog(fmt.Sprintf("✨ Tự động tạo Sub-Agent mới: '%s' (%s - Model: %s)", agent.Name, agent.Role, agent.Model), "SUCCESS")
	}

	o.emitLog(fmt.Sprintf("🤖 Phân công Task '%s' cho Agent '%s' (%s - %s)", task.Title, agent.Name, agent.Provider, agent.Model), "INFO")
	return agent
}

// runTask executes a single task to completion (or cancellation) in its own goroutine.
func (o *Orchestrator) runTask(ctx context.Context, task *models.Task, agent *models.Agent) {
	// A panic in one task must never crash the whole orchestrator/app.
	defer func() {
		if r := recover(); r != nil {
			o.emitTaskLog(task.TaskID, fmt.Sprintf("💥 Panic khi chạy task: %v", r), "ERROR")
			_ = o.taskRepo.UpdateStatus(task.TaskID, "failed", fmt.Sprintf("panic: %v", r), "")
			if agent != nil {
				agent.Status = "idle"
				_ = o.agentRepo.SetStatus(agent.AgentID, "idle", "", fmt.Sprintf("panic: %v", r))
			}
			o.emitBoard()
			o.emitAgents()
		}
	}()

	// Per-task log helper: streams to both the task inspector and the global log.
	onLog := func(msg, level string) {
		o.emitTaskLog(task.TaskID, fmt.Sprintf("[%s] %s", agent.Name, msg), level)
	}

	// Automated E2E Web Test Skill Auto-Binding — only for explicitly tagged tasks.
	titleLower := strings.ToLower(task.Title)
	agentLower := strings.ToLower(agent.Name + " " + agent.Role)
	isExplicitE2E := strings.Contains(titleLower, "[e2e]") || strings.Contains(titleLower, "[webtest]") ||
		strings.Contains(titleLower, "[web test]") || strings.Contains(agentLower, "e2e") ||
		strings.Contains(agentLower, "chrome cdp")
	if isExplicitE2E && o.browserSvc != nil {
		testUrl := extractTestURL(task.Prompt + " " + task.Description)
		expects := extractExpectedTexts(task.Prompt + " " + task.Description)
		onLog(fmt.Sprintf("🌐 Chrome CDP E2E Skill: đang mở & kiểm thử %s (%d assertion)...", testUrl, len(expects)), "INFO")
		bRes, bErr := o.browserSvc.RunE2ETest(testUrl, "", expects, true, true)
		if bErr == nil && bRes != nil && bRes.Success {
			if bRes.ScreenshotBase64 != "" {
				o.emitTaskScreenshot(task.TaskID, bRes.ScreenshotBase64)
			}
			_ = o.taskRepo.AssignTask(task.TaskID, "Web E2E Tester (Chrome CDP)")
			for _, a := range bRes.Assertions {
				onLog("   "+a, "INFO")
			}
			summary := fmt.Sprintf("URL: %s\nTitle: %s\nAssertions:\n%s", bRes.URL, bRes.Title, strings.Join(bRes.Assertions, "\n"))
			if len(bRes.ConsoleErrors) > 0 {
				summary += "\nConsole errors:\n- " + strings.Join(bRes.ConsoleErrors, "\n- ")
			}
			if bRes.Passed {
				_ = o.taskRepo.UpdateStatus(task.TaskID, "done", "✅ E2E PASSED\n"+summary, "chrome-cdp-auto-session")
				agent.Status = "idle"
				_ = o.agentRepo.CompleteTask(agent.AgentID)
				onLog(fmt.Sprintf("🎉 Chrome CDP E2E '%s' PASSED.", task.Title), "SUCCESS")
			} else {
				agent.Status = "idle"
				_ = o.agentRepo.SetStatus(agent.AgentID, "idle", "", "")
				// Autonomous fix loop: spawn a coding task seeded with the failure
				// detail, then requeue this E2E test to re-run after the fix — up to
				// MaxRetries times before giving up.
				maxFix := task.MaxRetries
				if maxFix <= 0 {
					maxFix = 3
				}
				if task.RetryCount < maxFix {
					fix := &models.Task{
						Title:    "[CODE] Auto-fix lỗi E2E: " + strings.TrimPrefix(task.Title, "[E2E]"),
						Priority: "high",
						Status:   "backlog",
						Prompt:   buildE2EFixPrompt(bRes),
					}
					if err := o.taskRepo.Create(fix); err == nil {
						_ = o.taskRepo.RequeueWithDependency(task.TaskID, fix.TaskID)
						onLog(fmt.Sprintf("🔁 E2E FAILED → tạo task tự sửa '%s'. Sẽ chạy lại E2E sau khi sửa (vòng %d/%d).", fix.Title, task.RetryCount+1, maxFix), "WARN")
					} else {
						_ = o.taskRepo.UpdateStatus(task.TaskID, "failed", "❌ E2E FAILED\n"+summary, "chrome-cdp-auto-session")
						onLog("❌ Không tạo được task sửa lỗi, đánh dấu E2E failed.", "ERROR")
					}
				} else {
					_ = o.taskRepo.UpdateStatus(task.TaskID, "failed", "❌ E2E FAILED (hết số vòng tự sửa)\n"+summary, "chrome-cdp-auto-session")
					_ = o.agentRepo.SetStatus(agent.AgentID, "idle", "", "E2E assertions/console failed after auto-fix retries")
					onLog(fmt.Sprintf("❌ Chrome CDP E2E '%s' FAILED sau %d vòng tự sửa.", task.Title, maxFix), "ERROR")
				}
			}
			o.emitBoard()
			o.emitAgents()
			return
		}
		onLog(fmt.Sprintf("⚠️ Chrome CDP Auto-Test warning: %v. Chuyển cho Agent thử lại...", bErr), "WARN")
	}

	// Approval checkpoint (BA / Architect roles, unless auto-approve-all).
	nameLower := strings.ToLower(agent.Name)
	if !o.GetAutoApproveAll() && (strings.Contains(nameLower, "architect") || strings.Contains(nameLower, "(ba)")) {
		onLog("Waiting for user approval...", "WARN")

		// Read the stop channel once, under the lock, and watch that one: reading
		// the field inside the select races with a later Start replacing it.
		stop := o.stopChannel()

		approvalCh := make(chan bool, 1)
		o.approvalMu.Lock()
		o.approvalChans[task.TaskID] = approvalCh
		o.approvalMu.Unlock()
		defer func() {
			o.approvalMu.Lock()
			delete(o.approvalChans, task.TaskID)
			o.approvalMu.Unlock()
		}()

		if o.ctx != nil {
			runtime.EventsEmit(o.ctx, "ask_approval", map[string]string{
				"taskId":    task.TaskID,
				"agentName": agent.Name,
				"taskTitle": task.Title,
			})
		}
		o.emitApproval(task.TaskID, agent.Name, task.Title)

		select {
		case approved := <-approvalCh:
			if !approved {
				_ = o.taskRepo.UpdateStatus(task.TaskID, "failed", "Rejected by user", "")
				onLog(fmt.Sprintf("Task '%s' rejected by user.", task.Title), "ERROR")
				agent.Status = "idle"
				_ = o.agentRepo.SetStatus(agent.AgentID, "idle", "", "")
				o.emitBoard()
				return
			}
			onLog(fmt.Sprintf("Task '%s' approved.", task.Title), "SUCCESS")
		case <-ctx.Done():
			o.markStopped(task, agent)
			return
		case <-stop:
			// The orchestrator was paused while this task sat at the prompt. Put it
			// back in the backlog so the next Start picks it up: leaving the row as
			// "running" stranded it, because only backlog and queued rows are ever
			// dispatched, and the agent stayed busy with it until the app restarted.
			_ = o.taskRepo.UpdateStatus(task.TaskID, "backlog", "", "")
			agent.Status = "idle"
			_ = o.agentRepo.SetStatus(agent.AgentID, "idle", "", "")
			onLog(fmt.Sprintf("Task '%s' returned to the backlog: approval was still pending when the orchestrator stopped.", task.Title), "WARN")
			o.emitBoard()
			o.emitAgents()
			return
		}
	}

	onLog(fmt.Sprintf("Executing task: '%s'", task.Title), "SEND")

	// Snapshot the workspace dir once for this task run — SetWorkspaceDir can be
	// called concurrently from the main goroutine while this task is executing.
	workspaceDir := o.GetWorkspaceDir()

	if workspaceDir != "" {
		_ = o.gitService.AutoSnapshot(workspaceDir)
	}

	fullPrompt := task.Prompt
	if fullPrompt == "" {
		fullPrompt = task.Description
	}
	if workspaceDir != "" {
		fullPrompt = fmt.Sprintf("[DIRECTIVE: CREATE OR MODIFY FILES DIRECTLY IN WORKSPACE %s]\n\n%s", workspaceDir, fullPrompt)
	}

	// ── Inject Markdown Roles ──────────────────────────────────────────────
	rolesDir := filepath.Join(filepath.Dir(database.GetDBPath()), "roles")
	var roleContexts []string

	readRoleFile := func(filename string) {
		path := filepath.Join(rolesDir, filename)
		if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
			roleContexts = append(roleContexts, fmt.Sprintf("--- ROLE CONFIG: %s ---\n%s", filename, string(data)))
		}
	}

	readRoleFile("agents.md") // global rules
	if strings.Contains(strings.ToLower(agent.Provider), "anti") {
		readRoleFile("antigravity.md")
	} else {
		readRoleFile("claude.md")
	}
	roleFileName := strings.ToLower(strings.ReplaceAll(agent.Role, " ", "-")) + ".md"
	readRoleFile(roleFileName)

	runAgent := *agent
	if len(roleContexts) > 0 {
		runAgent.System = strings.Join(roleContexts, "\n\n") + "\n\n" + runAgent.System
	}
	// ───────────────────────────────────────────────────────────────────────

	result := o.cliRunner.RunAgentCtx(ctx, &runAgent, fullPrompt, onLog, workspaceDir)

	// If the task was stopped by the user mid-flight, mark it and bail out.
	if ctx.Err() != nil {
		o.markStopped(task, agent)
		return
	}

	_ = o.memoryRepo.Add(&models.MemoryItem{
		AgentID:    agent.AgentID,
		TaskID:     task.TaskID,
		SessionID:  result.SessionID,
		Role:       "assistant",
		Content:    result.Output,
		TokensUsed: int(result.TokensUsed),
	})
	if result.TokensUsed > 0 {
		_ = o.agentRepo.AddTokens(agent.AgentID, result.TokensUsed)
	}

	// Smart fallback on quota exhaustion.
	if !result.Success && o.fallback.IsQuotaExhausted(agent, result.Error) {
		newProvider, newModel := o.fallback.GetFallbackModel(agent)
		agent.Provider = newProvider
		agent.Model = newModel
		_ = o.agentRepo.SetProviderModel(agent.AgentID, newProvider, newModel)
		onLog(fmt.Sprintf("⚠️ Smart Fallback: Switched to %s (%s)", newProvider, newModel), "WARN")
		result = o.cliRunner.RunAgentCtx(ctx, agent, fullPrompt, onLog, workspaceDir)
		if ctx.Err() != nil {
			o.markStopped(task, agent)
			return
		}
	}

	// Build verification: an agent's task is only "done" if the workspace still
	// builds. A failed build is turned into a task failure so the retry loop makes
	// the agent fix it.
	if result.Success && o.GetVerifyBuild() && workspaceDir != "" {
		onLog("🔧 Đang verify build workspace (go build / npm build)...", "SEND")
		vr := o.verifySvc.Verify(workspaceDir)
		if vr.Ran && !vr.Passed {
			result.Success = false
			result.Error = "Build verification FAILED:\n" + vr.Report
			onLog("❌ Build verify FAILED — trả task về để agent sửa.", "ERROR")
		} else if vr.Ran {
			onLog("✅ Build verify passed.", "SUCCESS")
		}
	}

	if result.Success {
		_ = o.taskRepo.UpdateStatus(task.TaskID, "done", result.Output, result.SessionID)
		agent.Status = "idle"
		_ = o.agentRepo.CompleteTask(agent.AgentID)
		costStr := ""
		if result.CostUSD > 0 {
			costStr = fmt.Sprintf(" · $%.4f", result.CostUSD)
		}
		onLog(fmt.Sprintf("DONE '%s' (%.1fs · %d tokens%s)", task.Title, result.DurationSec, result.TokensUsed, costStr), "SUCCESS")
		go o.notifierSvc.NotifyTaskEvent("task_done", task.Title, "done", fmt.Sprintf("Agent %s hoàn thành trong %.1fs", agent.Name, result.DurationSec))
	} else {
		// Count the attempt in the database. This task is a copy owned by this
		// goroutine, so incrementing the struct would be forgotten the moment the
		// run ends — and the limit below would never be reached.
		retries, maxRetries, err := o.taskRepo.RecordAttemptFailure(task.TaskID)
		if err != nil {
			// Without a trustworthy count, stop rather than risk retrying forever.
			retries, maxRetries = 1, 1
			onLog(fmt.Sprintf("Could not record the failed attempt (%v); not retrying.", err), "ERROR")
		}
		if retries < maxRetries {
			backoffSec := time.Duration(1<<uint(retries)) * time.Second
			_ = o.taskRepo.UpdateStatus(task.TaskID, "backlog", "", "")
			onLog(fmt.Sprintf("Failed '%s', retrying in %v (%d/%d)...", task.Title, backoffSec, retries, maxRetries), "WARN")
		} else {
			_ = o.taskRepo.UpdateStatus(task.TaskID, "failed", result.Error, "")
			onLog(fmt.Sprintf("FAILED '%s' after max retries", task.Title), "ERROR")
			go o.notifierSvc.NotifyTaskEvent("task_failed", task.Title, "failed", result.Error)
		}
		agent.Status = "idle"
		_ = o.agentRepo.SetStatus(agent.AgentID, "idle", "", result.Error)
	}

	o.emitBoard()
	o.emitAgents()
}

func (o *Orchestrator) markStopped(task *models.Task, agent *models.Agent) {
	_ = o.taskRepo.UpdateStatus(task.TaskID, "failed", "⏹️ Đã dừng bởi người dùng (Stopped)", "")
	agent.Status = "idle"
	_ = o.agentRepo.SetStatus(agent.AgentID, "idle", "", "")
	o.emitTaskLog(task.TaskID, fmt.Sprintf("⏹️ Task '%s' đã bị dừng bởi người dùng.", task.Title), "WARN")
	o.emitBoard()
	o.emitAgents()
}

// StopTask cancels an in-flight task, killing its CLI process. No-op if the task
// is not currently running.
func (o *Orchestrator) StopTask(taskID string) bool {
	o.runMu.Lock()
	cancel, ok := o.activeRuns[taskID]
	o.runMu.Unlock()
	if ok {
		cancel()
		o.emitLog(fmt.Sprintf("⏹️ Yêu cầu dừng task %s...", taskID), "WARN")
		return true
	}
	return false
}

// RetryTask resets a failed/done task back to the backlog so the orchestrator
// re-dispatches it on the next scan.
func (o *Orchestrator) RetryTask(taskID string) error {
	if err := o.taskRepo.ResetForRetry(taskID); err != nil {
		return err
	}
	o.emitLog(fmt.Sprintf("🔄 Task %s được đưa lại Backlog để chạy lại.", taskID), "INFO")
	o.emitBoard()
	return nil
}

var (
	urlRe    = regexp.MustCompile(`https?://[^\s"'` + "`" + `)\]]+`)
	expectRe = regexp.MustCompile(`(?i)\[EXPECT(?:\s+TEXT)?:\s*([^\]]+)\]`)
)

// buildE2EFixPrompt seeds a coding task with the concrete E2E failure detail so
// the agent can read the console errors / failed assertions and fix the code.
func buildE2EFixPrompt(res *services.BrowserActionResult) string {
	var b strings.Builder
	b.WriteString("📌 [ACTION]: Sửa lỗi khiến bài kiểm thử E2E trên trình duyệt thất bại.\n")
	b.WriteString(fmt.Sprintf("📑 [CONTEXT]: URL đã kiểm thử: %s (title: %s)\n", res.URL, res.Title))
	if len(res.ConsoleErrors) > 0 {
		b.WriteString("🐞 [CONSOLE ERRORS]:\n")
		for _, e := range res.ConsoleErrors {
			b.WriteString("- " + e + "\n")
		}
	}
	if len(res.Assertions) > 0 {
		b.WriteString("🎯 [ASSERTIONS]:\n")
		for _, a := range res.Assertions {
			b.WriteString("- " + a + "\n")
		}
	}
	b.WriteString("🎯 [EXPECTED OUTPUT]: Sửa trực tiếp các file trong workspace để loại bỏ mọi lỗi console và làm mọi assertion '❌' trở thành đạt. Không hỏi lại, tự sửa.")
	return b.String()
}

// extractTestURL pulls the first http(s) URL from the task text, defaulting to the
// local dev server when none is specified.
func extractTestURL(s string) string {
	if m := urlRe.FindString(s); m != "" {
		return strings.TrimRight(m, ".,")
	}
	return "http://localhost:5173"
}

// extractExpectedTexts reads explicit `[EXPECT: some text]` directives from a task
// prompt so E2E assertions never guess (avoiding false failures).
func extractExpectedTexts(s string) []string {
	var out []string
	for _, m := range expectRe.FindAllStringSubmatch(s, -1) {
		if len(m) > 1 {
			if v := strings.TrimSpace(m[1]); v != "" {
				out = append(out, v)
			}
		}
	}
	return out
}

// emitTaskScreenshot sends a base64 screenshot (data URL) for a task so the Task
// Inspector can display the E2E capture.
func (o *Orchestrator) emitTaskScreenshot(taskID, dataURL string) {
	if o.ctx != nil {
		runtime.EventsEmit(o.ctx, "task_screenshot", map[string]string{
			"task_id": taskID,
			"data":    dataURL,
		})
	}
}

func (o *Orchestrator) emitBoard() {
	o.mu.Lock()
	onBoard := o.onBoard
	o.mu.Unlock()
	if onBoard != nil {
		onBoard()
	}
	if o.ctx != nil {
		runtime.EventsEmit(o.ctx, "board_updated", nil)
	}
}

func (o *Orchestrator) emitAgents() {
	if o.ctx != nil {
		runtime.EventsEmit(o.ctx, "agent_updated", nil)
	}
}

func (o *Orchestrator) emitLog(msg, level string) {
	o.mu.Lock()
	onLog := o.onLog
	o.mu.Unlock()
	if onLog != nil {
		onLog(msg, level)
	}
	if o.ctx != nil {
		runtime.EventsEmit(o.ctx, "log_entry", map[string]string{
			"message": msg,
			"level":   level,
			"time":    time.Now().Format("15:04:05"),
		})
	}
}

func (o *Orchestrator) emitApproval(taskID, agentName, taskTitle string) {
	o.mu.Lock()
	onApproval := o.onApproval
	o.mu.Unlock()
	if onApproval != nil {
		onApproval(taskID, agentName, taskTitle)
	}
}

// emitTaskLog streams a log line scoped to a specific task (for the Task Inspector)
// and also mirrors it into the global log stream.
func (o *Orchestrator) emitTaskLog(taskID, msg, level string) {
	ts := time.Now().Format("15:04:05")
	// Frontends without Wails (the TUI) only see onLog, so task-scoped lines have
	// to travel that path too or agent output never reaches the terminal.
	o.mu.Lock()
	onLog := o.onLog
	o.mu.Unlock()
	if onLog != nil {
		onLog(msg, level)
	}
	if o.ctx != nil {
		runtime.EventsEmit(o.ctx, "task_log", map[string]string{
			"task_id": taskID,
			"message": msg,
			"level":   level,
			"time":    ts,
		})
		runtime.EventsEmit(o.ctx, "log_entry", map[string]string{
			"message": msg,
			"level":   level,
			"time":    ts,
		})
	}
}
