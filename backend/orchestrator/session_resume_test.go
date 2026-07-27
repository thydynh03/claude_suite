package orchestrator

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"agent_center/backend/cli"
	"agent_center/backend/database"
	"agent_center/backend/models"
	"agent_center/backend/services"
	"agent_center/backend/testsupport"
)

// Session resume is an explicit opt-in: with it on, a successful run's
// SessionID lands on the agent row (so the next dispatch resumes the CLI
// conversation); with it off — the default — nothing is written.
func TestSessionResumeWriteBackIsOptIn(t *testing.T) {
	run := func(t *testing.T, enabled bool) string {
		db, err := database.OpenAt(filepath.Join(t.TempDir(), "resume_test.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })

		taskRepo := database.NewTaskRepository(db)
		agentRepo := database.NewAgentRepository(db)
		runner := &testsupport.FakeRunner{
			Behaviour: func(ctx context.Context, agent *models.Agent, prompt string) *cli.RunResult {
				return &cli.RunResult{Success: true, Output: "done", SessionID: "sess-abc-123"}
			},
		}
		o := NewOrchestrator(agentRepo, taskRepo, database.NewMemoryRepository(db), runner,
			services.NewContextManager(), services.NewGitService(), services.NewBrowserAgentService())
		o.SetWorkspaceDir("")
		o.SetVerifyBuild(false)
		o.SetSessionResume(enabled)
		o.retryBackoff = func(int) time.Duration { return 0 }

		name := seedAgent(t, agentRepo, "Resumer")
		task := seedTask(t, taskRepo, "Resumable task", name, nil)

		waitFor(t, "the task to reach done", func() bool {
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

		agents, err := agentRepo.GetAll()
		if err != nil || len(agents) != 1 {
			t.Fatalf("reload agent: %v", err)
		}
		return agents[0].SessionID
	}

	if got := run(t, false); got != "" {
		t.Fatalf("resume OFF must not write a session id, got %q", got)
	}
	if got := run(t, true); got != "sess-abc-123" {
		t.Fatalf("resume ON must persist the run's session id, got %q", got)
	}
}
