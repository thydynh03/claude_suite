package orchestrator

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"claude_suite/backend/cli"
	"claude_suite/backend/database"
	"claude_suite/backend/models"
	"claude_suite/backend/services"
	"claude_suite/backend/testsupport"
)

// The full memory loop, end to end through the real orchestrator: a task
// fails then succeeds → a mechanical lesson and a regression exist → the NEXT
// task's prompt carries both. This is the feature's acceptance test.
func TestMemoryLoopFeedsTheNextTask(t *testing.T) {
	db, err := database.OpenAt(filepath.Join(t.TempDir(), "memory_loop_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	taskRepo := database.NewTaskRepository(db)
	agentRepo := database.NewAgentRepository(db)
	attemptRepo := database.NewAttemptRepository(db)
	wsRepo := database.NewWorkspaceRepository(db)
	obsRepo := database.NewObservationRepository(db)
	lessonRepo := database.NewLessonRepository(db)
	regressionRepo := database.NewRegressionRepository(db)
	gitSvc := services.NewGitService()

	fail := true
	runner := &testsupport.FakeRunner{
		Behaviour: func(ctx context.Context, agent *models.Agent, prompt string) *cli.RunResult {
			if fail {
				fail = false
				return &cli.RunResult{Success: false, Error: "webhook port 9111 already in use"}
			}
			return &cli.RunResult{Success: true, Output: "Đổi webhook sang bind synchronous rồi check lỗi port."}
		},
	}

	cm := services.NewContextManager()
	cm.AttachRepos(taskRepo, attemptRepo)
	cm.AttachMemoryRepos(lessonRepo, regressionRepo)

	learner := services.NewLearner(lessonRepo, obsRepo, wsRepo, attemptRepo, regressionRepo, gitSvc, runner, "claude-haiku-4-5")
	learner.SetLLMEnabled(false) // deterministic parts only in this test

	o := NewOrchestrator(agentRepo, taskRepo, database.NewMemoryRepository(db), runner,
		cm, gitSvc, services.NewBrowserAgentService())
	o.SetAttemptRepo(attemptRepo)
	o.SetMemoryStores(wsRepo, obsRepo)
	o.SetLearner(learner)
	// A real (non-git) workspace dir so workspace identity resolves.
	o.SetWorkspaceDir(t.TempDir())
	o.SetVerifyBuild(false)
	o.retryBackoff = func(int) time.Duration { return 0 }

	name := seedAgent(t, agentRepo, "Fixer")
	first := seedTask(t, taskRepo, "[CODE] sửa webhook port conflict", name, nil)

	waitFor(t, "the failing task to be fixed", func() bool {
		o.dispatchAvailable()
		o.wg.Wait()
		all, _ := taskRepo.GetAll()
		for _, x := range all {
			if x.TaskID == first.TaskID {
				return x.Status == "done"
			}
		}
		return false
	})

	// The cycle must have produced an active lesson and a regression.
	wsID := o.resolveWorkspaceID(o.GetWorkspaceDir())
	if wsID == "" {
		t.Fatal("workspace identity did not resolve")
	}
	active, err := lessonRepo.ListActive(wsID, 0.7, 8)
	if err != nil || len(active) == 0 {
		t.Fatalf("no active lesson after a fixed cycle: %v %+v", err, active)
	}
	regs, err := regressionRepo.List(wsID, 10)
	if err != nil || len(regs) == 0 {
		t.Fatalf("no regression after a fixed cycle: %v", err)
	}

	// A related follow-up task must receive both in its prompt.
	second := seedTask(t, taskRepo, "[CODE] refactor webhook service", name, nil)
	waitFor(t, "the follow-up task to run", func() bool {
		o.dispatchAvailable()
		o.wg.Wait()
		all, _ := taskRepo.GetAll()
		for _, x := range all {
			if x.TaskID == second.TaskID {
				return x.Status == "done"
			}
		}
		return false
	})

	calls := runner.Calls()
	last := calls[len(calls)-1].Prompt
	if !strings.Contains(last, "Lessons từ các lần chạy trước") {
		t.Fatalf("follow-up prompt missing the lessons section:\n%s", last)
	}
	if !strings.Contains(last, "port 9111") {
		t.Fatalf("follow-up prompt missing the lesson's content:\n%s", last)
	}
	if !strings.Contains(last, "Known fixed bugs — do not re-introduce") {
		t.Fatalf("follow-up prompt missing the regressions section:\n%s", last)
	}
}

// Lifecycle events and the injected pack must land in the observations table
// (via the async writer) — the audit trail the Memory page reads.
func TestTaskLifecycleIsCaptured(t *testing.T) {
	db, err := database.OpenAt(filepath.Join(t.TempDir(), "capture_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	taskRepo := database.NewTaskRepository(db)
	agentRepo := database.NewAgentRepository(db)
	wsRepo := database.NewWorkspaceRepository(db)
	obsRepo := database.NewObservationRepository(db)
	runner := &testsupport.FakeRunner{
		Behaviour: func(ctx context.Context, agent *models.Agent, prompt string) *cli.RunResult {
			return &cli.RunResult{Success: true, Output: "ok, có token=secret123 trong log"}
		},
	}

	o := NewOrchestrator(agentRepo, taskRepo, database.NewMemoryRepository(db), runner,
		services.NewContextManager(), services.NewGitService(), services.NewBrowserAgentService())
	o.SetMemoryStores(wsRepo, obsRepo)
	o.SetWorkspaceDir(t.TempDir())
	o.SetVerifyBuild(false)
	o.retryBackoff = func(int) time.Duration { return 0 }

	name := seedAgent(t, agentRepo, "Observed")
	task := seedTask(t, taskRepo, "Captured task", name, nil)

	waitFor(t, "the task to finish", func() bool {
		o.dispatchAvailable()
		o.wg.Wait()
		all, _ := taskRepo.GetAll()
		for _, x := range all {
			if x.TaskID == task.TaskID {
				return x.Status == "done"
			}
		}
		return false
	})

	// The writer flushes on a 250ms ticker.
	var got []models.Observation
	waitFor(t, "observations to be flushed", func() bool {
		got, _ = obsRepo.ListByTask(task.TaskID, 100)
		return len(got) >= 2
	})

	events := map[string]string{}
	for _, ob := range got {
		events[ob.Event] = ob.Payload
	}
	if _, ok := events["task_start"]; !ok {
		t.Fatalf("task_start not captured: %+v", events)
	}
	payload, ok := events["task_complete"]
	if !ok {
		t.Fatalf("task_complete not captured: %+v", events)
	}
	if strings.Contains(payload, "secret123") {
		t.Fatalf("secret survived into the observation store: %q", payload)
	}
}
