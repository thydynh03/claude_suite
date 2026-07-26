package orchestrator

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"claude_suite/backend/cli"
	"claude_suite/backend/database"
	"claude_suite/backend/models"
	"claude_suite/backend/services"
	"claude_suite/backend/testsupport"
)

// waitForIdle waits until no run is in flight. Checking the task's status alone
// is not enough: runTask writes the row before its goroutine removes itself from
// activeRuns, and a dispatch during that window is skipped as already busy.
func waitForIdle(t *testing.T, o *Orchestrator) {
	t.Helper()
	waitFor(t, "the run to be released", func() bool {
		o.runMu.Lock()
		defer o.runMu.Unlock()
		return len(o.activeRuns) == 0
	})
}

func taskStatus(t *testing.T, o *Orchestrator, taskID string) string {
	t.Helper()
	tasks, err := o.taskRepo.GetAll()
	if err != nil {
		t.Fatalf("read tasks: %v", err)
	}
	for _, task := range tasks {
		if task.TaskID == taskID {
			return task.Status
		}
	}
	t.Fatalf("task %s is gone", taskID)
	return ""
}

// Start owns the scan loop, so a seeded task has to run without a test poking
// dispatchAvailable itself.
func TestStartRunsTheScanLoop(t *testing.T) {
	o, taskRepo, agentRepo, runner := newTestOrchestrator(t)
	agent := seedAgent(t, agentRepo, "Runner")
	task := seedTask(t, taskRepo, "ship it", agent, nil)

	if o.IsRunning() {
		t.Fatal("a fresh orchestrator reports itself running")
	}

	o.Start()
	defer o.Stop()

	if !o.IsRunning() {
		t.Error("IsRunning() is false right after Start()")
	}
	waitFor(t, "the task to be dispatched by the loop", func() bool {
		return runner.CallCount() == 1
	})
	waitFor(t, "the task to finish", func() bool {
		return taskStatus(t, o, task.TaskID) == "done"
	})
}

// Start is wired to a button, so a second press must not leave two loops
// scanning the same backlog.
func TestStartTwiceRunsOneLoop(t *testing.T) {
	o, taskRepo, agentRepo, runner := newTestOrchestrator(t)
	agent := seedAgent(t, agentRepo, "Runner")
	seedTask(t, taskRepo, "only once", agent, nil)

	o.Start()
	o.Start()
	defer o.Stop()

	waitFor(t, "the task to run", func() bool { return runner.CallCount() >= 1 })
	// Two loops would each pick the task up on their own ticker.
	time.Sleep(2 * time.Second)

	if got := runner.CallCount(); got != 1 {
		t.Errorf("the task ran %d times, want once", got)
	}
}

// Stop closes a channel. Closing it twice panics, so the guard matters.
func TestStopTwiceDoesNotPanic(t *testing.T) {
	o, _, _, _ := newTestOrchestrator(t)

	o.Start()
	o.Stop()
	o.Stop()

	if o.IsRunning() {
		t.Error("IsRunning() is true after Stop()")
	}
}

func TestStopBeforeStartIsHarmless(t *testing.T) {
	o, _, _, _ := newTestOrchestrator(t)
	o.Stop()

	if o.IsRunning() {
		t.Error("IsRunning() is true having never started")
	}
}

// Stopping and starting again has to rebuild the stop channel, or the new loop
// exits immediately on the already-closed one.
func TestStartAfterStopScansAgain(t *testing.T) {
	o, taskRepo, agentRepo, runner := newTestOrchestrator(t)
	agent := seedAgent(t, agentRepo, "Runner")

	o.Start()
	o.Stop()

	seedTask(t, taskRepo, "after restart", agent, nil)
	o.Start()
	defer o.Stop()

	waitFor(t, "the task to run after restarting", func() bool {
		return runner.CallCount() == 1
	})
}

func TestStopTaskReportsNothingToStop(t *testing.T) {
	o, taskRepo, agentRepo, _ := newTestOrchestrator(t)
	agent := seedAgent(t, agentRepo, "Runner")
	task := seedTask(t, taskRepo, "not started", agent, nil)

	if o.StopTask(task.TaskID) {
		t.Error("StopTask reported it stopped a task that was never dispatched")
	}
	if o.StopTask("no-such-task") {
		t.Error("StopTask reported success for an unknown id")
	}
}

// The point of StopTask is that the CLI process dies, which the runner observes
// as its context being cancelled.
func TestStopTaskCancelsTheRunInFlight(t *testing.T) {
	o, taskRepo, agentRepo, runner := newTestOrchestrator(t)
	agent := seedAgent(t, agentRepo, "Runner")
	task := seedTask(t, taskRepo, "long job", agent, nil)

	started := make(chan struct{})
	var cancelled bool
	runner.Behaviour = func(ctx context.Context, _ *models.Agent, _ string) *cli.RunResult {
		close(started)
		<-ctx.Done() // hold the task open until StopTask cancels it
		cancelled = true
		return &cli.RunResult{Success: false, Error: "cancelled"}
	}

	o.dispatchAvailable()
	<-started

	if !o.StopTask(task.TaskID) {
		t.Fatal("StopTask did not find the running task")
	}

	waitFor(t, "the task to be marked stopped", func() bool {
		return taskStatus(t, o, task.TaskID) == "failed"
	})
	if !cancelled {
		t.Error("the runner's context was never cancelled")
	}
}

