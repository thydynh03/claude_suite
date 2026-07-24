package orchestrator

import (
	"testing"

	"claude_suite/backend/models"
)

func TestFindMatchingAgent_ExplicitAssignment(t *testing.T) {
	dispatcher := NewAgentDispatcher()
	agents := []models.Agent{
		{AgentID: "agent-1", Name: "Tech Lead & Architect", Role: "Architecture"},
		{AgentID: "agent-2", Name: "Business Analyst (BA)", Role: "Requirements"},
	}

	task := &models.Task{
		Title:      "Review requirement specs",
		AssignedTo: "agent-2",
	}

	matched := dispatcher.FindMatchingAgent(task, agents)
	if matched == nil {
		t.Fatalf("expected matched agent, got nil")
	}
	if matched.AgentID != "agent-2" {
		t.Errorf("expected agent-2, got %s", matched.AgentID)
	}
}

func TestFindMatchingAgent_TagBasedMatch(t *testing.T) {
	dispatcher := NewAgentDispatcher()
	agents := []models.Agent{
		{AgentID: "agent-arch", Name: "Tech Lead & Architect", Role: "Architecture"},
		{AgentID: "agent-ba", Name: "Business Analyst (BA)", Role: "Requirements"},
		{AgentID: "agent-code", Name: "Back-end Developer", Role: "Backend logic"},
	}

	task := &models.Task{
		Title:  "[BA] Analyze user requirement spec",
		Prompt: "Decompose BA user stories",
	}

	matched := dispatcher.FindMatchingAgent(task, agents)
	if matched == nil {
		t.Fatalf("expected matched agent, got nil")
	}
	if matched.Name != "Business Analyst (BA)" {
		t.Errorf("expected Business Analyst, got %s", matched.Name)
	}
}

func TestFindMatchingAgent_AutoCreateRoleWhenEmpty(t *testing.T) {
	dispatcher := NewAgentDispatcher()
	var agents []models.Agent

	task := &models.Task{
		Title:  "[QA] Write automated test cases",
		Prompt: "Execute end-to-end testing",
	}

	matched := dispatcher.FindMatchingAgent(task, agents)
	if matched == nil {
		t.Fatalf("expected matched agent, got nil")
	}
	if matched.Name != "QA/QC Specialist" {
		t.Errorf("expected QA/QC Specialist, got %s", matched.Name)
	}
}
