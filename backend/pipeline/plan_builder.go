package pipeline

import (
	"encoding/json"
	"fmt"
	"regexp"

	"claude_suite/backend/cli"
	"claude_suite/backend/models"
)

type PlanBuilder struct {
	cliRunner cli.CLIRunner
}

func NewPlanBuilder(cliRunner cli.CLIRunner) *PlanBuilder {
	return &PlanBuilder{cliRunner: cliRunner}
}

type DecomposedTask struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Prompt      string   `json:"prompt"`
	Priority    string   `json:"priority"`
	ModelHint   string   `json:"model_hint"`
	DependsOn   []int    `json:"depends_on"` // 0-indexed dependency indices
}

type DecomposedPlan struct {
	Tasks []DecomposedTask `json:"tasks"`
}

func (p *PlanBuilder) Decompose(projectRequirement string, cwd string) ([]models.Task, error) {
	systemPrompt := `You are a Principal Software Architect. Decompose the following project requirement into structured, executable tasks.
OUTPUT ONLY VALID JSON with this exact schema:
{
  "tasks": [
    {
      "title": "Task Title",
      "description": "Short description",
      "prompt": "Detailed actionable prompt for LLM agent",
      "priority": "high" | "normal" | "low",
      "model_hint": "claude-opus-4-8" | "claude-sonnet-4-5",
      "depends_on": [] // 0-based task indices
    }
  ]
}`

	prompt := fmt.Sprintf("Project Requirement:\n%s\n\nDecompose into JSON task tree.", projectRequirement)

	result := p.cliRunner.RunOnce(prompt, "claude-opus-4-8", systemPrompt, nil, cwd)

	if !result.Success {
		// Fallback to simple split
		return p.SimpleSplit(projectRequirement), nil
	}

	tasks, err := parseJSONTasks(result.Output, projectRequirement)
	if err != nil || len(tasks) == 0 {
		return p.SimpleSplit(projectRequirement), nil
	}

	return tasks, nil
}

func (p *PlanBuilder) SimpleSplit(requirement string) []models.Task {
	return []models.Task{
		{Title: "Phân tích yêu cầu", Description: "Phân tích và hiểu yêu cầu: " + requirement, Prompt: "Analyze requirements for: " + requirement, Priority: "high", Status: "backlog"},
		{Title: "Lập kế hoạch thực hiện", Description: "Tạo kế hoạch chi tiết", Prompt: "Create detailed execution plan for: " + requirement, Priority: "high", Status: "backlog"},
		{Title: "Thực hiện Phase 1", Description: "Code và phát triển tính năng core", Prompt: "Execute phase 1 of: " + requirement, Priority: "normal", Status: "backlog"},
		{Title: "Kiểm thử & Review", Description: "Viết unit test và review code", Prompt: "Review and write tests for: " + requirement, Priority: "normal", Status: "backlog"},
	}
}

func parseJSONTasks(rawJSON string, requirement string) ([]models.Task, error) {
	// Extract JSON block using regex if wrapped in markdown ```json
	re := regexp.MustCompile(`(?s)\{.*?\}`)
	match := re.FindString(rawJSON)
	if match == "" {
		match = rawJSON
	}

	var plan DecomposedPlan
	if err := json.Unmarshal([]byte(match), &plan); err != nil {
		return nil, err
	}

	var tasks []models.Task
	taskIDMap := make(map[int]string)

	for i, dt := range plan.Tasks {
		t := models.Task{
			Title:       dt.Title,
			Description: dt.Description,
			Prompt:      dt.Prompt,
			Priority:    dt.Priority,
			Status:      "backlog",
			DependsOn:   []string{},
		}
		if t.Priority == "" {
			t.Priority = "normal"
		}
		tasks = append(tasks, t)
		taskIDMap[i] = tasks[i].TaskID
	}

	// Resolve 0-based indices to task IDs
	for i, dt := range plan.Tasks {
		for _, depIdx := range dt.DependsOn {
			if parentID, ok := taskIDMap[depIdx]; ok {
				tasks[i].DependsOn = append(tasks[i].DependsOn, parentID)
			}
		}
	}

	return tasks, nil
}
