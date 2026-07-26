package orchestrator

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"claude_suite/backend/cli"
	"claude_suite/backend/database"
	"claude_suite/backend/models"
	"claude_suite/backend/services"
	"claude_suite/backend/testsupport"
)

// newTestOrchestrator wires an orchestrator against a throwaway database and a
// fake CLI runner. Tests drive dispatchAvailable directly instead of Start() so
// they do not wait on the 1.5s loop ticker.
func newTestOrchestrator(t *testing.T) (*Orchestrator, *database.TaskRepository, *database.AgentRepository, *testsupport.FakeRunner) {
	t.Helper()

	db, err := database.OpenAt(filepath.Join(t.TempDir(), "orchestrator_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	taskRepo := database.NewTaskRepository(db)
	agentRepo := database.NewAgentRepository(db)
	memoryRepo := database.NewMemoryRepository(db)
	runner := &testsupport.FakeRunner{}

	o := NewOrchestrator(
		agentRepo, taskRepo, memoryRepo, runner,
		services.NewContextManager(), services.NewGitService(),
		services.NewBrowserAgentService(),
	)
	// No workspace and no build verification: this exercises dispatch and
	// approval, not the git/build side effects.
	o.SetWorkspaceDir("")
	o.SetVerifyBuild(false)
	// Zero backoff: lifecycle tests drive scans directly and assert on the
	// retry *count*, not the pacing. TestFailedTaskWaitsOutItsBackoff covers
	// the pacing with a real duration.
	o.retryBackoff = func(int) time.Duration { return 0 }

	return o, taskRepo, agentRepo, runner
}

// seedAgent creates an agent and returns its name. Tasks assigned to that name
// bypass keyword matching, which keeps dispatch deterministic.
func seedAgent(t *testing.T, agentRepo *database.AgentRepository, name string) string {
	t.Helper()
	agent := &models.Agent{
		Name:     name,
		Role:     "Tester",
		Provider: "claude_cli",
		Model:    "claude-sonnet-4-5",
		Status:   "idle",
	}
	if err := agentRepo.Create(agent); err != nil {
		t.Fatalf("create agent %q: %v", name, err)
	}
	return agent.Name
}

func seedTask(t *testing.T, taskRepo *database.TaskRepository, title, assignee string, dependsOn []string) *models.Task {
	t.Helper()
	task := &models.Task{
		Title:      title,
		Status:     "backlog",
		Priority:   "normal",
		Prompt:     "do the thing",
		AssignedTo: assignee,
		DependsOn:  dependsOn,
	}
	if err := taskRepo.Create(task); err != nil {
		t.Fatalf("create task %q: %v", title, err)
	}
	return task
}

// waitFor polls until cond holds, so tests never depend on a fixed sleep.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestDispatchSkipsTaskWithUnfinishedDependency(t *testing.T) {
	o, taskRepo, agentRepo, runner := newTestOrchestrator(t)
	name := seedAgent(t, agentRepo, "Builder")

	first := seedTask(t, taskRepo, "First task", name, nil)
	blocked := seedTask(t, taskRepo, "Blocked task", name, []string{first.TaskID})

	// Hold the first run open. The fake runner finishes instantly by default,
	// and dispatchAvailable keeps scanning in a loop — so on a slow scheduler
	// (the race-detector CI job) the dependency completes before the next
	// NextDispatchable, at which point dispatching the blocked task is correct
	// behaviour and the test's "still not done" premise is simply false.
	release := make(chan struct{})
	runner.Behaviour = func(ctx context.Context, agent *models.Agent, prompt string) *cli.RunResult {
		<-release
		return &cli.RunResult{Success: true, Output: "done"}
	}

	o.SetMaxConcurrency(4)
	o.dispatchAvailable()

	waitFor(t, "the unblocked task to run", func() bool { return runner.CallCount() >= 1 })
	// Give an (incorrectly) dispatched blocked task a chance to show up before
	// asserting — the dependency is genuinely unfinished for this whole window.
	time.Sleep(300 * time.Millisecond)
	if got := runner.CallCount(); got != 1 {
		t.Fatalf("expected exactly 1 run while the dependency is unfinished, got %d", got)
	}
	all, err := taskRepo.GetAll()
	if err != nil {
		t.Fatalf("reload tasks: %v", err)
	}
	for _, task := range all {
		if task.TaskID == blocked.TaskID && task.Status != "backlog" {
			t.Fatalf("blocked task status = %q, want backlog", task.Status)
		}
	}

	close(release)
	o.wg.Wait()
}

func TestDispatchRespectsMaxConcurrency(t *testing.T) {
	o, taskRepo, agentRepo, runner := newTestOrchestrator(t)
	name := seedAgent(t, agentRepo, "Builder")

	const tasks = 5
	for i := 0; i < tasks; i++ {
		seedTask(t, taskRepo, "Task "+string(rune('A'+i)), name, nil)
	}

	// Hold every run open until the test releases it, so overlap is observable.
	release := make(chan struct{})
	runner.Behaviour = func(ctx context.Context, agent *models.Agent, prompt string) *cli.RunResult {
		<-release
		return &cli.RunResult{Success: true, Output: "done"}
	}

	o.SetMaxConcurrency(2)
	// Dispatch repeatedly: each pass fills the free slots, and with all slots
	// busy the extra passes must not start anything more.
	for i := 0; i < tasks; i++ {
		o.dispatchAvailable()
	}

	waitFor(t, "the concurrency limit to be saturated", func() bool { return runner.PeakInFlight() >= 2 })
	// Give any (incorrectly) extra goroutine a chance to start before asserting.
	time.Sleep(300 * time.Millisecond)
	peak := runner.PeakInFlight()

	close(release)
	o.wg.Wait()

	if peak > 2 {
		t.Fatalf("peak concurrent runs = %d, want at most 2 (maxConcurrency)", peak)
	}
}

// The approval callback used to carry no task id, so the TUI could only call
// ResolveApproval(""), which releases every waiting task at once. With more than
// one task in flight that authorises work the user never saw.
func TestApprovalReleasesOnlyTheNamedTask(t *testing.T) {
	o, taskRepo, agentRepo, runner := newTestOrchestrator(t)
	// "architect" in the name is what triggers the approval checkpoint.
	name := seedAgent(t, agentRepo, "Architect One")

	taskA := seedTask(t, taskRepo, "Design A", name, nil)
	seedTask(t, taskRepo, "Design B", name, nil)

	var approvals []string
	o.SetEventHandlers(
		func(message, level string) {},
		func(taskID, agentName, taskTitle string) {
			o.approvalMu.Lock()
			approvals = append(approvals, taskID)
			o.approvalMu.Unlock()
		},
		func() {},
	)

	o.SetMaxConcurrency(2)
	o.dispatchAvailable()
	o.dispatchAvailable()

	// Both tasks should now be parked on their own approval channel.
	waitFor(t, "both tasks to request approval", func() bool {
		o.approvalMu.Lock()
		defer o.approvalMu.Unlock()
		return len(o.approvalChans) == 2
	})

	// Every emitted approval must name its task, otherwise the UI has nothing
	// to send back and falls into the resolve-everything branch.
	o.approvalMu.Lock()
	emitted := append([]string(nil), approvals...)
	o.approvalMu.Unlock()
	for _, id := range emitted {
		if id == "" {
			t.Fatal("approval event emitted without a task id")
		}
	}

	o.ResolveApproval(taskA.TaskID, true)

	waitFor(t, "the approved task to run", func() bool { return runner.CallCount() >= 1 })
	// The other task must stay parked rather than riding along on this decision.
	time.Sleep(300 * time.Millisecond)
	if got := runner.CallCount(); got != 1 {
		t.Fatalf("%d tasks ran after approving one; the decision leaked to the other", got)
	}

	o.approvalMu.Lock()
	stillWaiting := len(o.approvalChans)
	o.approvalMu.Unlock()
	if stillWaiting != 1 {
		t.Fatalf("%d tasks still awaiting approval, want 1", stillWaiting)
	}

	// Releasing the second one explicitly should let it through.
	o.approvalMu.Lock()
	var remaining string
	for id := range o.approvalChans {
		remaining = id
	}
	o.approvalMu.Unlock()

	o.ResolveApproval(remaining, true)
	waitFor(t, "the second task to run", func() bool { return runner.CallCount() >= 2 })
	o.wg.Wait()
}
