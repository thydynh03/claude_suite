package database

import (
	"path/filepath"
	"strings"
	"testing"

	"agent_center/backend/models"
)

func newAttemptTestRepos(t *testing.T) (*AttemptRepository, *TaskRepository) {
	t.Helper()
	db, err := OpenAt(filepath.Join(t.TempDir(), "attempts_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewAttemptRepository(db), NewTaskRepository(db)
}

func TestAttemptNumbersCountUpAndListNewestFirst(t *testing.T) {
	attempts, tasks := newAttemptTestRepos(t)
	task := &models.Task{Title: "failing task"}
	if err := tasks.Create(task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := attempts.Add(task.TaskID, "first error"); err != nil {
		t.Fatalf("add attempt 1: %v", err)
	}
	if err := attempts.Add(task.TaskID, "second error"); err != nil {
		t.Fatalf("add attempt 2: %v", err)
	}

	got, err := attempts.ListByTask(task.TaskID, 10)
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("attempts = %d, want 2", len(got))
	}
	if got[0].AttemptNo != 2 || got[1].AttemptNo != 1 {
		t.Fatalf("order = [%d, %d], want newest first [2, 1]", got[0].AttemptNo, got[1].AttemptNo)
	}
	if got[0].Error != "second error" || got[1].Error != "first error" {
		t.Fatalf("error text misassigned: %q / %q", got[0].Error, got[1].Error)
	}
}

// The manual Retry button resets tasks.retry_count to zero; attempt numbering
// must come from the attempts table itself or the two desynchronize.
func TestAttemptNumberingSurvivesResetForRetry(t *testing.T) {
	attempts, tasks := newAttemptTestRepos(t)
	task := &models.Task{Title: "reset then fail again", Status: "failed"}
	if err := tasks.Create(task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	_ = attempts.Add(task.TaskID, "error before reset 1")
	_ = attempts.Add(task.TaskID, "error before reset 2")

	if err := tasks.ResetForRetry(task.TaskID); err != nil {
		t.Fatalf("reset for retry: %v", err)
	}

	if err := attempts.Add(task.TaskID, "error after reset"); err != nil {
		t.Fatalf("add attempt after reset: %v", err)
	}

	got, err := attempts.ListByTask(task.TaskID, 10)
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("attempts = %d, want 3 (history survives the reset)", len(got))
	}
	if got[0].AttemptNo != 3 {
		t.Fatalf("attempt after reset numbered %d, want 3", got[0].AttemptNo)
	}
	if !strings.Contains(got[0].Error, "after reset") {
		t.Fatalf("newest attempt error = %q, want the post-reset one", got[0].Error)
	}
}

func TestAttemptsAreScopedPerTask(t *testing.T) {
	attempts, tasks := newAttemptTestRepos(t)
	a := &models.Task{Title: "task a"}
	b := &models.Task{Title: "task b"}
	if err := tasks.Create(a); err != nil {
		t.Fatal(err)
	}
	if err := tasks.Create(b); err != nil {
		t.Fatal(err)
	}

	_ = attempts.Add(a.TaskID, "a failed")
	_ = attempts.Add(b.TaskID, "b failed")

	got, err := attempts.ListByTask(a.TaskID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Error != "a failed" {
		t.Fatalf("task a attempts = %+v, want only its own", got)
	}
	if got[0].AttemptNo != 1 {
		t.Fatalf("task a first attempt numbered %d, want 1 (numbering is per task)", got[0].AttemptNo)
	}
}
