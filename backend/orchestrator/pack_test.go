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

// packHarness wires an orchestrator whose ContextManager has real repos, so
// the context pack path runs end-to-end against a throwaway database.
func packHarness(t *testing.T, runner *testsupport.FakeRunner) (*Orchestrator, *database.TaskRepository, *database.AgentRepository) {
	t.Helper()
	db, err := database.OpenAt(filepath.Join(t.TempDir(), "pack_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	taskRepo := database.NewTaskRepository(db)
	agentRepo := database.NewAgentRepository(db)
	attemptRepo := database.NewAttemptRepository(db)
	cm := services.NewContextManager()
	cm.AttachRepos(taskRepo, attemptRepo)

	o := NewOrchestrator(
		agentRepo, taskRepo, database.NewMemoryRepository(db), runner,
		cm, services.NewGitService(), services.NewBrowserAgentService(),
	)
	o.SetAttemptRepo(attemptRepo)
	o.SetWorkspaceDir("")
	o.SetVerifyBuild(false)
	o.retryBackoff = func(int) time.Duration { return 0 }
	return o, taskRepo, agentRepo
}

// A retried task must be told what its earlier attempt failed with — the
// retry prompt used to be byte-identical to the first attempt.
func TestRetryPromptCarriesThePreviousError(t *testing.T) {
	fail := true
	runner := &testsupport.FakeRunner{
		Behaviour: func(ctx context.Context, agent *models.Agent, prompt string) *cli.RunResult {
			if fail {
				fail = false
				return &cli.RunResult{Success: false, Error: "npm ERR! missing dependency left-pad"}
			}
			return &cli.RunResult{Success: true, Output: "done"}
		},
	}
	o, taskRepo, agentRepo := packHarness(t, runner)
	name := seedAgent(t, agentRepo, "Retrier")
	task := seedTask(t, taskRepo, "Install packages", name, nil)

	waitFor(t, "the task to reach done after one retry", func() bool {
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

	calls := runner.Calls()
	if len(calls) != 2 {
		t.Fatalf("runs = %d, want 2 (fail then retry)", len(calls))
	}
	if strings.Contains(calls[0].Prompt, "Previous attempts") {
		t.Fatalf("first attempt must not carry retry context:\n%s", calls[0].Prompt)
	}
	if !strings.Contains(calls[1].Prompt, "Previous attempts on THIS task failed with") {
		t.Fatalf("retry prompt missing the attempts section:\n%s", calls[1].Prompt)
	}
	if !strings.Contains(calls[1].Prompt, "npm ERR! missing dependency left-pad") {
		t.Fatalf("retry prompt missing the actual error text:\n%s", calls[1].Prompt)
	}
	if !strings.Contains(calls[1].Prompt, "HISTORICAL REFERENCE ONLY") {
		t.Fatalf("retry prompt missing the stale-replay guard:\n%s", calls[1].Prompt)
	}
}

// A dependent task must see what its prerequisite produced — depends_on used
// to gate scheduling only and carried no content.
func TestDependentTaskPromptCarriesPrerequisiteResult(t *testing.T) {
	runner := &testsupport.FakeRunner{
		Behaviour: func(ctx context.Context, agent *models.Agent, prompt string) *cli.RunResult {
			return &cli.RunResult{Success: true, Output: "REST endpoint /api/users implemented with pagination"}
		},
	}
	o, taskRepo, agentRepo := packHarness(t, runner)
	name := seedAgent(t, agentRepo, "Chainer")

	first := seedTask(t, taskRepo, "[CODE] build the endpoint", name, nil)
	seedTask(t, taskRepo, "[QA] verify the endpoint", name, []string{first.TaskID})

	waitFor(t, "both tasks to finish in order", func() bool {
		o.dispatchAvailable()
		o.wg.Wait()
		all, _ := taskRepo.GetAll()
		doneCount := 0
		for _, x := range all {
			if x.Status == "done" {
				doneCount++
			}
		}
		return doneCount == 2
	})

	calls := runner.Calls()
	if len(calls) != 2 {
		t.Fatalf("runs = %d, want 2", len(calls))
	}
	qaPrompt := calls[1].Prompt
	if !strings.Contains(qaPrompt, "Results of prerequisite tasks") {
		t.Fatalf("dependent prompt missing the prerequisite section:\n%s", qaPrompt)
	}
	if !strings.Contains(qaPrompt, "REST endpoint /api/users implemented") {
		t.Fatalf("dependent prompt missing the prerequisite's result:\n%s", qaPrompt)
	}
}

// Budget zero is the kill switch: no pack, prompts untouched.
func TestPackBudgetZeroDisablesInjection(t *testing.T) {
	fail := true
	runner := &testsupport.FakeRunner{
		Behaviour: func(ctx context.Context, agent *models.Agent, prompt string) *cli.RunResult {
			if fail {
				fail = false
				return &cli.RunResult{Success: false, Error: "transient"}
			}
			return &cli.RunResult{Success: true, Output: "done"}
		},
	}
	o, taskRepo, agentRepo := packHarness(t, runner)
	o.SetPackBudget(0)
	name := seedAgent(t, agentRepo, "NoPack")
	task := seedTask(t, taskRepo, "No pack task", name, nil)

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

	for i, c := range runner.Calls() {
		if strings.Contains(c.Prompt, "CONTEXT PACK") {
			t.Fatalf("run %d received a pack although the budget is 0:\n%s", i, c.Prompt)
		}
	}
}

func TestAdjustEstimatedUsageSubtractsOnlyEstimates(t *testing.T) {
	est := &cli.RunResult{TokensUsed: 4000, UsageEstimated: true}
	adjustEstimatedUsage(est, 8000) // pack ≈ 2000 tokens
	if est.TokensUsed != 2000 {
		t.Fatalf("estimated usage = %d, want 2000 after subtracting the pack", est.TokensUsed)
	}

	real := &cli.RunResult{TokensUsed: 4000, UsageEstimated: false}
	adjustEstimatedUsage(real, 8000)
	if real.TokensUsed != 4000 {
		t.Fatalf("CLI-reported usage must never be adjusted, got %d", real.TokensUsed)
	}

	tiny := &cli.RunResult{TokensUsed: 10, UsageEstimated: true}
	adjustEstimatedUsage(tiny, 8000)
	if tiny.TokensUsed != 10 {
		t.Fatalf("subtraction must never drive usage negative or zero it out blindly, got %d", tiny.TokensUsed)
	}
}