func TestRetryTaskReturnsAFailedTaskToTheBacklog(t *testing.T) {
	o, taskRepo, agentRepo, runner := newTestOrchestrator(t)
	agent := seedAgent(t, agentRepo, "Runner")
	task := seedTask(t, taskRepo, "flaky job", agent, nil)

	runner.Behaviour = func(context.Context, *models.Agent, string) *cli.RunResult {
		return &cli.RunResult{Success: false, Error: "boom"}
	}
	// MaxRetries defaults to 3, so drive it until the task gives up.
	waitFor(t, "the task to fail for good", func() bool {
		o.dispatchAvailable()
		return taskStatus(t, o, task.TaskID) == "failed"
	})

	if err := o.RetryTask(task.TaskID); err != nil {
		t.Fatalf("RetryTask: %v", err)
	}
	if got := taskStatus(t, o, task.TaskID); got != "backlog" {
		t.Fatalf("status after retry = %q, want backlog", got)
	}

	// Back in the backlog it must be dispatchable again.
	runner.Behaviour = nil
	waitFor(t, "the retried task to finish", func() bool {
		o.dispatchAvailable()
		return taskStatus(t, o, task.TaskID) == "done"
	})
}

// The backoff used to exist only inside the log line: the failed task went
// straight back to the backlog and the next scan re-ran it, so every paid
// retry fired before a transient error could clear.
func TestFailedTaskWaitsOutItsBackoff(t *testing.T) {
	o, taskRepo, agentRepo, runner := newTestOrchestrator(t)
	agent := seedAgent(t, agentRepo, "Runner")
	seedTask(t, taskRepo, "transient failure", agent, nil)

	o.retryBackoff = func(int) time.Duration { return 500 * time.Millisecond }
	runner.Behaviour = func(context.Context, *models.Agent, string) *cli.RunResult {
		return &cli.RunResult{Success: false, Error: "boom"}
	}

	o.dispatchAvailable()
	waitForIdle(t, o)
	if got := runner.CallCount(); got != 1 {
		t.Fatalf("first attempt ran %d times, want 1", got)
	}

	// Scans inside the backoff window must not re-dispatch it.
	for i := 0; i < 5; i++ {
		o.dispatchAvailable()
		waitForIdle(t, o)
	}
	if got := runner.CallCount(); got != 1 {
		t.Fatalf("task re-ran during its backoff window: %d runs", got)
	}

	// Once the window has passed, the retry goes through.
	time.Sleep(600 * time.Millisecond)
	o.dispatchAvailable()
	waitForIdle(t, o)
	if got := runner.CallCount(); got != 2 {
		t.Fatalf("after the backoff elapsed: %d runs, want 2", got)
	}
}

// A task that always fails must stop after max_retries attempts. It used to run
// forever: the orchestrator incremented the retry counter on its own copy of the
// task, the database column stayed at zero, and every scan started another paid
// agent run.
func TestAFailingTaskStopsAtItsRetryLimit(t *testing.T) {
	o, taskRepo, agentRepo, runner := newTestOrchestrator(t)
	agent := seedAgent(t, agentRepo, "Runner")
	task := seedTask(t, taskRepo, "always fails", agent, nil)

	runner.Behaviour = func(context.Context, *models.Agent, string) *cli.RunResult {
		return &cli.RunResult{Success: false, Error: "boom"}
	}

	// Scan far more times than the limit allows.
	for i := 0; i < 10; i++ {
		o.dispatchAvailable()
		waitForIdle(t, o)
	}

	if got := taskStatus(t, o, task.TaskID); got != "failed" {
		t.Errorf("status = %q after ten scans, want failed", got)
	}

	stored, err := taskRepo.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	limit := stored[0].MaxRetries
	if got := runner.CallCount(); got > limit {
		t.Errorf("the agent ran %d times for a limit of %d — each run costs money", got, limit)
	}
	if stored[0].RetryCount == 0 {
		t.Error("retry_count was never persisted, so the limit cannot hold across scans")
	}
}

// Stopping the orchestrator while a task sits at an approval prompt used to
// strand it: the goroutine returned without putting the task back, and nothing
// picks a "running" row up again, so the task and its agent stayed busy until
// the app restarted.
func TestStopWhileAwaitingApprovalReleasesTheTask(t *testing.T) {
	o, taskRepo, agentRepo, _ := newTestOrchestrator(t)
	// The approval gate triggers on architect and BA roles.
	agent := seedAgent(t, agentRepo, "Tech Lead & Architect")
	task := seedTask(t, taskRepo, "design the thing", agent, nil)

	o.Start()
	waitFor(t, "the task to reach the approval gate", func() bool {
		o.approvalMu.Lock()
		_, waiting := o.approvalChans[task.TaskID]
		o.approvalMu.Unlock()
		return waiting
	})

	o.Stop()

	waitFor(t, "the task to be released rather than left running", func() bool {
		return taskStatus(t, o, task.TaskID) != "running"
	})
}

