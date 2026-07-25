package orchestrator

import (
	"context"
	"testing"
	"time"

	"claude_suite/backend/cli"
	"claude_suite/backend/models"
)

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
		waitFor(t, "the run to settle", func() bool {
			return taskStatus(t, o, task.TaskID) != "running"
		})
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
