package database

import (
	"path/filepath"
	"testing"

	"claude_suite/backend/models"
)

func agentRepoWithOne(t *testing.T) (*AgentRepository, *models.Agent) {
	t.Helper()

	db, err := OpenAt(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	repo := NewAgentRepository(db)
	agent := &models.Agent{
		Name:     "Runner",
		Role:     "Tester",
		Provider: "claude_cli",
		Model:    "claude-sonnet-4-5",
		Status:   "idle",
	}
	if err := repo.Create(agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return repo, agent
}

func reload(t *testing.T, repo *AgentRepository) models.Agent {
	t.Helper()
	agents, err := repo.GetAll()
	if err != nil {
		t.Fatalf("read agents: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected one agent, found %d", len(agents))
	}
	return agents[0]
}

// Update writes the whole row from the caller's copy. Two tasks can share an
// agent, and each run holds its own copy, so the later write puts back whatever
// that copy still believed — this is the shape of the bug, stated plainly.
func TestUpdateFromAStaleCopyOverwritesOtherChanges(t *testing.T) {
	repo, agent := agentRepoWithOne(t)

	stale := *agent // a second task's copy, taken before anything changed

	switched := *agent
	switched.Provider = "anti_cli"
	switched.Model = "gemini-3.6-flash-high"
	if err := repo.Update(&switched); err != nil {
		t.Fatalf("update: %v", err)
	}

	stale.Status = "idle"
	if err := repo.Update(&stale); err != nil {
		t.Fatalf("update: %v", err)
	}

	if got := reload(t, repo); got.Provider != "claude_cli" {
		t.Fatalf("this test no longer demonstrates the clobber: provider = %q", got.Provider)
	}
}

// Saving a persona edit while tasks run must not roll the live counters back
// to what the settings page happened to load: Update writes the profile, and
// only the profile.
func TestUpdateLeavesLiveStateAlone(t *testing.T) {
	repo, agent := agentRepoWithOne(t)

	pageCopy := *agent // what the settings page holds, loaded before any run

	// A task finishes while the user is typing.
	if err := repo.CompleteTask(agent.AgentID); err != nil {
		t.Fatalf("complete: %v", err)
	}

	pageCopy.System = "edited persona"
	if err := repo.Update(&pageCopy); err != nil {
		t.Fatalf("update: %v", err)
	}

	got := reload(t, repo)
	if got.System != "edited persona" {
		t.Errorf("system = %q, the profile edit was lost", got.System)
	}
	if got.TasksDone != 1 {
		t.Errorf("tasks_done = %d, the save rolled live counters back", got.TasksDone)
	}
}

// SetProviderModel records the fallback switch on its own, so a task finishing
// afterwards cannot put the exhausted provider back.
func TestSetProviderModelSurvivesALaterStatusWrite(t *testing.T) {
	repo, agent := agentRepoWithOne(t)

	if err := repo.SetProviderModel(agent.AgentID, "anti_cli", "gemini-3.6-flash-high"); err != nil {
		t.Fatalf("set provider: %v", err)
	}
	// A concurrent task finishing, holding a copy from before the switch.
	if err := repo.SetStatus(agent.AgentID, "idle", "some task", ""); err != nil {
		t.Fatalf("set status: %v", err)
	}

	got := reload(t, repo)
	if got.Provider != "anti_cli" || got.Model != "gemini-3.6-flash-high" {
		t.Errorf("provider/model = %q/%q, want the fallback to stand", got.Provider, got.Model)
	}
	if got.Status != "idle" || got.LastTask != "some task" {
		t.Errorf("status = %q, last task = %q — the status write was lost", got.Status, got.LastTask)
	}
}

// CompleteTask counts in SQL, so two tasks finishing together cannot lose a
// count the way read-modify-write does.
func TestCompleteTaskCountsEveryFinish(t *testing.T) {
	repo, agent := agentRepoWithOne(t)

	for i := 0; i < 5; i++ {
		if err := repo.CompleteTask(agent.AgentID); err != nil {
			t.Fatalf("complete: %v", err)
		}
	}

	got := reload(t, repo)
	if got.TasksDone != 5 {
		t.Errorf("tasks_done = %d, want 5", got.TasksDone)
	}
	if got.Status != "idle" {
		t.Errorf("status = %q, want idle", got.Status)
	}
}

func TestSetStatusLeavesTheCountAlone(t *testing.T) {
	repo, agent := agentRepoWithOne(t)

	if err := repo.CompleteTask(agent.AgentID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if err := repo.SetStatus(agent.AgentID, "running", "next task", ""); err != nil {
		t.Fatalf("set status: %v", err)
	}

	got := reload(t, repo)
	if got.TasksDone != 1 {
		t.Errorf("tasks_done = %d, want the earlier finish to stand", got.TasksDone)
	}
	if got.Status != "running" {
		t.Errorf("status = %q, want running", got.Status)
	}
}
