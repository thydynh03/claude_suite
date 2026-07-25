package orchestrator

import (
	"regexp"
	"strings"

	"claude_suite/backend/models"
)

// keywordMatch reports whether kw occurs in text as a whole word (single tokens)
// or as a substring (multi-word phrases like "WEB TEST"). This prevents false
// positives such as "DATABASE" matching "BA" or "GOOGLE" matching "GO".
func keywordMatch(text, kw string) bool {
	if strings.Contains(kw, " ") {
		return strings.Contains(text, kw)
	}
	re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(kw) + `\b`)
	return re.MatchString(text)
}

// AgentDispatcher matches tasks to the most suitable Agent
type AgentDispatcher struct{}

func NewAgentDispatcher() *AgentDispatcher {
	return &AgentDispatcher{}
}

// FindMatchingAgent finds or selects an agent based on task tags/title
func (d *AgentDispatcher) FindMatchingAgent(task *models.Task, agents []models.Agent) *models.Agent {
	// 1. If task has explicit assignment, match by ID or Name
	if task.AssignedTo != "" && len(agents) > 0 {
		for i, a := range agents {
			if a.AgentID == task.AssignedTo || strings.EqualFold(a.Name, task.AssignedTo) {
				return &agents[i]
			}
		}
	}

	tags := task.ExtractTags()
	tagStr := strings.ToUpper(task.Title + " " + task.Prompt + " " + strings.Join(tags, " "))

	// Determine provider and model preferences from tags
	provider := "claude_cli"
	model := "claude-sonnet-4-5"
	if keywordMatch(tagStr, "ANTI") || keywordMatch(tagStr, "GEMINI") {
		provider = "anti_cli"
		model = "gemini-3.6-flash-high"
	}

	type RoleDef struct {
		RoleName string
		RoleDesc string
		Keywords []string
		Icon     string
		System   string
	}

	roles := []RoleDef{
		{RoleName: "Tech Lead & Architect", RoleDesc: "Technical Leadership & System Architecture", Keywords: []string{"TECHLEAD", "ARCH", "DESIGN", "SYSTEM", "STRUCTURE"}, Icon: "🏗️", System: "You are a Tech Lead & Principal Systems Architect."},
		{RoleName: "Product Manager (PdM)", RoleDesc: "Product Strategy & Feature Definition", Keywords: []string{"PDM", "PRODUCT", "FEATURE", "ROADMAP"}, Icon: "🎯", System: "You are a Product Manager defining feature roadmap and product vision."},
		{RoleName: "Project Manager (PM)", RoleDesc: "Agile Execution & Task Orchestration", Keywords: []string{"PM", "SPRINT", "AGILE", "TASK"}, Icon: "📊", System: "You are a Technical Project Manager tracking project milestones."},
		{RoleName: "Business Analyst (BA)", RoleDesc: "Requirements Specification & Domain Analysis", Keywords: []string{"BA", "REQUIREMENT", "SPEC", "ANALYSIS"}, Icon: "📋", System: "You are a Senior Business Analyst specifying acceptance criteria."},
		{RoleName: "Front-end Developer", RoleDesc: "UI/UX & Frontend Web Engineering", Keywords: []string{"FE", "FRONTEND", "UI", "UX", "HTML", "CSS", "SVELTE", "REACT"}, Icon: "🎨", System: "You are a Senior Frontend Developer specializing in UI/UX web applications."},
		{RoleName: "Back-end Developer", RoleDesc: "Core Business Logic & API/Database Engineering", Keywords: []string{"BE", "BACKEND", "API", "DB", "DATABASE", "GO", "NODE", "SQL", "CODE"}, Icon: "⚙️", System: "You are a Senior Backend Engineer specializing in core logic, APIs, and databases."},
		{RoleName: "DevOps Engineer", RoleDesc: "CI/CD, Build Systems & Deployment Automation", Keywords: []string{"DEVOPS", "DOCKER", "CI", "CD", "BUILD", "DEPLOY"}, Icon: "rocket_launch", System: "You are a DevOps Specialist managing build pipelines and deployment automation."},
		{RoleName: "QA/QC Specialist", RoleDesc: "Quality Control & Automated Testing", Keywords: []string{"QA", "QC", "TEST", "AUTOMATION", "REVIEW", "AUDIT"}, Icon: "science", System: "You are a QA/QC Engineer writing automated test suites and validating functionality."},
		{RoleName: "Web E2E Tester (Chrome CDP)", RoleDesc: "Automated Web Testing & Screenshot Verification", Keywords: []string{"E2E", "WEB TEST", "BROWSER", "UI TEST", "SCREENSHOT", "CHROME", "CDP", "PLAYWRIGHT"}, Icon: "language", System: "You are an Automated Web E2E Testing Agent controlling Chrome CDP to navigate web apps, extract DOM, and verify UI screenshot proofs."},
	}

	for _, r := range roles {
		for _, kw := range r.Keywords {
			if keywordMatch(tagStr, kw) {
				// Search for existing matching agent
				for i, a := range agents {
					if keywordMatch(strings.ToUpper(a.Name), kw) || keywordMatch(strings.ToUpper(a.Role), kw) || strings.EqualFold(a.Name, r.RoleName) {
						return &agents[i]
					}
				}

				// Auto-create Sub-Agent dynamically for this role
				return &models.Agent{
					Name:     r.RoleName,
					Role:     r.RoleDesc,
					Model:    model,
					Provider: provider,
					Icon:     r.Icon,
					System:   r.System,
					Status:   "idle",
				}
			}
		}
	}

	if len(agents) > 0 {
		for i, a := range agents {
			if strings.Contains(a.Name, "Architect") || strings.Contains(a.Name, "Chief") {
				return &agents[i]
			}
		}
		return &agents[0]
	}

	// Dynamic fallback if no agents exist in database
	return &models.Agent{
		Name:     "Tech Lead & Architect",
		Role:     "Technical Leadership & System Architecture",
		Model:    model,
		Provider: provider,
		Icon:     "🏗️",
		System:   "You are a Tech Lead & Principal Systems Architect.",
		Status:   "idle",
	}
}
