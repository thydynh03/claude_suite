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
	"claude_suite/backend/modelcatalog"
	"claude_suite/backend/models"
	"claude_suite/backend/services"
	"claude_suite/backend/services/projectmap"
	"claude_suite/backend/textutil"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// defaultMaxConcurrency is how many sub-agents may execute tasks in parallel.
const defaultMaxConcurrency = 3

// e2eTagRe matches every spelling runTask accepts as a browser-test tag. The
// auto-fix task's title is cleaned with this exact set — detection and
// stripping drifting apart is what made fix tasks re-run as browser tests.
var e2eTagRe = regexp.MustCompile(`(?i)\[(?:e2e|web ?test)\]`)

type Orchestrator struct {
	ctx            context.Context
	agentRepo      *database.AgentRepository
	taskRepo       *database.TaskRepository
	memoryRepo     *database.MemoryRepository
	attemptRepo    *database.AttemptRepository
	wsRepo         *database.WorkspaceRepository
	obsRepo        *database.ObservationRepository
	obsW           *obsWriter
	learner        *services.Learner
	mapper         *projectmap.Mapper
	modelCatalog   *modelcatalog.Catalog
	cliRunner      cli.CLIRunner
	contextMgr     *services.ContextManager
	gitService     *services.GitService
	browserSvc     *services.BrowserAgentService
	verifySvc      *services.VerifyService
	budget         *services.BudgetGuard
	notifierSvc    *services.NotifierService
	dispatcher     *AgentDispatcher
	fallback       *FallbackHandler
	workspaceDir   string
	verifyBuild    bool
	budgetNoticeAt float64
	packBudget     int
	sessionResume  bool

	running        bool
	autoApproveAll bool
	stopCh         chan struct{}
	// loopDone is closed by the scan loop as it exits; Stop waits on it so a
	// Stop-then-Start can never leave two loops scanning at once.
	loopDone chan struct{}
	// dispatchMu serialises dispatchAvailable: two interleaved scans both see
	// the same backlog task before either registers it, and the second
	// activeRuns write makes the first run unstoppable.
	dispatchMu     sync.Mutex
	mu             sync.Mutex
	onLog          func(message, level string)
	onApproval     func(taskID, agentName, taskTitle string)
	onBoard        func()

	approvalMu    sync.Mutex
	approvalChans map[string]chan bool // taskID -> approval channel (per-task)

	maxConcurrency int
	runMu          sync.Mutex
	activeRuns     map[string]context.CancelFunc // taskID -> cancel func for in-flight tasks
	// retryAt holds each failed task's earliest next attempt. In memory on
	// purpose: the backoff is seconds-scale, so surviving a restart buys
	// nothing, and a DB column would drag the Task model (and the Wails
	// bindings) along with it.
	retryAt map[string]time.Time
	// retryBackoff computes the wait before attempt retries+1. A field so
	// tests can zero it; production keeps the exponential default.
	retryBackoff func(retries int) time.Duration
	wg           sync.WaitGroup

	// wsIDCache memoizes workspace identity per directory: one git exec per
	// directory per app lifetime instead of one per captured event.
	wsIDMu    sync.Mutex
	wsIDCache map[string]string
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
		budget:         services.NewBudgetGuard(),
		notifierSvc:    services.NewNotifierService(),
		verifyBuild:    true,
		dispatcher:     NewAgentDispatcher(),
		fallback:       NewFallbackHandler(),
		approvalChans:  make(map[string]chan bool),
		maxConcurrency: defaultMaxConcurrency,
		packBudget:     services.DefaultPackBudget,
		activeRuns:     make(map[string]context.CancelFunc),
		retryAt:        make(map[string]time.Time),
		wsIDCache:      make(map[string]string),
		retryBackoff: func(retries int) time.Duration {
			return time.Duration(1<<uint(retries)) * time.Second
		},
	}
}

func (o *Orchestrator) SetContext(ctx context.Context) {
	o.ctx = ctx
}

// SetAttemptRepo enables per-attempt failure recording. A setter rather than a
// NewOrchestrator parameter so existing construction sites and tests keep
// compiling; when unset, attempts simply are not recorded.
func (o *Orchestrator) SetAttemptRepo(r *database.AttemptRepository) {
	o.attemptRepo = r
}

