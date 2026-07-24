package tui

import (
	"testing"

	"claude_suite/backend/models"
)

func TestRepositoryTaskActionsMutateOnlySelectedDatabase(t *testing.T) {
	path := createLegacyDatabase(t)
	actions, err := OpenRepositoryTaskActions(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := actions.CreateTask(models.Task{Title: "TUI task", Prompt: "TUI task"}); err != nil {
		t.Fatal(err)
	}
	if err := actions.Close(); err != nil {
		t.Fatal(err)
	}

	snapshot, err := LoadReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Tasks) != 2 {
		t.Fatalf("tasks=%d", len(snapshot.Tasks))
	}
	found := false
	for _, task := range snapshot.Tasks {
		if task.Title == "TUI task" && task.Status == "backlog" {
			found = true
		}
	}
	if !found {
		t.Fatal("created task was not readable through shared snapshot")
	}
}
