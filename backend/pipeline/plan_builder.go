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
	RoleTag      string   `json:"role_tag"`      // "BA" | "ARCH" | "CODE" | "REVIEW" | "QA" | "DEVOPS"
	TargetFiles  string   `json:"target_files"`  // Target files list
	Action       string   `json:"action"`        // Action summary
	Output       string   `json:"output"`        // Expected output
	DependsOn    []int    `json:"depends_on"`    // 0-indexed dependency indices
}

type DecomposedPlan struct {
	Tasks []DecomposedTask `json:"tasks"`
}

func (p *PlanBuilder) Decompose(projectRequirement string, cwd string) ([]models.Task, error) {
	return p.DecomposeWithProvider(projectRequirement, cwd, "claude_cli", "claude-opus-4-8")
}

func (p *PlanBuilder) DecomposeWithProvider(projectRequirement string, cwd string, provider string, model string) ([]models.Task, error) {
	systemPrompt := "You are a Senior Principal Software Architect and Lead Systems Engineer.\n" +
		"Your goal is to perform deep technical brainstorming and decompose the given project requirement into comprehensive, highly structured, production-ready tasks.\n\n" +
		"CRITICAL STRUCTURED TAG REQUIREMENTS:\n" +
		"Every single task MUST include explicit structured tags in its prompt and JSON fields:\n" +
		"- [ACTION]: Specific technical action to take.\n" +
		"- [REQUIREMENTS]: Detailed specifications, business logic, data models, edge cases, and security.\n" +
		"- [TARGET FILES]: Specific file paths to create, modify, or test.\n" +
		"- [EXPECTED OUTPUT]: Concrete deliverables, verification criteria, and test coverage.\n\n" +
		"RULES FOR BRAINSTORMING & DECOMPOSITION:\n" +
		"1. Even if the user requirement is short or vague (e.g. 'sss' or 'create task app'), expand it into a full enterprise-grade architecture with BA, ARCH, CODE, QA, and DEVOPS phases.\n" +
		"2. Provide rich, clear titles starting with role tags like '[BA]', '[ARCH]', '[CODE]', '[QA]', or '[DEVOPS]'.\n" +
		"3. Write comprehensive multi-sentence descriptions covering data models, API endpoints, UI state, performance, and unit testing.\n" +
		"4. Format every task prompt with clear markdown sections: '📌 [ACTION]', '📑 [REQUIREMENTS]', '📁 [TARGET FILES]', '🎯 [EXPECTED OUTPUT]'.\n" +
		"5. Respond ONLY with valid JSON in a ```json ... ``` codeblock matching this EXACT schema:\n" +
		"{\n" +
		"  \"tasks\": [\n" +
		"    {\n" +
		"      \"title\": \"[ARCH] Detailed task title\",\n" +
		"      \"description\": \"Comprehensive multi-sentence architectural description detailing requirements, endpoints, UI design, logic, and acceptance criteria\",\n" +
		"      \"prompt\": \"📌 [ACTION]: Build core REST API endpoints\\n📑 [REQUIREMENTS]: Handle JWT auth, SQLite persistence, rate limiting\\n📁 [TARGET FILES]: backend/api/router.go, backend/models/task.go\\n🎯 [EXPECTED OUTPUT]: Passing unit tests, 100% clean build, swagger specs\",\n" +
		"      \"priority\": \"high\" | \"normal\" | \"low\",\n" +
		"      \"provider_hint\": \"claude_cli\" | \"anti_cli\",\n" +
		"      \"role_tag\": \"BA\" | \"ARCH\" | \"CODE\" | \"REVIEW\" | \"QA\" | \"DEVOPS\",\n" +
		"      \"target_files\": \"backend/api/router.go, backend/models/task.go\",\n" +
		"      \"action\": \"Build core REST API endpoints\",\n" +
		"      \"output\": \"Passing unit tests, 100% clean build\",\n" +
		"      \"depends_on\": []\n" +
		"    }\n" +
		"  ]\n" +
		"}"

	reqText := strings.TrimSpace(projectRequirement)
	if len(reqText) < 5 {
		reqText = fmt.Sprintf("Develop full enterprise application suite for feature requirement: %s", projectRequirement)
	}

	prompt := fmt.Sprintf("Project Requirement for Brainstorming & Task Decomposition:\n\"%s\"\n\nDecompose into 4 to 6 highly detailed, comprehensive production tasks with all required tags ([ACTION], [REQUIREMENTS], [TARGET FILES], [EXPECTED OUTPUT]).", reqText)

	var result *cli.RunResult
	if provider == "anti_cli" {
		anti := cli.NewAntigravityCLI()
		result = anti.RunOnce(prompt, model, systemPrompt, nil, cwd)
	} else {
		result = p.cliRunner.RunOnce(prompt, model, systemPrompt, nil, cwd)
	}

	if !result.Success {
		return p.SimpleSplit(projectRequirement), nil
	}

	tasks, err := parseJSONTasks(result.Output, projectRequirement)
	if err != nil || len(tasks) == 0 {
		return p.SimpleSplit(projectRequirement), nil
	}

	return tasks, nil
}

