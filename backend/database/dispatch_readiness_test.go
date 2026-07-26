package database

import (
	"testing"

	"claude_suite/backend/models"
)

// mustCreate inserts a task and returns its generated ID, mapping the caller's
// short label to it so dependencies can be written in terms of labels.
func mustCreate(t *testing.T, repo *TaskRepository, ids map[string]string, label, title, status string, dependsOn []string) {
	t.Helper()

	deps := make([]string, 0, len(dependsOn))
	for _, d := range dependsOn {
		if real, ok := ids[d]; ok {
			deps = append(deps, real)
			continue
		}
		// A label with no task behind it is deliberate in one test: it stands for
		// a dependency whose task was deleted.
		deps = append(deps, d)
	}

	task := &models.Task{Title: title, Status: status, Priority: "normal", DependsOn: deps}
	if err := repo.Create(task); err != nil {
		t.Fatalf("create %s: %v", label, err)
	}
	ids[label] = task.TaskID
}

// A backlog task whose dependencies all failed can never run. The board shows it
// as pending forever, which is what "Execute Plan does nothing" turned out to be.
func TestATaskWaitingOnAFailedTaskIsReportedAsDeadlocked(t *testing.T) {
	repo := newTestTaskRepo(t)
	ids := map[string]string{}

	mustCreate(t, repo, ids, "arch", "Thiết kế kiến trúc", "failed", nil)
	mustCreate(t, repo, ids, "e2e", "Kiểm thử E2E", "backlog", []string{"arch"})

	got, err := repo.DispatchReadiness()
	if err != nil {
		t.Fatalf("DispatchReadiness: %v", err)
	}

	if got.Ready != 0 {
		t.Errorf("ready = %d, want 0: nothing can be dispatched", got.Ready)
	}
	if len(got.Blocked) != 1 {
		t.Fatalf("blocked = %d, want 1", len(got.Blocked))
	}
	if !got.Blocked[0].Dead {
		t.Error("dead = false, want true: the blocker failed and will not clear itself")
	}
	if len(got.Blocked[0].Blockers) != 1 || got.Blocked[0].Blockers[0].Title != "Thiết kế kiến trúc" {
		t.Errorf("blockers = %+v, want the failed task named", got.Blocked[0].Blockers)
	}
}

// A dependency that is merely unfinished blocks too, but it is not deadlocked:
// finishing it releases the task. The UI shows these differently.
func TestAnUnfinishedDependencyBlocksWithoutBeingDead(t *testing.T) {
	repo := newTestTaskRepo(t)
	ids := map[string]string{}

	mustCreate(t, repo, ids, "api", "Backend API", "running", nil)
	mustCreate(t, repo, ids, "ui", "Giao diện", "backlog", []string{"api"})

	got, err := repo.DispatchReadiness()
	if err != nil {
		t.Fatalf("DispatchReadiness: %v", err)
	}

	if len(got.Blocked) != 1 || got.Blocked[0].Dead {
		t.Errorf("blocked = %+v, want one entry that is not dead", got.Blocked)
	}
	if got.Running != 1 {
		t.Errorf("running = %d, want 1", got.Running)
	}
}

// A dependency on a task that no longer exists is as final as a failed one —
// deleting a task used to strand everything downstream with no explanation.
func TestADependencyOnAMissingTaskCountsAsDead(t *testing.T) {
	repo := newTestTaskRepo(t)
	ids := map[string]string{}

	mustCreate(t, repo, ids, "orphan", "Task mồ côi", "backlog", []string{"deleted-long-ago"})

	got, err := repo.DispatchReadiness()
	if err != nil {
		t.Fatalf("DispatchReadiness: %v", err)
	}

	if len(got.Blocked) != 1 || !got.Blocked[0].Dead {
		t.Fatalf("blocked = %+v, want one dead entry", got.Blocked)
	}
	if got.Blocked[0].Blockers[0].Status != "missing" {
		t.Errorf("blocker status = %q, want %q", got.Blocked[0].Blockers[0].Status, "missing")
	}
}

func TestTasksWithNoDependenciesAreReadyToDispatch(t *testing.T) {
	repo := newTestTaskRepo(t)
	ids := map[string]string{}

	mustCreate(t, repo, ids, "a", "Task A", "backlog", nil)
	mustCreate(t, repo, ids, "b", "Task B", "queued", nil)
	mustCreate(t, repo, ids, "c", "Task C", "done", nil)

	got, err := repo.DispatchReadiness()
	if err != nil {
		t.Fatalf("DispatchReadiness: %v", err)
	}

	if got.Ready != 2 {
		t.Errorf("ready = %d, want 2 (backlog + queued, not done)", got.Ready)
	}
	if len(got.Blocked) != 0 {
		t.Errorf("blocked = %+v, want none", got.Blocked)
	}
}