// SetMemoryStores enables workspace identity and observation capture. Both are
// optional the same way SetAttemptRepo is: unset means nothing is recorded.
func (o *Orchestrator) SetMemoryStores(ws *database.WorkspaceRepository, obs *database.ObservationRepository) {
	o.wsRepo = ws
	o.obsRepo = obs
	o.obsW = newObsWriter(obs)
}

// SetLearner connects the memory learner: per-task feedback, mechanical
// lessons, regression recording, and distillation nudges.
func (o *Orchestrator) SetLearner(l *services.Learner) {
	o.learner = l
}

// SetModelCatalog lets runs teach the catalog which model ids actually exist —
// the CLI reports the real model per run, including ids newer than the app.
func (o *Orchestrator) SetModelCatalog(c *modelcatalog.Catalog) {
	o.modelCatalog = c
}

// SetProjectMapper enables the post-task freshness hook: after each run, the
// task's own changed files are re-fingerprinted so the map stays at most one
// task behind the workspace, at zero token cost.
func (o *Orchestrator) SetProjectMapper(m *projectmap.Mapper) {
	o.mapper = m
}

// SetPackBudget bounds the injected context pack in characters. Zero disables
// injection entirely — the kill switch for the whole memory feature.
func (o *Orchestrator) SetPackBudget(chars int) {
	o.mu.Lock()
	o.packBudget = chars
	o.mu.Unlock()
}

func (o *Orchestrator) GetPackBudget() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.packBudget
}

// SetSessionResume controls whether a successful run's SessionID is written
// back to the agent row, activating --resume/--conversation on its next task.
// Off by default: long-lived sessions grow context the CLI compacts
// unpredictably, so this is an explicit opt-in in Settings.
func (o *Orchestrator) SetSessionResume(enabled bool) {
	o.mu.Lock()
	o.sessionResume = enabled
	o.mu.Unlock()
}

func (o *Orchestrator) GetSessionResume() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.sessionResume
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

// Start begins scanning the backlog and reports whether this call is what
// started it. A second press used to return silently and the UI announced a
// fresh start either way, so "nothing happened" and "already running" looked
// identical to the user.
func (o *Orchestrator) Start() bool {
	o.mu.Lock()
	if o.running {
		o.mu.Unlock()
		return false
	}
	o.running = true
	o.stopCh = make(chan struct{})
	o.loopDone = make(chan struct{})
	// Hand this run's channels to the loop rather than letting it read the
	// fields. A later Start replaces the fields, and an old loop still reading
	// them is a data race — one the race detector catches on the
	// Stop-then-Start path a user takes with two button presses.
	stop := o.stopCh
	done := o.loopDone
	o.mu.Unlock()

	o.emitLog(fmt.Sprintf("🚀 Orchestrator ACTIVE: Quét Backlog & phân công song song (tối đa %d agent).", o.GetMaxConcurrency()), "INFO")
	go o.loop(stop, done)
	return true
}

// stopChannel returns the channel for the current run under the lock. A caller
// holds on to what it is given: a task that started under one run should keep
// watching that run's channel even if the orchestrator is started again.
func (o *Orchestrator) stopChannel() <-chan struct{} {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.stopCh
}

// Stop halts backlog scanning and reports whether this call is what stopped it.
//
// It waits for the scan loop to actually exit. Returning while a scan is
// mid-flight let the next Start run a second loop beside it: both picked the
// same backlog task, and the activeRuns overwrite left one paid run
// unstoppable and invisible to slot accounting.
func (o *Orchestrator) Stop() bool {
	o.mu.Lock()
	if !o.running {
		o.mu.Unlock()
		return false
	}
	o.running = false
	close(o.stopCh)
	done := o.loopDone
	o.mu.Unlock()

	if done != nil {
		<-done
	}

	o.emitLog("🛑 Orchestrator INACTIVE: Đã tạm dừng quét Backlog (task đang chạy vẫn tiếp tục).", "WARN")
	return true
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

func (o *Orchestrator) loop(stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			// A closed stop channel and a pending tick are both ready, and
			// select picks randomly — without this check a stopped loop can
			// run one more whole scan half the time.
			select {
			case <-stop:
				return
			default:
			}
			o.dispatchAvailable()
		}
	}
}