func (p *PlanBuilder) SimpleSplit(requirement string) []models.Task {
	reqText := strings.TrimSpace(requirement)
	if reqText == "" {
		reqText = "Hệ thống Quản lý và Điều phối Task Tự động"
	}

	return []models.Task{
		{
			Title:       "[BA] Phân tích & Làm rõ Yêu cầu Nghiệp vụ (Requirements Analysis)",
			Description: fmt.Sprintf("Nghiên cứu yêu cầu '%s', định nghĩa User Stories, Use-cases và tiêu chuẩn chấp nhận (Acceptance Criteria).", reqText),
			Prompt:      fmt.Sprintf("[BA][claude_cli]\n📌 [ACTION]: Khảo sát & Phân tích Yêu cầu Nghiệp vụ\n📑 [REQUIREMENTS]: Xác định đối tượng người dùng, quy trình nghiệp vụ chính, các luồng ngoại lệ và ràng buộc bảo mật cho: %s\n📁 [TARGET FILES]: docs/requirements_spec.md, docs/user_stories.md\n🎯 [EXPECTED OUTPUT]: Tài liệu SRS chi tiết với danh sách 100%% Use Cases & Acceptance Criteria.", reqText),
			Priority:    "high",
			Status:      "backlog",
		},
		{
			Title:       "[ARCH] Thiết kế Kiến trúc Hệ thống, Data Schema & REST APIs",
			Description: fmt.Sprintf("Xây dựng mô hình cơ sở dữ liệu SQLite, RESTful API specs, mô hình dữ liệu DTO và sơ đồ luồng dữ liệu cho: %s.", reqText),
			Prompt:      fmt.Sprintf("[ARCH][claude_cli]\n📌 [ACTION]: Thiết kế Architecture & Database Schema cho: %s\n📑 [REQUIREMENTS]: Thiết kế ERD DB Schema, quy hoạch REST API endpoints (/api/v1/...), cơ chế Caching và Middleware xác thực.\n📁 [TARGET FILES]: backend/models/schema.go, backend/api/routes.go, docs/architecture.png\n🎯 [EXPECTED OUTPUT]: Schema SQL hợp lệ, Swagger API Docs và Sơ đồ Mermaid Architecture.", reqText),
			Priority:    "high",
			Status:      "backlog",
		},
		{
			Title:       "[CODE] Phát triển Module Backend Cốt lõi & Dynamic Persistence Layer",
			Description: fmt.Sprintf("Lập trình các API Endpoints, Repositories, Services và tích hợp xử lý giao dịch dữ liệu SQLite cho: %s.", reqText),
			Prompt:      fmt.Sprintf("[CODE][anti_cli]\n📌 [ACTION]: Code Backend Service & Repository Layer cho: %s\n📑 [REQUIREMENTS]: Viết CRUD operations, tích hợp Context timeout, Logging cấu trúc JSON và xử lý lỗi Rate Limit 429.\n📁 [TARGET FILES]: backend/services/core_service.go, backend/database/repository.go\n🎯 [EXPECTED OUTPUT]: Backend biên dịch clean (0 warning, 0 error), chạy benchmark thành công.", reqText),
			Priority:    "normal",
			Status:      "backlog",
		},
		{
			Title:       "[CODE] Xây dựng Giao diện Người dùng (Frontend UI/UX & Reactive State)",
			Description: fmt.Sprintf("Thiết kế giao diện người dùng bằng Svelte 5 + TailwindCSS, tích hợp Reactive Store và đa ngôn ngữ (i18n) cho: %s.", reqText),
			Prompt:      fmt.Sprintf("[CODE][anti_cli]\n📌 [ACTION]: Lập trình Frontend Dashboard UI & State Store cho: %s\n📑 [REQUIREMENTS]: Xây dựng Responsive UI theo phong cách Material Design 3, hỗ trợ Dark/Light Theme và bọc trạng thái loading/error.\n📁 [TARGET FILES]: frontend/src/components/pages/Dashboard.svelte, frontend/src/lib/stores/appState.ts\n🎯 [EXPECTED OUTPUT]: Giao diện mượt mà 60 FPS, vượt qua 'npm run build' không lỗi.", reqText),
			Priority:    "normal",
			Status:      "backlog",
		},
		{
			Title:       "[QA] Kiểm thử Tự động (Unit & Integration Tests) & Security Audit",
			Description: fmt.Sprintf("Viết bộ Unit Test kiểm thử logic nghiệp vụ, E2E test quy trình và rà soát lỗ hổng bảo mật cho: %s.", reqText),
			Prompt:      fmt.Sprintf("[QA][anti_cli]\n📌 [ACTION]: Thực thi Test Suite & Security Audit cho: %s\n📑 [REQUIREMENTS]: Kiểm thử độ phủ mã (Code Coverage > 80%%), kiểm tra leak memory, test API rate limits và bảo vệ secret keys.\n📁 [TARGET FILES]: backend/tests/core_test.go, tests/e2e_spec.ts\n🎯 [EXPECTED OUTPUT]: Báo cáo 'go test ./...' 100%% PASSED và báo cáo Security Clean.", reqText),
			Priority:    "normal",
			Status:      "backlog",
		},
	}
}

