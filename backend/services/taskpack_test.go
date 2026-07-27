package services

import (
	"path/filepath"
	"strings"
	"testing"

	"agent_center/backend/database"
	"agent_center/backend/models"
)

func newPackHarness(t *testing.T) (*ContextManager, *database.TaskRepository, *database.AttemptRepository) {
	t.Helper()
	db, err := database.OpenAt(filepath.Join(t.TempDir(), "taskpack_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	taskRepo := database.NewTaskRepository(db)
	attemptRepo := database.NewAttemptRepository(db)
	cm := NewContextManager()
	cm.AttachRepos(taskRepo, attemptRepo)
	return cm, taskRepo, attemptRepo
}

func TestBuildTaskPackWithoutReposOrHistoryIsEmpty(t *testing.T) {
	bare := NewContextManager() // no repos attached
	task := &models.Task{TaskID: "x", Title: "t"}
	if got := bare.BuildTaskPack("", "", task, DefaultPackBudget); got.Text != "" {
		t.Fatalf("no repos: pack = %q, want empty", got)
	}

	cm, taskRepo, _ := newPackHarness(t)
	fresh := &models.Task{Title: "fresh task"}
	if err := taskRepo.Create(fresh); err != nil {
		t.Fatal(err)
	}
	if got := cm.BuildTaskPack("", "", fresh, DefaultPackBudget); got.Text != "" {
		t.Fatalf("fresh task with no history: pack = %q, want empty", got)
	}
}

func TestBuildTaskPackBudgetZeroIsTheKillSwitch(t *testing.T) {
	cm, taskRepo, attemptRepo := newPackHarness(t)
	task := &models.Task{Title: "has history"}
	if err := taskRepo.Create(task); err != nil {
		t.Fatal(err)
	}
	_ = attemptRepo.Add(task.TaskID, "some failure")

	if got := cm.BuildTaskPack("", "", task, 0); got.Text != "" {
		t.Fatalf("budget 0 must disable injection entirely, got %q", got)
	}
}

func TestBuildTaskPackCarriesDependencyResults(t *testing.T) {
	cm, taskRepo, _ := newPackHarness(t)

	dep := &models.Task{Title: "[CODE] fix the updater", Status: "done"}
	if err := taskRepo.Create(dep); err != nil {
		t.Fatal(err)
	}
	if err := taskRepo.UpdateStatus(dep.TaskID, "done", "Đã sửa retry loop trong updater.go, thêm backoff.", "s1"); err != nil {
		t.Fatal(err)
	}

	qa := &models.Task{Title: "[QA] verify the fix", DependsOn: []string{dep.TaskID}}
	if err := taskRepo.Create(qa); err != nil {
		t.Fatal(err)
	}

	pack := cm.BuildTaskPack("", "", qa, DefaultPackBudget).Text
	if !strings.Contains(pack, "Results of prerequisite tasks") {
		t.Fatalf("pack missing the dependency section:\n%s", pack)
	}
	if !strings.Contains(pack, "Đã sửa retry loop trong updater.go") {
		t.Fatalf("pack missing the dependency's result text:\n%s", pack)
	}
	if !strings.Contains(pack, "HISTORICAL REFERENCE ONLY") {
		t.Fatalf("pack missing the stale-replay guard:\n%s", pack)
	}
}

func TestBuildTaskPackCarriesPreviousAttempts(t *testing.T) {
	cm, taskRepo, attemptRepo := newPackHarness(t)
	task := &models.Task{Title: "flaky build"}
	if err := taskRepo.Create(task); err != nil {
		t.Fatal(err)
	}
	_ = attemptRepo.Add(task.TaskID, "go build failed: undefined symbol foo")

	pack := cm.BuildTaskPack("", "", task, DefaultPackBudget).Text
	if !strings.Contains(pack, "Previous attempts on THIS task failed with") {
		t.Fatalf("pack missing the attempts section:\n%s", pack)
	}
	if !strings.Contains(pack, "undefined symbol foo") {
		t.Fatalf("pack missing the attempt's error text:\n%s", pack)
	}
}

// The pack must respect its budget no matter how much history exists, and the
// cut must be visible, not silent.
func TestBuildTaskPackNeverExceedsItsBudget(t *testing.T) {
	cm, taskRepo, attemptRepo := newPackHarness(t)
	task := &models.Task{Title: "very noisy task"}
	if err := taskRepo.Create(task); err != nil {
		t.Fatal(err)
	}
	huge := strings.Repeat("lỗi rất dài với tiếng Việt có dấu ", 200)
	for i := 0; i < 3; i++ {
		_ = attemptRepo.Add(task.TaskID, huge)
	}

	budget := 600
	pack := cm.BuildTaskPack("", "", task, budget).Text
	if pack == "" {
		t.Fatal("expected a pack")
	}
	if len(pack) > budget+len(packTruncationMarker) {
		t.Fatalf("pack length %d exceeds budget %d (+marker %d)", len(pack), budget, len(packTruncationMarker))
	}
	if !strings.Contains(pack, "truncated at budget") {
		t.Fatal("truncation must be visible, not silent")
	}
	// UTF-8 must survive the cut (Vietnamese text corrupts on byte slicing).
	if !strings.ContainsRune(pack, 'ỗ') && !strings.Contains(pack, "lỗi") {
		t.Fatalf("expected Vietnamese text to survive intact in the pack:\n%s", pack)
	}
}

func TestBuildTaskPackTruncatesEachDependencyResult(t *testing.T) {
	cm, taskRepo, _ := newPackHarness(t)

	dep := &models.Task{Title: "chatty dependency"}
	if err := taskRepo.Create(dep); err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("x", 5000)
	if err := taskRepo.UpdateStatus(dep.TaskID, "done", long, ""); err != nil {
		t.Fatal(err)
	}
	next := &models.Task{Title: "dependent", DependsOn: []string{dep.TaskID}}
	if err := taskRepo.Create(next); err != nil {
		t.Fatal(err)
	}

	pack := cm.BuildTaskPack("", "", next, DefaultPackBudget).Text
	if strings.Contains(pack, strings.Repeat("x", packDepResultCap+100)) {
		t.Fatal("one dependency result must not exceed its own cap")
	}
}
