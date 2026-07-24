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

type Orchestrator struct {
	ctx          context.Context
	agentRepo    *database.AgentRepository
	taskRepo     *database.TaskRepository
	memoryRepo   *database.MemoryRepository
	cliRunner    cli.CLIRunner
	contextMgr   *services.ContextManager
	gitService   *services.GitService
	dispatcher   *AgentDispatcher
	fallback     *FallbackHandler
	workspaceDir string

	running        bool
	autoApproveAll bool
	stopCh         chan struct{}
	approvalCh     chan bool
	mu             sync.Mutex
}

func NewOrchestrator(
	agentRepo *database.AgentRepository,
	taskRepo *database.TaskRepository,
	memoryRepo *database.MemoryRepository,
	cliRunner cli.CLIRunner,
	ctxMgr *services.ContextManager,
	gitSvc *services.GitService,
) *Orchestrator {
	return &Orchestrator{
		agentRepo:  agentRepo,
		taskRepo:   taskRepo,
		memoryRepo: memoryRepo,
		cliRunner:  cliRunner,
		contextMgr: ctxMgr,
		gitService: gitSvc,
		dispatcher: NewAgentDispatcher(),
		fallback:   NewFallbackHandler(),
		approvalCh: make(chan bool),
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

func (o *Orchestrator) Start() {
	o.mu.Lock()
	if o.running {
		o.mu.Unlock()
		return
	}
	o.running = true
	o.stopCh = make(chan struct{})
	o.mu.Unlock()

	o.emitLog("🚀 Orchestrator ACTIVE: Đã bật chế độ tự động đọc Backlog & Phân công Task.", "INFO")
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

	o.emitLog("🛑 Orchestrator INACTIVE: Đã tạm dừng đọc Backlog.", "WARN")
}

func (o *Orchestrator) IsRunning() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.running
}

func (o *Orchestrator) ResolveApproval(approved bool) {
	select {
	case o.approvalCh <- approved:
	default:
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
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-o.stopCh:
			return
		case <-ticker.C:
			o.processNextTask()
		}
	}
}