func parseJSONTasks(rawJSON string, requirement string) ([]models.Task, error) {
	if strings.TrimSpace(rawJSON) == "" {
		return nil, fmt.Errorf("empty raw JSON")
	}

	var jsonStr string
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
		return nil, fmt.Errorf("unmarshal error: %v", err)
	}

	var tasks []models.Task

	for _, dt := range plan.Tasks {
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

		title := dt.Title
		if !strings.HasPrefix(title, "[") {
			title = fmt.Sprintf("[%s] %s", roleTag, dt.Title)
		}

		prompt := dt.Prompt
		if !strings.Contains(prompt, "[ACTION]") {
			actionStr := dt.Action
			if actionStr == "" {
				actionStr = dt.Title
			}
			filesStr := dt.TargetFiles
			if filesStr == "" {
				filesStr = "Tệp thuộc module cần sửa"
			}
			outputStr := dt.Output
			if outputStr == "" {
				outputStr = "Biên dịch clean, unit test 100% PASSED"
			}

			prompt = fmt.Sprintf("[%s][%s]\n📌 [ACTION]: %s\n📑 [REQUIREMENTS]: %s\n📁 [TARGET FILES]: %s\n🎯 [EXPECTED OUTPUT]: %s",
				roleTag, providerHint, actionStr, dt.Description, filesStr, outputStr)
		} else if !strings.HasPrefix(prompt, "[") {
			prompt = fmt.Sprintf("[%s][%s]\n%s", roleTag, providerHint, prompt)
		}

		priority := dt.Priority
		if priority == "" {
			priority = "normal"
		}

		desc := dt.Description
		if desc == "" {
			desc = dt.Title
		}

		tasks = append(tasks, models.Task{
			Title:       title,
			Description: desc,
			Prompt:      prompt,
			Priority:    priority,
			Status:      "backlog",
		})
	}

	return tasks, nil
}
