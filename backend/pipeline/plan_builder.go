package pipeline

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

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
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Prompt       string   `json:"prompt"`
	Priority     string   `json:"priority"`
	ModelHint    string   `json:"model_hint"`
	ProviderHint string   `json:"provider_hint"` // "anti_cli" | "claude_cli"
	RoleTag      string   `json:"role_tag"`      // "BRAINSTORM" | "PLAN" | "ARCH" | "CODE" | "REVIEW" | "TEST" | "QA"
	DependsOn    []int    `json:"depends_on"`    // 0-indexed dependency indices
}

type DecomposedPlan struct {
	Tasks []DecomposedTask `json:"tasks"`
}

func (p *PlanBuilder) Decompose(projectRequirement string, cwd string) ([]models.Task, error) {
	return p.DecomposeWithProvider(projectRequirement, cwd, "claude_cli", "claude-opus-4-8")
}

func (p *PlanBuilder) DecomposeWithProvider(projectRequirement string, cwd string, provider string, model string) ([]models.Task, error) {
	systemPrompt := "You are an Elite Enterprise Tech Lead & Software Architect.\n" +
		"Decompose the given project requirement into comprehensive, highly detailed, production-ready executable tasks.\n\n" +
		"RULES FOR HIGH DETAIL OUTPUT:\n" +
		"1. Provide rich, clear titles with explicit technical scope.\n" +
		"2. Write comprehensive multi-sentence descriptions covering requirements, specs, data structures, and edge cases.\n" +
		"3. Write exhaustive, multi-step prompt instructions for the assigned LLM subagent (specify file paths, API contracts, libraries, error handling, and unit test requirements).\n" +
		"4. Respond ONLY with valid JSON in a ```json ... ``` codeblock matching this EXACT schema:\n" +
		"{\n" +
		"  \"tasks\": [\n" +
		"    {\n" +
		"      \"title\": \"Detailed task title\",\n" +
		"      \"description\": \"Comprehensive description detailing requirements, endpoints, UI design, logic, and acceptance criteria\",\n" +
		"      \"prompt\": \"Step-by-step actionable prompt for LLM agent detailing exact implementation steps, files to create/modify, logic flow, error handling, and automated unit tests\",\n" +
		"      \"priority\": \"high\" | \"normal\" | \"low\",\n" +
		"      \"provider_hint\": \"claude_cli\" | \"anti_cli\",\n" +
		"      \"role_tag\": \"BA\" | \"ARCH\" | \"CODE\" | \"REVIEW\" | \"QA\" | \"DEVOPS\" | \"PM\" | \"PDM\",\n" +
		"      \"depends_on\": []\n" +
		"    }\n" +
		"  ]\n" +
		"}"

	prompt := fmt.Sprintf("Project Requirement:\n\"%s\"\n\nDecompose into 3 to 7 highly detailed, comprehensive tasks with step-by-step prompts.", projectRequirement)

	var result *cli.RunResult
	if provider == "anti_cli" {
		anti := cli.NewAntigravityCLI()
		result = anti.RunOnce(prompt, model, systemPrompt, nil, cwd)
	} else {
		result = p.cliRunner.RunOnce(prompt, model, systemPrompt, nil, cwd)
	}

	if !result.Success {
		if result.Error != "" {
			return nil, fmt.Errorf("CLI Error: %s", result.Error)
		}
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
		{Title: "[BA] Phân tích yêu cầu", Description: "Phân tích và hiểu yêu cầu: " + requirement, Prompt: "[BA][claude_cli] Analyze requirements for: " + requirement, Priority: "high", Status: "backlog"},
		{Title: "[ARCH] Lập kiến trúc & kế hoạch", Description: "Tạo kế hoạch chi tiết", Prompt: "[ARCH][claude_cli] Create detailed architecture plan for: " + requirement, Priority: "high", Status: "backlog"},
		{Title: "[CODE] Thực hiện Phase 1 (Core Dev)", Description: "Code và phát triển tính năng core", Prompt: "[CODE][anti_cli] Execute core feature development for: " + requirement, Priority: "normal", Status: "backlog"},
		{Title: "[QA] Kiểm thử & Review Code", Description: "Viết unit test và review code", Prompt: "[QA][anti_cli] Review and write automated tests for: " + requirement, Priority: "normal", Status: "backlog"},
	}
}

func parseJSONTasks(rawJSON string, requirement string) ([]models.Task, error) {
	if strings.TrimSpace(rawJSON) == "" {
		return nil, fmt.Errorf("empty raw JSON")
	}

	var jsonStr string
	// Check for ```json ... ``` codeblock
	codeBlockReg := regexp.MustCompile("(?s)```(?:json)?\\s*(\\{[\\s\\S]*?\\})\\s*```")
	matches := codeBlockReg.FindStringSubmatch(rawJSON)

	if len(matches) > 1 && strings.TrimSpace(matches[1]) != "" {
		jsonStr = matches[1]
	} else {
		firstOpen := strings.Index(rawJSON, "{")
		lastClose := strings.LastIndex(rawJSON, "}")
		if firstOpen != -1 && lastClose > firstOpen {
			jsonStr = rawJSON[firstOpen : lastClose+1]
		} else {
			jsonStr = rawJSON
		}
	}

	var plan DecomposedPlan
	if err := json.Unmarshal([]byte(jsonStr), &plan); err != nil {
		return nil, fmt.Errorf("unmarshal error: %v (jsonStr len=%d)", err, len(jsonStr))
	}

	var tasks []models.Task
	taskIDMap := make(map[int]string)

	for i, dt := range plan.Tasks {
		roleTag := dt.RoleTag
		if roleTag == "" {
			titleLower := fmt.Sprintf("%s %s", dt.Title, dt.Prompt)
			if regexp.MustCompile(`(?i)test|qa|kiểm`).MatchString(titleLower) {
				roleTag = "QA"
			} else if regexp.MustCompile(`(?i)arch|design|kiến trúc`).MatchString(titleLower) {
				roleTag = "ARCH"
			} else if regexp.MustCompile(`(?i)ba|req|yêu cầu|plan`).MatchString(titleLower) {
				roleTag = "BA"
			} else if regexp.MustCompile(`(?i)review|audit|sec`).MatchString(titleLower) {
				roleTag = "REVIEW"
			} else {
				roleTag = "CODE"
			}
		}

		providerHint := dt.ProviderHint
		if providerHint == "" {
			providerHint = "claude_cli"
		}

		promptWithTags := fmt.Sprintf("[%s][%s] %s", roleTag, providerHint, dt.Prompt)
		if dt.Prompt == "" {
			promptWithTags = fmt.Sprintf("[%s][%s] %s", roleTag, providerHint, dt.Title)
		}

		t := models.Task{
			Title:       fmt.Sprintf("[%s] %s", roleTag, dt.Title),
			Description: dt.Description,
			Prompt:      promptWithTags,
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

	for i, dt := range plan.Tasks {
		for _, depIdx := range dt.DependsOn {
			if parentID, ok := taskIDMap[depIdx]; ok {
				tasks[i].DependsOn = append(tasks[i].DependsOn, parentID)
			}
		}
	}

	return tasks, nil
}