func (o *Orchestrator) processNextTask() {
	task, err := o.taskRepo.NextDispatchable()
	if err != nil || task == nil {
		return
	}

	o.emitLog(fmt.Sprintf("📋 Quét Backlog: Tìm thấy Task '%s'. Đang phân tích Agent phù hợp...", task.Title), "INFO")

	agents, err := o.agentRepo.GetAll()
	if err != nil || len(agents) == 0 {
		o.emitLog("⚠️ Không tìm thấy Agent trong Database. Đang tự động tạo 8 vị trí Agent mặc định...", "WARN")
		_ = o.agentRepo.ResetToDefaults()
		agents, _ = o.agentRepo.GetAll()
		if len(agents) > 0 {
			o.emitLog("✨ Đã tạo 8 danh mục Agent thành công!", "SUCCESS")
		}
	}

	agent := o.dispatcher.FindMatchingAgent(task, agents)
	if agent == nil {
		o.emitLog(fmt.Sprintf("⚠️ Không tìm thấy Agent phù hợp cho task '%s'", task.Title), "WARN")
		return
	}

	if agent.AgentID == "" {
		_ = o.agentRepo.Create(agent)
		o.emitLog(fmt.Sprintf("✨ Tự động tạo Sub-Agent mới: '%s' (%s - Model: %s)", agent.Name, agent.Role, agent.Model), "SUCCESS")
	}

	o.emitLog(fmt.Sprintf("🤖 Phân công Task '%s' cho Agent '%s' (%s - %s)", task.Title, agent.Name, agent.Provider, agent.Model), "INFO")

	// Assign agent & mark task running
	_ = o.taskRepo.AssignTask(task.TaskID, agent.Name)
	_ = o.taskRepo.UpdateStatus(task.TaskID, "running", "", "")
	agent.Status = "running"
	agent.LastTask = task.Title
	_ = o.agentRepo.Update(agent)

	if o.ctx != nil {
		runtime.EventsEmit(o.ctx, "board_updated", nil)
	}

	// Approval Checkpoint
	nameLower := strings.ToLower(agent.Name)
	if !o.autoApproveAll && (strings.Contains(nameLower, "architect") || strings.Contains(nameLower, "ba")) {
		o.emitLog(fmt.Sprintf("[%s] Waiting for user approval...", agent.Name), "WARN")
		if o.ctx != nil {
			runtime.EventsEmit(o.ctx, "ask_approval", map[string]string{
				"agentName": agent.Name,
				"taskTitle": task.Title,
			})
		}

		select {
		case approved := <-o.approvalCh:
			if !approved {
				_ = o.taskRepo.UpdateStatus(task.TaskID, "failed", "Rejected by user", "")
				o.emitLog(fmt.Sprintf("[%s] Task '%s' rejected by user.", agent.Name, task.Title), "ERROR")
				agent.Status = "idle"
				_ = o.agentRepo.Update(agent)
				if o.ctx != nil {
					runtime.EventsEmit(o.ctx, "board_updated", nil)
				}
				return
			}
			o.emitLog(fmt.Sprintf("[%s] Task '%s' approved.", agent.Name, task.Title), "SUCCESS")
		case <-o.stopCh:
			return
		}
	}

	o.emitLog(fmt.Sprintf("[%s] Executing task: '%s'", agent.Name, task.Title), "SEND")

	// Git auto-snapshot
	if o.workspaceDir != "" {
		_ = o.gitService.AutoSnapshot(o.workspaceDir)
	}

	// Build full prompt with workspace directive
	fullPrompt := task.Prompt
	if fullPrompt == "" {
		fullPrompt = task.Description
	}
	if o.workspaceDir != "" {
		fullPrompt = fmt.Sprintf("[DIRECTIVE: CREATE OR MODIFY FILES DIRECTLY IN WORKSPACE %s]\n\n%s", o.workspaceDir, fullPrompt)
	}

	onLog := func(msg, level string) {
		o.emitLog(fmt.Sprintf("  [%s] %s", agent.Name, msg), level)
	}

	// Run task
	result := o.cliRunner.RunAgent(agent, fullPrompt, onLog, o.workspaceDir)

	// Save memory
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

	// Fallback check
	if !result.Success && o.fallback.IsQuotaExhausted(agent, result.Error) {
		newProvider, newModel := o.fallback.GetFallbackModel(agent)
		agent.Provider = newProvider
		agent.Model = newModel
		_ = o.agentRepo.Update(agent)
		o.emitLog(fmt.Sprintf("⚠️ Smart Fallback: Switched %s to %s (%s)", agent.Name, newProvider, newModel), "WARN")

		// Retry with fallback
		result = o.cliRunner.RunAgent(agent, fullPrompt, onLog, o.workspaceDir)
	}

	if result.Success {
		_ = o.taskRepo.UpdateStatus(task.TaskID, "done", result.Output, result.SessionID)
		agent.Status = "idle"
		agent.TasksDone++
		_ = o.agentRepo.Update(agent)
		o.emitLog(fmt.Sprintf("[%s] DONE '%s' (%.1fs)", agent.Name, task.Title, result.DurationSec), "SUCCESS")
	} else {
		task.RetryCount++
		if task.RetryCount < task.MaxRetries {
			_ = o.taskRepo.UpdateStatus(task.TaskID, "backlog", "", "")
			o.emitLog(fmt.Sprintf("[%s] Failed '%s', retrying (%d/%d)", agent.Name, task.Title, task.RetryCount, task.MaxRetries), "WARN")
		} else {
			_ = o.taskRepo.UpdateStatus(task.TaskID, "failed", result.Error, "")
			o.emitLog(fmt.Sprintf("[%s] FAILED '%s' after max retries", agent.Name, task.Title), "ERROR")
		}
		agent.Status = "idle"
		agent.LastError = result.Error
		_ = o.agentRepo.Update(agent)
	}

	// Notify Wails frontend to refresh board
	if o.ctx != nil {
		runtime.EventsEmit(o.ctx, "board_updated", nil)
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
