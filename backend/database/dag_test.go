package database

import (
	"database/sql"
	"testing"

	"claude_suite/backend/models"
)

func newTestTaskRepo(t *testing.T) *TaskRepository {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := migrateSchema(db); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
	return NewTaskRepository(db)
}

// A task depending on an unfinished task must never be dispatched (DAG gating).
func TestNextDispatchable_BlockedByUnfinishedDependency(t *testing.T) {
	repo := newTestTaskRepo(t)

	dep := &models.Task{Title: "Step 1", Status: "backlog", Priority: "normal"}
	if err := repo.Create(dep); err != nil {
		t.Fatal(err)
	}

	dependent := &models.Task{
		Title: "Step 2", Status: "backlog", Priority: "high",
		DependsOn: []string{dep.TaskID},
	}
	if err := repo.Create(dependent); err != nil {
		t.Fatal(err)
	}

	got, err := repo.NextDispatchable()
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.TaskID != dep.TaskID {
		t.Fatalf("expected the unblocked dependency (Step 1) first, got %+v", got)
	}
}

// Once the dependency is marked done, the dependent task becomes dispatchable.
func TestNextDispatchable_UnblockedAfterDependencyDone(t *testing.T) {
	repo := newTestTaskRepo(t)

	dep := &models.Task{Title: "Step 1", Status: "backlog", Priority: "normal"}
	repo.Create(dep)
	dependent := &models.Task{Title: "Step 2", Status: "backlog", Priority: "normal", DependsOn: []string{dep.TaskID}}
	repo.Create(dependent)

	if err := repo.UpdateStatus(dep.TaskID, "done", "ok", ""); err != nil {
		t.Fatal(err)
	}

	got, err := repo.NextDispatchable()
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.TaskID != dependent.TaskID {
		t.Fatalf("expected Step 2 to be dispatchable once its dependency is done, got %+v", got)
	}
}

// Among unblocked candidates, higher priority is picked first.
func TestNextDispatchable_PrefersHigherPriority(t *testing.T) {
	repo := newTestTaskRepo(t)
	repo.Create(&models.Task{Title: "Low", Status: "backlog", Priority: "low"})
	high := &models.Task{Title: "High", Status: "backlog", Priority: "high"}
	repo.Create(high)

	got, err := repo.NextDispatchable()
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.TaskID != high.TaskID {
		t.Fatalf("expected the high priority task, got %+v", got)
	}
}

// RequeueWithDependency must move the task back to backlog and gate it on depID.
func TestRequeueWithDependency(t *testing.T) {
	repo := newTestTaskRepo(t)
	e2e := &models.Task{Title: "E2E test", Status: "running", Priority: "high"}
	repo.Create(e2e)
	fix := &models.Task{Title: "Fix", Status: "backlog", Priority: "high"}
	repo.Create(fix)

	if err := repo.RequeueWithDependency(e2e.TaskID, fix.TaskID); err != nil {
		t.Fatal(err)
	}

	got, err := repo.NextDispatchable()
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.TaskID != fix.TaskID {
		t.Fatalf("expected the fix task to run before the requeued E2E task, got %+v", got)
	}
}