// persistTask reports a failed write of a task's own state instead of discarding
// it. These are the writes that decide what happens next: a lost "done" leaves
// the row running and the work is never picked up again, and a lost "backlog"
// costs the retry.
//
// Every one of them used to be `_ =`, which is how SQLITE_BUSY stayed invisible
// while the board quietly stopped matching what the agents were doing. Agent
// rows are deliberately not routed through here — a stale status or task count
// is cosmetic, and a warning per agent write would bury these.
func (o *Orchestrator) persistTask(taskID, what string, err error) {
	if err == nil {
		return
	}
	o.emitTaskLog(taskID, fmt.Sprintf("⚠️ Không ghi được %s vào cơ sở dữ liệu: %v", what, err), "ERROR")
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
	// One scan at a time, whoever calls: the pick→register sequence below is
	// not atomic, and two interleaved scans dispatch the same task twice.
	o.dispatchMu.Lock()
	defer o.dispatchMu.Unlock()

	// Nothing new starts once the day's ceiling is reached. Work already running
	// is left to finish: killing an agent mid-edit would leave the workspace in a
	// worse state than the cost of letting it end.
	if allowed, spent, limit := o.budget.Allow(); !allowed {
		o.reportBudgetStop(spent, limit)
		return
	}

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
		if notBefore, backing := o.retryAt[task.TaskID]; backing {
			if time.Now().Before(notBefore) {
				// Its backoff has not elapsed; let this scan tick pass. The
				// wait is seconds, so anything queued behind it is delayed at
				// most that long.
				o.runMu.Unlock()
				return
			}
			delete(o.retryAt, task.TaskID)
		}
		o.runMu.Unlock()

		agent := o.prepareAgentForTask(task)
		if agent == nil {
			return
		}

		// Mark running synchronously so the next NextDispatchable won't repick it.
		o.persistTask(task.TaskID, "phân công agent", o.taskRepo.AssignTask(task.TaskID, agent.Name))
		o.persistTask(task.TaskID, "trạng thái task", o.taskRepo.UpdateStatus(task.TaskID, "running", "", ""))
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
			o.persistTask(task.TaskID, "trạng thái task", o.taskRepo.UpdateStatus(task.TaskID, "failed", fmt.Sprintf("panic: %v", r), ""))
			if agent != nil {
				agent.Status = "idle"
				_ = o.agentRepo.SetStatus(agent.AgentID, "idle", "", fmt.Sprintf("panic: %v", r))
			}
			o.emitBoard()
			o.emitAgents()
		}
	}()

	// Per-task log helper: streams to both the task inspector and the global
	// log, and captures tool events as observations. taskWsID is assigned once
	// the workspace is snapshotted below; before that (E2E pre-checks) capture
	// is silently off. The runner's streaming goroutines start after the
	// assignment, so they observe it.
	taskWsID := ""
	onLog := func(msg, level string) {
		o.emitTaskLog(task.TaskID, fmt.Sprintf("[%s] %s", agent.Name, msg), level)
		if level == "TOOL" && taskWsID != "" {
			o.observe(taskWsID, task.TaskID, agent.AgentID, "tool_call", "", msg)
		}
	}

	// Automated E2E Web Test Skill Auto-Binding — only for explicitly tagged tasks.
	// Kept in lockstep with e2eTagRe: what this detects, the auto-fix title must strip.
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
			o.persistTask(task.TaskID, "phân công agent", o.taskRepo.AssignTask(task.TaskID, "Web E2E Tester (Chrome CDP)"))
			for _, a := range bRes.Assertions {
				onLog("   "+a, "INFO")
			}
			summary := fmt.Sprintf("URL: %s\nTitle: %s\nAssertions:\n%s", bRes.URL, bRes.Title, strings.Join(bRes.Assertions, "\n"))
			if len(bRes.ConsoleErrors) > 0 {
				summary += "\nConsole errors:\n- " + strings.Join(bRes.ConsoleErrors, "\n- ")
			}
			if bRes.Passed {
				o.persistTask(task.TaskID, "trạng thái task", o.taskRepo.UpdateStatus(task.TaskID, "done", "✅ E2E PASSED\n"+summary, "chrome-cdp-auto-session"))
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
					// The E2E failure is this task's attempt history — recorded
					// so the eventual fixed-cycle regression has its symptom.
					if o.attemptRepo != nil {
						_ = o.attemptRepo.Add(task.TaskID, textutil.Truncate("E2E FAILED\n"+summary, 4000, "…"))
					}
					// Strip every spelling the detection above accepts, not just
					// the uppercase prefix: a surviving "[webtest]" tag makes the
					// fix task run as another browser test, which fails the same
					// way and spawns its own fix task — an unbounded chain of
					// Chrome runs with no coding agent ever invoked.
					// ParentID links the fix back to the failing E2E task — it is
					// what lets the learner recognize the fixed cycle and record
					// the regression.
					fix := &models.Task{
						Title:    "[CODE] Auto-fix lỗi E2E: " + strings.TrimSpace(e2eTagRe.ReplaceAllString(task.Title, "")),
						Priority: "high",
						Status:   "backlog",
						Prompt:   buildE2EFixPrompt(bRes),
						ParentID: task.TaskID,
					}
					if err := o.taskRepo.Create(fix); err == nil {
						o.persistTask(task.TaskID, "đưa task về hàng đợi", o.taskRepo.RequeueWithDependency(task.TaskID, fix.TaskID))
						onLog(fmt.Sprintf("🔁 E2E FAILED → tạo task tự sửa '%s'. Sẽ chạy lại E2E sau khi sửa (vòng %d/%d).", fix.Title, task.RetryCount+1, maxFix), "WARN")
					} else {
						o.persistTask(task.TaskID, "trạng thái task", o.taskRepo.UpdateStatus(task.TaskID, "failed", "❌ E2E FAILED\n"+summary, "chrome-cdp-auto-session"))
						onLog("❌ Không tạo được task sửa lỗi, đánh dấu E2E failed.", "ERROR")
					}
				} else {
					o.persistTask(task.TaskID, "trạng thái task", o.taskRepo.UpdateStatus(task.TaskID, "failed", "❌ E2E FAILED (hết số vòng tự sửa)\n"+summary, "chrome-cdp-auto-session"))
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
				o.persistTask(task.TaskID, "trạng thái task", o.taskRepo.UpdateStatus(task.TaskID, "failed", "Rejected by user", ""))
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
			o.persistTask(task.TaskID, "trạng thái task", o.taskRepo.UpdateStatus(task.TaskID, "backlog", "", ""))
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

	// Workspace identity + task lifecycle capture.
	taskWsID = o.resolveWorkspaceID(workspaceDir)
	o.observe(taskWsID, task.TaskID, agent.AgentID, "task_start", "", task.Title)

	// Context pack: project map + lessons + known fixed bugs + what this
	// task's prerequisites produced + what its own earlier attempts failed
	// with. Prepended to the USER prompt because that is the only channel that
	// reaches both providers identically — Antigravity has no separate system
	// channel. The exact injected text is emitted on task_log and stored as a
	// pack_injected observation, so any task's inspector shows precisely what
	// memory it received.
	packLen := 0
	pack := o.contextMgr.BuildTaskPack(workspaceDir, taskWsID, task, o.GetPackBudget())
	if pack.Text != "" {
		fullPrompt = pack.Text + "\n\n" + fullPrompt
		packLen = len(pack.Text)
		o.emitTaskLog(task.TaskID, fmt.Sprintf("📦 Context pack (%d ký tự):\n%s", packLen, pack.Text), "INFO")
		o.observe(taskWsID, task.TaskID, agent.AgentID, "pack_injected", "", pack.Text)
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
	adjustEstimatedUsage(result, packLen)

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
	// Count the run against the day's budget. Only the Claude runner reports a
	// real figure, from the CLI's own usage event; a run that reports nothing
	// adds nothing, so the ceiling guards what it can measure rather than
	// guessing.
	o.budget.Record(result.CostUSD)

	// Smart fallback on quota exhaustion.
	if !result.Success && o.fallback.IsQuotaExhausted(agent, result.Error) {
		newProvider, newModel := o.fallback.GetFallbackModel(agent, result.AccountID)
		agent.Provider = newProvider
		agent.Model = newModel
		_ = o.agentRepo.SetProviderModel(agent.AgentID, newProvider, newModel)
		onLog(fmt.Sprintf("⚠️ Smart Fallback: Switched to %s (%s)", newProvider, newModel), "WARN")
		result = o.cliRunner.RunAgentCtx(ctx, agent, fullPrompt, onLog, workspaceDir)
		adjustEstimatedUsage(result, packLen)
		if ctx.Err() != nil {
			o.markStopped(task, agent)
			return
		}
	}

	// Surface the model the CLI REPORTED running — a measured fact the
	// configured model cannot promise (aliases resolve, CLIs fall back). New
	// ids feed the catalog, which is how the app learns about models that
	// shipped after this build.
	if result.ModelUsed != "" {
		if result.ModelUsed != runAgent.Model {
			onLog(fmt.Sprintf("🤖 Model thực tế: %s (cấu hình: %s)", result.ModelUsed, orDash(runAgent.Model)), "INFO")
		} else {
			onLog("🤖 Model thực tế: "+result.ModelUsed, "INFO")
		}
		if o.modelCatalog != nil {
			o.modelCatalog.RecordObserved(result.ModelUsed)
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
			o.observe(taskWsID, task.TaskID, agent.AgentID, "build_verify_failed", "", vr.Report)
			onLog("❌ Build verify FAILED — trả task về để agent sửa.", "ERROR")
		} else if vr.Ran {
			onLog("✅ Build verify passed.", "SUCCESS")
		}
	}

	if result.Success {
		o.persistTask(task.TaskID, "trạng thái task", o.taskRepo.UpdateStatus(task.TaskID, "done", result.Output, result.SessionID))
		agent.Status = "idle"
		_ = o.agentRepo.CompleteTask(agent.AgentID)
		// Session resume (opt-in): the run's session becomes the agent's,
		// so its next dispatch continues the same CLI conversation.
		if o.GetSessionResume() && result.SessionID != "" && result.SessionID != agent.SessionID {
			_ = o.agentRepo.SetSessionID(agent.AgentID, result.SessionID)
		}
		costStr := ""
		if result.CostUSD > 0 {
			costStr = fmt.Sprintf(" · $%.4f", result.CostUSD)
		}
		o.observe(taskWsID, task.TaskID, agent.AgentID, "task_complete", "", result.Output)
		onLog(fmt.Sprintf("DONE '%s' (%.1fs · %d tokens%s)", task.Title, result.DurationSec, result.TokensUsed, costStr), "SUCCESS")
		go o.notifierSvc.NotifyTaskEvent("task_done", task.Title, "done", fmt.Sprintf("Agent %s hoàn thành trong %.1fs", agent.Name, result.DurationSec))
	} else {
		// Keep this attempt's error text. Until it was recorded, every retry
		// re-ran a byte-identical prompt with no idea what went wrong before.
		if o.attemptRepo != nil {
			_ = o.attemptRepo.Add(task.TaskID, textutil.Truncate(result.Error, 4000, "…"))
		}
		o.observe(taskWsID, task.TaskID, agent.AgentID, "task_failed", "", result.Error)
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
			backoffSec := o.retryBackoff(retries)
			// Record when the retry may actually run. The log below used to
			// be the only place this duration existed — nothing waited, so
			// every paid retry fired on the very next 1.5s scan, burning the
			// whole retry budget before a transient error could clear.
			o.runMu.Lock()
			o.retryAt[task.TaskID] = time.Now().Add(backoffSec)
			o.runMu.Unlock()
			o.persistTask(task.TaskID, "trạng thái task", o.taskRepo.UpdateStatus(task.TaskID, "backlog", "", ""))
			onLog(fmt.Sprintf("Failed '%s', retrying in %v (%d/%d)...", task.Title, backoffSec, retries, maxRetries), "WARN")
		} else {
			o.persistTask(task.TaskID, "trạng thái task", o.taskRepo.UpdateStatus(task.TaskID, "failed", result.Error, ""))
			onLog(fmt.Sprintf("FAILED '%s' after max retries", task.Title), "ERROR")
			go o.notifierSvc.NotifyTaskEvent("task_failed", task.Title, "failed", result.Error)
		}
		agent.Status = "idle"
		_ = o.agentRepo.SetStatus(agent.AgentID, "idle", "", result.Error)
	}

	// Learner hook: feedback for the lessons this task carried, mechanical
	// lesson + regression extraction on fixed-after-failing tasks, and a
	// distillation nudge. Synchronous — a few local DB writes.
	if o.learner != nil {
		o.learner.OnTaskFinished(taskWsID, workspaceDir, task, result, pack.LessonIDs)
	}

	// Freshness hook: whatever this run changed (AutoSnapshot committed before
	// it, so the uncommitted diff IS this task's edits — including a failed
	// run's half-done edits) gets re-fingerprinted in the background. Tracked
	// on the WaitGroup so tests and shutdown can wait for it.
	if workspaceDir != "" && o.mapper != nil {
		o.wg.Add(1)
		go func() {
			defer o.wg.Done()
			_ = o.mapper.IncrementalUpdate(workspaceDir, o.gitService.ChangedFiles(workspaceDir))
		}()
	}

	o.emitBoard()
	o.emitAgents()
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// adjustEstimatedUsage subtracts the injected pack from a len/4-estimated
// token count. The pack is the orchestrator's own text; counting it against
// the agent made estimated providers (Antigravity reports no usage) reach
// their quota ceiling on fiction and trip fallback early.
func adjustEstimatedUsage(r *cli.RunResult, packLen int) {
	if r == nil || !r.UsageEstimated || packLen <= 0 {
		return
	}
	cut := int64(packLen / 4)
	if r.TokensUsed > cut {
		r.TokensUsed -= cut
	}
}

func (o *Orchestrator) markStopped(task *models.Task, agent *models.Agent) {
	o.persistTask(task.TaskID, "trạng thái task", o.taskRepo.UpdateStatus(task.TaskID, "failed", "⏹️ Đã dừng bởi người dùng (Stopped)", ""))
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

// NotifyDigest sends a summary through the same outbound webhook task events
// use, so the daily digest arrives wherever the user already gets notifications.
func (o *Orchestrator) NotifyDigest(summary string) {
	go o.notifierSvc.NotifyTaskEvent("digest", "Tổng kết hằng ngày", "ok", summary)
}

// SetDailyBudgetUSD caps what may be spent in a day. Zero or less removes the cap.
func (o *Orchestrator) SetDailyBudgetUSD(limit float64) {
	o.budget.SetLimit(limit)
	if limit > 0 {
		o.emitLog(fmt.Sprintf("💰 Trần chi phí mỗi ngày: $%.2f", limit), "INFO")
	} else {
		o.emitLog("💰 Đã bỏ trần chi phí mỗi ngày.", "WARN")
	}
}

// BudgetSnapshot reports the day, what has been spent, and the ceiling.
func (o *Orchestrator) BudgetSnapshot() (string, float64, float64) {
	return o.budget.Snapshot()
}

// UseBudgetStore keeps the running total on disk, so restarting the app does not
// hand the agents a fresh budget.
func (o *Orchestrator) UseBudgetStore(path string) error {
	return o.budget.UseStore(path)
}

// reportBudgetStop says why nothing is being dispatched, but only when the
// number changes — the scan runs every 1.5 seconds and would otherwise fill the
// log with the same line.
func (o *Orchestrator) reportBudgetStop(spent, limit float64) {
	o.mu.Lock()
	already := o.budgetNoticeAt == spent
	o.budgetNoticeAt = spent
	o.mu.Unlock()
	if already {
		return
	}
	o.emitLog(fmt.Sprintf(
		"🛑 Đã dùng $%.2f trong ngày, chạm trần $%.2f — tạm ngưng giao việc mới. Task đang chạy vẫn tiếp tục.",
		spent, limit,
	), "WARN")
}
