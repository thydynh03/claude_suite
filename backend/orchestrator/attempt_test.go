package orchestrator

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent_center/backend/cli"
	"agent_center/backend/database"
	"agent_center/backend/models"
	"agent_center/backend/services"
	"agent_center/backend/testsupport"
)

// A task that fails to its retry limit must leave one attempt row per failure,
// each carrying the run's actual error text — that history is what the retry
// context in the task prompt is built from.
func TestFailedAttemptsRecordTheirErrorText(t *testing.T) {
	db, err := database.OpenAt(filepath.Join(t.TempDir(), "attempt_orch_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	taskRepo := database.NewTaskRepository(db)
	agentRepo := database.NewAgentRepository(db)
	attemptRepo := database.NewAttemptRepository(db)
	runner := &testsupport.FakeRunner{
		Behaviour: func(ctx context.Context, agent *models.Agent, prompt string) *cli.RunResult {
			return &cli.RunResult{Success: false, Error: "boom: go build failed with exit 1"}
		},
	}

	o := NewOrchestrator(
		agentRepo, taskRepo, database.NewMemoryRepository(db), runner,
		services.NewContextManager(), services.NewGitService(),
		services.NewBrowserAgentService(),
	)
	o.SetAttemptRepo(attemptRepo)
	o.SetWorkspaceDir("")
	o.SetVerifyBuild(false)
	o.retryBackoff = func(int) time.Duration { return 0 }

	name := seedAgent(t, agentRepo, "Failer")
	task := seedTask(t, taskRepo, "Always fails", name, nil)

	// Drive scans until the task exhausts max_retries (default 3) and lands on
	// failed. Each scan dispatches at most one attempt of this task.
	waitFor(t, "the task to reach failed", func() bool {
		o.dispatchAvailable()
		o.wg.Wait()
		all, err := taskRepo.GetAll()
		if err != nil {
			return false
		}
		for _, x := range all {
			if x.TaskID == task.TaskID {
				return x.Status == "failed"
			}
		}
		return false
	})

	attempts, err := attemptRepo.ListByTask(task.TaskID, 10)
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	if len(attempts) != 3 {
		t.Fatalf("attempts recorded = %d, want 3 (one per failed run)", len(attempts))
	}
	for _, a := range attempts {
		if !strings.Contains(a.Error, "boom: go build failed") {
			t.Fatalf("attempt %d error = %q, want the run's real error text", a.AttemptNo, a.Error)
		}
	}
	if attempts[0].AttemptNo != 3 {
		t.Fatalf("newest attempt numbered %d, want 3", attempts[0].AttemptNo)
	}
}
