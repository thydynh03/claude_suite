package database

import (
	"path/filepath"
	"testing"

	"agent_center/backend/models"
)

func TestFullDatabaseLifecycleIntegration(t *testing.T) {
	// A temp file, never ":memory:" — each new pool connection to ":memory:"
	// is its own empty database, so the tables vanish whenever database/sql
	// opens a second connection mid-test. This is the flake that failed CI
	// with "no such table: tasks"; db.go documents the pitfall.
	db, err := OpenAt(filepath.Join(t.TempDir(), "integration_test.db"))
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	taskRepo := NewTaskRepository(db)
	agentRepo := NewAgentRepository(db)
	memRepo := NewMemoryRepository(db)

	// 1. Create Agent
	ag := &models.Agent{
		Name:     "Integration Test Agent",
		Role:     "Tester",
		Provider: "claude_cli",
		Model:    "claude-sonnet-4-5",
	}
	err = agentRepo.Create(ag)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// 2. Create Task
	tsk := &models.Task{
		Title:       "Integration Task Pipeline",
		Description: "Testing database persistence",
		Priority:    "high",
		Status:      "backlog",
	}
	err = taskRepo.Create(tsk)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// 3. Assign & Update Task Status
	_ = taskRepo.AssignTask(tsk.TaskID, ag.Name)
	_ = taskRepo.UpdateStatus(tsk.TaskID, "running", "", "")

	// 4. Memory Entry Creation
	mem := &models.MemoryItem{
		AgentID: ag.AgentID,
		TaskID:  tsk.TaskID,
		Role:    "assistant",
		Content: "Task executed successfully",
	}
	err = memRepo.Add(mem)
	if err != nil {
		t.Fatalf("failed to add memory item: %v", err)
	}

	// 5. Verify status update to done
	_ = taskRepo.UpdateStatus(tsk.TaskID, "done", "Execution output", "session-123")

	allTasks, err := taskRepo.GetAll()
	if err != nil || len(allTasks) == 0 {
		t.Fatalf("expected tasks in DB, got error %v", err)
	}

	memories, err := memRepo.GetByAgent(ag.AgentID, 10)
	if err != nil || len(memories) == 0 {
		t.Fatalf("expected memory items in DB, got error %v", err)
	}
}
