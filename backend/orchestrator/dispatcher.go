package orchestrator

import (
	"strings"

	"claude_suite/backend/models"
)

// AgentDispatcher matches tasks to the most suitable Agent
type AgentDispatcher struct{}

func NewAgentDispatcher() *AgentDispatcher {
	return &AgentDispatcher{}
}

// FindMatchingAgent finds or selects an agent based on task tags/title
func (d *AgentDispatcher) FindMatchingAgent(task *models.Task, agents []models.Agent) *models.Agent {
	if len(agents) == 0 {
		return nil
	}

	// 1. If task has explicit assignment, match by ID or Name
	if task.AssignedTo != "" {
		for i, a := range agents {
			if a.AgentID == task.AssignedTo || strings.EqualFold(a.Name, task.AssignedTo) {
				return &agents[i]
			}
		}
	}

	tags := task.ExtractTags()
	tagStr := strings.Join(tags, " ")

	// 2. Role-based matching rule
	roleKeywords := map[string][]string{
		"Architect": {"ARCH", "DESIGN", "SYSTEM", "STRUCTURE"},
		"Reviewer":  {"REVIEW", "SECURITY", "AUDIT"},
		"QA":        {"TEST", "QA", "AUTOMATION"},
		"BA":        {"PLAN", "REQUIREMENT", "SPEC"},
		"Developer": {"CODE", "DEV", "FULLSTACK", "FE", "BE"},
	}

	for role, keywords := range roleKeywords {
		for _, kw := range keywords {
			if strings.Contains(tagStr, kw) {
				for i, a := range agents {
					if strings.Contains(a.Name, role) || strings.Contains(a.Role, role) {
						return &agents[i]
					}
				}
			}
		}
	}

	// 3. Fallback to Chief Architect or first available agent
	for i, a := range agents {
		if strings.Contains(a.Name, "Architect") || strings.Contains(a.Name, "Chief") {
			return &agents[i]
		}
	}

	return &agents[0]
}
