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

// Regression: a task mentioning "DATABASE" must go to the Back-end Developer,
// not the Business Analyst (whose "BA" keyword is a substring of "DATABASE").
func TestFindMatchingAgent_NoSubstringFalseMatch(t *testing.T) {
	dispatcher := NewAgentDispatcher()
	agents := []models.Agent{
		{AgentID: "agent-ba", Name: "Business Analyst (BA)", Role: "Requirements"},
		{AgentID: "agent-code", Name: "Back-end Developer", Role: "Backend logic"},
	}

	task := &models.Task{
		Title:  "Design the DATABASE schema and API",
		Prompt: "Create SQL tables",
	}

	matched := dispatcher.FindMatchingAgent(task, agents)
	if matched == nil {
		t.Fatalf("expected matched agent, got nil")
	}
	if matched.Name == "Business Analyst (BA)" {
		t.Errorf("DATABASE task wrongly matched Business Analyst via substring 'BA'")
	}
}

func TestKeywordMatch(t *testing.T) {
	cases := []struct {
		text, kw string
		want     bool
	}{
		{"DESIGN THE DATABASE SCHEMA", "BA", false},
		{"USE GOOGLE CLOUD", "GO", false},
		{"[BA] ANALYZE", "BA", true},
		{"RUN WEB TEST SUITE", "WEB TEST", true},
		{"BUILD THE UI", "UI", true},
	}
	for _, c := range cases {
		if got := keywordMatch(c.text, c.kw); got != c.want {
			t.Errorf("keywordMatch(%q,%q)=%v want %v", c.text, c.kw, got, c.want)
		}
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