// Two tasks can be matched to the same agent and run side by side. Each run gets
// its own copy of the agent, so writing the whole row back lost whatever the
// other run had recorded — one of two completions simply vanished.
func TestConcurrentTasksOnOneAgentBothCount(t *testing.T) {
	o, taskRepo, agentRepo, runner := newTestOrchestrator(t)
	agent := seedAgent(t, agentRepo, "Runner")
	seedTask(t, taskRepo, "task one", agent, nil)
	seedTask(t, taskRepo, "task two", agent, nil)

	// Hold both runs open so the two copies of the agent overlap.
	runner.Behaviour = func(context.Context, *models.Agent, string) *cli.RunResult {
		time.Sleep(200 * time.Millisecond)
		return &cli.RunResult{Success: true, Output: "ok"}
	}

	o.dispatchAvailable()
	waitForIdle(t, o)

	if got := runner.PeakInFlight(); got < 2 {
		t.Fatalf("the runs did not overlap (peak %d), so this proves nothing", got)
	}

	agents, err := agentRepo.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	if agents[0].TasksDone != 2 {
		t.Errorf("tasks_done = %d after two finished tasks, want 2", agents[0].TasksDone)
	}
}

// A failed write of a task's state used to be discarded, which is how the board
// could quietly stop matching what the agents were doing. This builds its own
// database so it can break the connection mid-run and make the writes fail.
func TestAFailedTaskWriteIsReported(t *testing.T) {
	db, err := database.OpenAt(filepath.Join(t.TempDir(), "doomed.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer db.Close()

	taskRepo := database.NewTaskRepository(db)
	agentRepo := database.NewAgentRepository(db)
	runner := &testsupport.FakeRunner{}

	o := NewOrchestrator(
		agentRepo, taskRepo, database.NewMemoryRepository(db), runner,
		services.NewContextManager(), services.NewGitService(),
		services.NewBrowserAgentService(),
	)
	o.SetWorkspaceDir("")
	o.SetVerifyBuild(false)

	agent := seedAgent(t, agentRepo, "Runner")
	seedTask(t, taskRepo, "doomed write", agent, nil)

	var logMu sync.Mutex
	var reported []string
	o.SetEventHandlers(func(message, level string) {
		if level != "ERROR" {
			return
		}
		logMu.Lock()
		reported = append(reported, message)
		logMu.Unlock()
	}, nil, nil)

	// Break the connection while the agent works, so every write after it fails.
	runner.Behaviour = func(context.Context, *models.Agent, string) *cli.RunResult {
		_ = db.Close()
		return &cli.RunResult{Success: true, Output: "done"}
	}

	o.dispatchAvailable()
	waitForIdle(t, o)

	logMu.Lock()
	defer logMu.Unlock()
	for _, message := range reported {
		if strings.Contains(message, "Không ghi được") {
			return
		}
	}
	t.Errorf("a failed task write was never reported; errors seen: %v", reported)
}

// The orchestrator retries on its own and a scheduled job can start it at 2am,
// so a runaway costs real money with nobody watching. Past the day's ceiling,
// nothing new is dispatched.
func TestNoNewWorkOnceTheDailyBudgetIsSpent(t *testing.T) {
	o, taskRepo, agentRepo, runner := newTestOrchestrator(t)
	agent := seedAgent(t, agentRepo, "Runner")
	seedTask(t, taskRepo, "first", agent, nil)
	seedTask(t, taskRepo, "second", agent, nil)

	o.SetMaxConcurrency(1)
	o.SetDailyBudgetUSD(0.50)

	// One run that costs more than the whole day's allowance.
	runner.Behaviour = func(context.Context, *models.Agent, string) *cli.RunResult {
		return &cli.RunResult{Success: true, Output: "done", CostUSD: 0.75}
	}

	o.dispatchAvailable()
	waitForIdle(t, o)

	// The second task must not start now the ceiling has been passed.
	o.dispatchAvailable()
	waitForIdle(t, o)

	if got := runner.CallCount(); got != 1 {
		t.Errorf("the agent ran %d times after passing the budget, want 1", got)
	}
	if _, spent, limit := o.BudgetSnapshot(); spent < 0.74 || limit != 0.50 {
		t.Errorf("budget reports spent=%v limit=%v, want about 0.75 and 0.50", spent, limit)
	}
}

// A budget nobody set must not stop anything.
func TestWorkRunsFreelyWithNoBudgetSet(t *testing.T) {
	o, taskRepo, agentRepo, runner := newTestOrchestrator(t)
	agent := seedAgent(t, agentRepo, "Runner")
	seedTask(t, taskRepo, "costly", agent, nil)

	runner.Behaviour = func(context.Context, *models.Agent, string) *cli.RunResult {
		return &cli.RunResult{Success: true, Output: "done", CostUSD: 999}
	}

	o.dispatchAvailable()
	waitForIdle(t, o)

	if runner.CallCount() != 1 {
		t.Errorf("the task did not run with no budget configured")
	}
}
