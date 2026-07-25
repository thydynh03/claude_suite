package orchestrator

import (
	"context"
	"fmt"
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
	dispatcher   *AgentDispatcher
	fallback     *FallbackHandler
	workspaceDir string

	running        bool
	autoApproveAll bool
	stopCh         chan struct{}
	mu             sync.Mutex

	approvalMu    sync.Mutex
	approvalChans map[string]chan bool // taskID -> approval channel (per-task)

	maxConcurrency int
	runMu          sync.Mutex
	activeRuns     map[string]context.CancelFunc // taskID -> cancel func for in-flight tasks
	wg             sync.WaitGroup
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

func (o *Orchestrator) Start() {
	o.mu.Lock()
	if o.running {
		o.mu.Unlock()
		return
	}
	o.running = true
	o.stopCh = make(chan struct{})
	o.mu.Unlock()

	o.emitLog(fmt.Sprintf("🚀 Orchestrator ACTIVE: Quét Backlog & phân công song song (tối đa %d agent).", o.GetMaxConcurrency()), "INFO")
	go o.loop()
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

func (o *Orchestrator) loop() {
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-o.stopCh:
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
		_ = o.agentRepo.Update(agent)
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
		testUrl := "http://localhost:5173"
		onLog(fmt.Sprintf("🌐 Chrome CDP E2E Skill: đang mở & kiểm thử %s...", testUrl), "INFO")
		bRes, bErr := o.browserSvc.RunBrowserTask(testUrl, true)
		if bErr == nil && bRes != nil && bRes.Success {
			outputSummary := fmt.Sprintf("✅ Chrome CDP E2E Test Passed!\nURL: %s\nTitle: %s\nText Snippet: %s\nScreenshot Captured: Yes", bRes.URL, bRes.Title, bRes.TextContent)
			_ = o.taskRepo.AssignTask(task.TaskID, "Web E2E Tester (Chrome CDP)")
			_ = o.taskRepo.UpdateStatus(task.TaskID, "done", outputSummary, "chrome-cdp-auto-session")
			agent.Status = "idle"
			agent.TasksDone++
			_ = o.agentRepo.Update(agent)
			onLog(fmt.Sprintf("🎉 Chrome CDP hoàn thành E2E '%s'.", task.Title), "SUCCESS")
			o.emitBoard()
			return
		}
		onLog(fmt.Sprintf("⚠️ Chrome CDP Auto-Test warning: %v. Chuyển cho Agent thử lại...", bErr), "WARN")
	}

	// Approval checkpoint (BA / Architect roles, unless auto-approve-all).
	nameLower := strings.ToLower(agent.Name)
	if !o.GetAutoApproveAll() && (strings.Contains(nameLower, "architect") || strings.Contains(nameLower, "(ba)")) {
		onLog("Waiting for user approval...", "WARN")

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
		select {
		case approved := <-approvalCh:
			if !approved {
				_ = o.taskRepo.UpdateStatus(task.TaskID, "failed", "Rejected by user", "")
				onLog(fmt.Sprintf("Task '%s' rejected by user.", task.Title), "ERROR")
				agent.Status = "idle"
				_ = o.agentRepo.Update(agent)
				o.emitBoard()
				return
			}
			onLog(fmt.Sprintf("Task '%s' approved.", task.Title), "SUCCESS")
		case <-ctx.Done():
			o.markStopped(task, agent)
			return
		case <-o.stopCh:
			// Orchestrator paused; leave task running to be handled next start.
			return
		}
	}

	onLog(fmt.Sprintf("Executing task: '%s'", task.Title), "SEND")

	if o.workspaceDir != "" {
		_ = o.gitService.AutoSnapshot(o.workspaceDir)
	}

	fullPrompt := task.Prompt
	if fullPrompt == "" {
		fullPrompt = task.Description
	}
	if o.workspaceDir != "" {
		fullPrompt = fmt.Sprintf("[DIRECTIVE: CREATE OR MODIFY FILES DIRECTLY IN WORKSPACE %s]\n\n%s", o.workspaceDir, fullPrompt)
	}

	result := o.cliRunner.RunAgentCtx(ctx, agent, fullPrompt, onLog, o.workspaceDir)

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
		_ = o.agentRepo.Update(agent)
		onLog(fmt.Sprintf("⚠️ Smart Fallback: Switched to %s (%s)", newProvider, newModel), "WARN")
		result = o.cliRunner.RunAgentCtx(ctx, agent, fullPrompt, onLog, o.workspaceDir)
		if ctx.Err() != nil {
			o.markStopped(task, agent)
			return
		}
	}

	if result.Success {
		_ = o.taskRepo.UpdateStatus(task.TaskID, "done", result.Output, result.SessionID)
		agent.Status = "idle"
		agent.TasksDone++
		_ = o.agentRepo.Update(agent)
		costStr := ""
		if result.CostUSD > 0 {
			costStr = fmt.Sprintf(" · $%.4f", result.CostUSD)
		}
		onLog(fmt.Sprintf("DONE '%s' (%.1fs · %d tokens%s)", task.Title, result.DurationSec, result.TokensUsed, costStr), "SUCCESS")
	} else {
		task.RetryCount++
		if task.RetryCount < task.MaxRetries {
			backoffSec := time.Duration(1<<uint(task.RetryCount)) * time.Second
			_ = o.taskRepo.UpdateStatus(task.TaskID, "backlog", "", "")
			onLog(fmt.Sprintf("Failed '%s', retrying in %v (%d/%d)...", task.Title, backoffSec, task.RetryCount, task.MaxRetries), "WARN")
		} else {
			_ = o.taskRepo.UpdateStatus(task.TaskID, "failed", result.Error, "")
			onLog(fmt.Sprintf("FAILED '%s' after max retries", task.Title), "ERROR")
		}
		agent.Status = "idle"
		agent.LastError = result.Error
		_ = o.agentRepo.Update(agent)
	}

	o.emitBoard()
	o.emitAgents()
}

func (o *Orchestrator) markStopped(task *models.Task, agent *models.Agent) {
	_ = o.taskRepo.UpdateStatus(task.TaskID, "failed", "⏹️ Đã dừng bởi người dùng (Stopped)", "")
	agent.Status = "idle"
	_ = o.agentRepo.Update(agent)
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

func (o *Orchestrator) emitBoard() {
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
	if o.ctx != nil {
		runtime.EventsEmit(o.ctx, "log_entry", map[string]string{
			"message": msg,
			"level":   level,
			"time":    time.Now().Format("15:04:05"),
		})
	}
}

// emitTaskLog streams a log line scoped to a specific task (for the Task Inspector)
// and also mirrors it into the global log stream.
func (o *Orchestrator) emitTaskLog(taskID, msg, level string) {
	ts := time.Now().Format("15:04:05")
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
