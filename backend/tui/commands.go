package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"claude_suite/backend/cli"
	"claude_suite/backend/models"

	tea "charm.land/bubbletea/v2"
)

func (m Model) toggleOrchestrator() (tea.Model, tea.Cmd) {
	if m.tasks == nil {
		m.status, m.statusErr = "Orchestrator unavailable: start with --write", true
		return m, nil
	}
	if m.tasks.IsOrchestratorRunning() {
		m.tasks.StopOrchestrator()
		m.orchestratorRunning = false
		m.status, m.statusErr = "Orchestrator stopped", false
	} else {
		m.tasks.StartOrchestrator()
		m.orchestratorRunning = true
		m.status, m.statusErr = "Orchestrator started", false
	}
	return m, nil
}

func (m Model) toggleAutoApprove() (tea.Model, tea.Cmd) {
	if m.tasks == nil {
		m.status, m.statusErr = "Auto-approve unavailable: start with --write", true
		return m, nil
	}
	m.autoApproveAll = m.tasks.SetAutoApproveAll(!m.autoApproveAll)
	if m.autoApproveAll {
		m.status, m.statusErr = "Auto-approve enabled for all tasks", false
	} else {
		m.status, m.statusErr = "Auto-approve disabled", false
	}
	return m, nil
}

func (m Model) cycleAssignedAgent() (tea.Model, tea.Cmd) {
	if m.tasks == nil {
		m.status, m.statusErr = "Assign unavailable: start with --write", true
		return m, nil
	}
	rows := m.rows()
	if len(rows) == 0 {
		return m, nil
	}
	item := rows[m.cursor[m.screen]]
	currentAssignee := ""
	for _, task := range m.snapshot.Tasks {
		if task.TaskID == item.id {
			currentAssignee = task.AssignedTo
			break
		}
	}
	names := []string{""}
	for _, agent := range m.snapshot.Agents {
		names = append(names, agent.Name)
	}
	current := indexOfString(names, currentAssignee)
	next := names[(max(0, current)+1)%len(names)]
	return m.runTaskMutation("Assigning task…", func() error {
		return m.tasks.AssignTask(item.id, next)
	})
}

func (m Model) runBrowserTask(targetURL string) (tea.Model, tea.Cmd) {
	if strings.TrimSpace(targetURL) == "" {
		m.status, m.statusErr = "URL is required", true
		return m, nil
	}
	if m.tasks == nil {
		m.status, m.statusErr = "Browser agent unavailable: start with --write", true
		return m, nil
	}
	m.browserURL = strings.TrimSpace(targetURL)
	m.browserRunning = true
	m.browserResult = nil
	m.browserError = ""
	url, screenshot := m.browserURL, m.browserScreenshot
	m.status, m.statusErr = "Running browser agent…", false
	return m, func() tea.Msg {
		result, err := m.tasks.RunBrowserTask(url, screenshot)
		return browserRunMsg{result: result, err: err}
	}
}

func (m Model) runPipeline() (tea.Model, tea.Cmd) {
	return m.runTaskMutation("Running 5-stage pipeline (Plan → Architect → Code → Review → QA)…", func() error {
		return m.tasks.RunPipeline()
	})
}

func (m Model) exportReport() (tea.Model, tea.Cmd) {
	if m.tasks == nil {
		m.status, m.statusErr = "Report export unavailable: shared service not connected", true
		return m, nil
	}
	m.status, m.statusErr = "Exporting Kanban report…", false
	return m, func() tea.Msg {
		path, err := m.tasks.ExportKanbanReport()
		return reportMsg{path: path, err: err}
	}
}

func (m Model) updateConfirmation(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "n", "esc", "q":
		if m.confirm == "approval" && m.tasks != nil {
			m.tasks.ResolveApproval(m.approvalTaskID, false)
		}
		m.confirm = ""
		m.approvalAgent, m.approvalTask = "", ""
		m.pendingAntiKeyName, m.pendingAntiKeySecret = "", ""
		m.pendingGitBranch, m.pendingGitMessage, m.pendingGitHash = "", "", ""
	case "y", "enter":
		action := m.confirm
		m.confirm = ""
		switch action {
		case "delete":
			return m.deleteSelectedTask()
		case "delete-agent":
			return m.deleteSelectedAgent()
		case "reset-agents":
			return m.runTaskMutation("Resetting agents to defaults…", func() error {
				return m.tasks.ResetAgentsToDefaults()
			})
		case "add-antikey":
			cli.GlobalAntiPool.AddKey(m.pendingAntiKeyName, m.pendingAntiKeySecret)
			m.status, m.statusErr = "Added Antigravity key: "+m.pendingAntiKeyName, false
			m.pendingAntiKeyName, m.pendingAntiKeySecret = "", ""
		case "git-create-branch":
			branch := m.pendingGitBranch
			m.pendingGitBranch = ""
			return m.runGitMutation("Creating branch "+branch+"…", func() error {
				return m.tasks.CreateGitBranch(branch)
			})
		case "git-checkout-branch":
			branch := m.pendingGitBranch
			m.pendingGitBranch = ""
			return m.runGitMutation("Checking out "+branch+"…", func() error {
				return m.tasks.CheckoutGitBranch(branch)
			})
		case "git-commit":
			message := m.pendingGitMessage
			m.pendingGitMessage = ""
			return m.runGitMutation("Committing…", func() error {
				return m.tasks.CreateGitCommit(message)
			})
		case "git-revert":
			hash := m.pendingGitHash
			m.pendingGitHash = ""
			return m.runGitMutation("Reverting "+hash+"…", func() error {
				return m.tasks.RevertGitCommit(hash)
			})
		case "toggle-webhook":
			if m.tasks != nil {
				m.webhookRunning = m.tasks.ToggleWebhook(m.webhookPort)
				if m.webhookRunning {
					m.status, m.statusErr = fmt.Sprintf("Webhook listening on port %d", m.webhookPort), false
				} else {
					m.status, m.statusErr = "Webhook stopped", false
				}
			}
		case "clear-done":
			return m.runTaskMutation("Clearing completed tasks…", func() error {
				return m.tasks.DeleteDoneTasks()
			})
		case "approval":
			if m.tasks != nil {
				m.tasks.ResolveApproval(m.approvalTaskID, true)
			}
			m.approvalAgent, m.approvalTask = "", ""
			m.status, m.statusErr = "Task approved", false
		}
	}
	return m, nil
}

func (m Model) moveSelectedTask(delta int) (tea.Model, tea.Cmd) {
	rows := m.rows()
	if len(rows) == 0 {
		return m, nil
	}
	target := min(len(taskStatuses)-1, max(0, m.taskColumn+delta))
	if target == m.taskColumn {
		return m, nil
	}
	item := rows[m.cursor[tasksScreen]]
	return m.runTaskMutation("Moving task to "+taskStatuses[target]+"…", func() error {
		return m.tasks.UpdateTaskStatus(item.id, taskStatuses[target])
	})
}

func (m Model) deleteSelectedTask() (tea.Model, tea.Cmd) {
	rows := m.rows()
	if len(rows) == 0 {
		return m, nil
	}
	item := rows[m.cursor[tasksScreen]]
	return m.runTaskMutation("Deleting task…", func() error {
		return m.tasks.DeleteTask(item.id)
	})
}

func (m Model) deleteSelectedAgent() (tea.Model, tea.Cmd) {
	rows := m.rows()
	if len(rows) == 0 {
		return m, nil
	}
	item := rows[m.cursor[agentsScreen]]
	return m.runTaskMutation("Deleting agent…", func() error {
		return m.tasks.DeleteAgent(item.id)
	})
}

func (m Model) stageAntigravityKey(value string) (tea.Model, tea.Cmd) {
	name, secret := "", value
	if idx := strings.Index(value, ":"); idx >= 0 {
		name, secret = strings.TrimSpace(value[:idx]), strings.TrimSpace(value[idx+1:])
	}
	if secret == "" {
		m.status, m.statusErr = "Key value is required (format name:secret)", true
		return m, nil
	}
	if name == "" {
		name = fmt.Sprintf("Key %d", len(cli.GlobalAntiPool.GetKeys())+1)
	}
	m.pendingAntiKeyName, m.pendingAntiKeySecret = name, secret
	m.confirm = "add-antikey"
	return m, nil
}

func (m Model) stageGitBranch(name, action string) (tea.Model, tea.Cmd) {
	if strings.TrimSpace(name) == "" {
		m.status, m.statusErr = "Branch name is required", true
		return m, nil
	}
	m.pendingGitBranch = strings.TrimSpace(name)
	m.confirm = action
	return m, nil
}

func (m Model) stageGitCommit(message string) (tea.Model, tea.Cmd) {
	if strings.TrimSpace(message) == "" {
		m.status, m.statusErr = "Commit message is required", true
		return m, nil
	}
	m.pendingGitMessage = strings.TrimSpace(message)
	m.confirm = "git-commit"
	return m, nil
}

func (m Model) stageGitRevert(hash string) (tea.Model, tea.Cmd) {
	if strings.TrimSpace(hash) == "" {
		m.status, m.statusErr = "Commit hash is required", true
		return m, nil
	}
	m.pendingGitHash = strings.TrimSpace(hash)
	m.confirm = "git-revert"
	return m, nil
}

func (m Model) runGitMutation(label string, mutate func() error) (tea.Model, tea.Cmd) {
	if m.tasks == nil {
		m.status, m.statusErr = "Git actions unavailable: start with --write", true
		return m, nil
	}
	m.status, m.statusErr = label, false
	m.gitLoading = true
	return m, func() tea.Msg {
		if err := mutate(); err != nil {
			return gitLoadedMsg{err: err}
		}
		status, statusErr := m.tasks.GetGitStatus()
		branches, branchErr := m.tasks.GetGitBranches()
		log, logErr := m.tasks.GetGitLog(8)
		if statusErr != nil {
			return gitLoadedMsg{err: statusErr}
		}
		if branchErr != nil {
			return gitLoadedMsg{err: branchErr}
		}
		if logErr != nil {
			return gitLoadedMsg{err: logErr}
		}
		return gitLoadedMsg{status: status, branches: branches, log: log}
	}
}

func (m Model) createAgent(name string) (tea.Model, tea.Cmd) {
	if name == "" {
		m.status, m.statusErr = "Agent name is required", true
		return m, nil
	}
	return m.runTaskMutation("Creating agent…", func() error {
		return m.tasks.SaveAgent(models.Agent{Name: name, Role: "Specialist", Status: "idle", Model: "claude-sonnet-4-5"})
	})
}

func (m Model) runTaskMutation(label string, mutate func() error) (tea.Model, tea.Cmd) {
	if m.tasks == nil || m.loader == nil {
		m.status, m.statusErr = "Task mutation unavailable: shared service not connected", true
		return m, nil
	}
	m.status, m.statusErr = label, false
	return m, func() tea.Msg {
		if err := mutate(); err != nil {
			return reloadMsg{err: err}
		}
		snapshot, err := m.loader(m.dbPath)
		return reloadMsg{snapshot: snapshot, err: err}
	}
}

type paletteAction struct {
	label, hint string
	target      screen
	reload      bool
	action      string
}

var paletteActions = []paletteAction{
	{label: "Open Cockpit", hint: "Compose and run an AI request", target: cockpitScreen},
	{label: "Open Task Board", hint: "Plan and manage work", target: tasksScreen},
	{label: "Open Agents", hint: "Inspect the agent team", target: agentsScreen},
	{label: "Open Pipeline", hint: "Review execution stages", target: pipelineScreen},
	{label: "Open Scheduler", hint: "Manage prompt automation", target: schedulerScreen},
	{label: "Open Operations", hint: "Watch live activity", target: operationsScreen},
	{label: "Open Settings", hint: "Workspace and services", target: settingsScreen},
	{label: "Reload database", hint: "Refresh the read model", reload: true},
	{label: "Create task", hint: "Add a task to the shared board", action: "create"},
	{label: "AI Plan Builder", hint: "Decompose a requirement into tasks", action: "plan"},
	{label: "Export report", hint: "Write the current Kanban report", action: "report"},
	{label: "Toggle orchestrator", hint: "Start or stop shared execution", action: "orchestrator"},
}

func (m Model) updatePalette(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "ctrl+k":
		m.palette = false
	case "j", "down":
		m.paletteAt = min(len(paletteActions)-1, m.paletteAt+1)
	case "k", "up":
		m.paletteAt = max(0, m.paletteAt-1)
	case "enter":
		return m.activatePalette()
	}
	return m, nil
}

func (m Model) activatePalette() (tea.Model, tea.Cmd) {
	action := paletteActions[m.paletteAt]
	m.palette = false
	if action.reload {
		return m.reload()
	}
	switch action.action {
	case "create":
		m.switchScreen(tasksScreen)
		m.navFocus = false
		m.inputMode, m.input = 'a', ""
		return m, nil
	case "plan":
		m.switchScreen(tasksScreen)
		m.navFocus = false
		m.inputMode, m.input = 'D', ""
		return m, nil
	case "report":
		m.switchScreen(tasksScreen)
		m.navFocus = false
		return m.exportReport()
	case "orchestrator":
		m.navFocus = false
		return m.toggleOrchestrator()
	}
	m.switchScreen(action.target)
	m.navFocus = false
	return m, nil
}

func (m Model) updateInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.inputMode, m.input = 0, ""
	case "ctrl+enter":
		if m.inputMode != 'p' {
			return m, nil
		}
		value := strings.TrimSpace(m.input)
		m.inputMode, m.input = 0, ""
		return m.runQuickPrompt(value)
	case "enter":
		if m.inputMode == 'p' {
			m.input += "\n"
			return m, nil
		}
		mode, value := m.inputMode, strings.TrimSpace(m.input)
		m.inputMode, m.input = 0, ""
		if mode == '/' {
			m.query = value
			m.rebuildDisplayRows()
			m.clampCursor()
			return m, nil
		}
		if mode == 'a' {
			return m.createTask(value)
		}
		if mode == 'D' {
			return m.decomposePlan(value)
		}
		if mode == 'N' {
			return m.createAgent(value)
		}
		if mode == 'K' {
			return m.stageAntigravityKey(value)
		}
		if mode == 'B' {
			return m.stageGitBranch(value, "git-create-branch")
		}
		if mode == 'O' {
			return m.stageGitBranch(value, "git-checkout-branch")
		}
		if mode == 'M' {
			return m.stageGitCommit(value)
		}
		if mode == 'V' {
			return m.stageGitRevert(value)
		}
		if mode == 'U' {
			return m.runBrowserTask(value)
		}
		if mode == 'P' {
			if port, err := strconv.Atoi(value); err == nil && port > 0 && port < 65536 {
				m.webhookPort = port
				m.status, m.statusErr = fmt.Sprintf("Webhook port set to %d", port), false
			} else {
				m.status, m.statusErr = "Invalid port", true
			}
			return m, nil
		}
		return m, nil
	case "backspace":
		if m.input != "" {
			_, size := utf8.DecodeLastRuneInString(m.input)
			m.input = m.input[:len(m.input)-size]
		}
	default:
		if msg.Key().Text != "" {
			m.input += msg.Key().Text
		}
	}
	return m, nil
}

func (m Model) runQuickPrompt(prompt string) (tea.Model, tea.Cmd) {
	if strings.TrimSpace(prompt) == "" {
		m.status, m.statusErr = "A prompt is required", true
		return m, nil
	}
	if m.tasks == nil {
		m.status, m.statusErr = "Quick execution unavailable: start with --write", true
		return m, nil
	}
	m.cockpitPrompt = prompt
	m.cockpitRunning = true
	m.cockpitResult = ""
	provider, model, files := m.cockpitProvider, m.cockpitModel(), m.cockpitFiles
	m.status, m.statusErr = "Running "+m.cockpitProviderLabel()+" CLI…", false
	return m, func() tea.Msg {
		output, err := m.tasks.RunQuickCLI(prompt, provider, model, files)
		return quickRunMsg{output: output, err: err}
	}
}

func (m Model) loadGitInfo() tea.Cmd {
	tasks := m.tasks
	return func() tea.Msg {
		status, err := tasks.GetGitStatus()
		if err != nil {
			return gitLoadedMsg{err: err}
		}
		branches, err := tasks.GetGitBranches()
		if err != nil {
			return gitLoadedMsg{err: err}
		}
		log, err := tasks.GetGitLog(8)
		if err != nil {
			return gitLoadedMsg{err: err}
		}
		return gitLoadedMsg{status: status, branches: branches, log: log}
	}
}

func (m Model) loadWorkspaceFiles() tea.Cmd {
	tasks := m.tasks
	return func() tea.Msg {
		files, err := tasks.ScanWorkspaceFiles()
		return filesLoadedMsg{files: files, err: err}
	}
}

func (m Model) previewCodeStudioFile() (tea.Model, tea.Cmd) {
	if m.codeStudioCursor >= len(m.cockpitAvailFiles) {
		return m, nil
	}
	file := m.cockpitAvailFiles[m.codeStudioCursor]
	m.codeStudioLoading = true
	tasks := m.tasks
	return m, func() tea.Msg {
		content, err := tasks.ReadFileContent(file)
		return codeStudioPreviewMsg{file: file, content: content, err: err}
	}
}

func (m Model) openCodeStudioFile() (tea.Model, tea.Cmd) {
	if m.codeStudioCursor >= len(m.cockpitAvailFiles) {
		return m, nil
	}
	file := m.cockpitAvailFiles[m.codeStudioCursor]
	fullPath := file
	if !filepath.IsAbs(file) && m.snapshot.Workspace != "" {
		fullPath = filepath.Join(m.snapshot.Workspace, file)
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	cmd := exec.Command(editor, fullPath)
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return codeStudioEditMsg{err: err}
	})
}

func (m Model) updateFilePicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter":
		m.cockpitFilePicker = false
	case "j", "down":
		if len(m.cockpitAvailFiles) > 0 {
			m.cockpitFileCursor = min(len(m.cockpitAvailFiles)-1, m.cockpitFileCursor+1)
		}
	case "k", "up":
		m.cockpitFileCursor = max(0, m.cockpitFileCursor-1)
	case " ", "space":
		if m.cockpitFileCursor < len(m.cockpitAvailFiles) {
			file := m.cockpitAvailFiles[m.cockpitFileCursor]
			if idx := indexOfString(m.cockpitFiles, file); idx >= 0 {
				m.cockpitFiles = append(append([]string{}, m.cockpitFiles[:idx]...), m.cockpitFiles[idx+1:]...)
			} else {
				m.cockpitFiles = append(m.cockpitFiles, file)
			}
		}
	case "c":
		m.cockpitFiles = nil
	}
	return m, nil
}

func indexOfString(list []string, value string) int {
	for i, item := range list {
		if item == value {
			return i
		}
	}
	return -1
}

func (m Model) decomposePlan(requirement string) (tea.Model, tea.Cmd) {
	if requirement == "" {
		m.status, m.statusErr = "Project requirement is required", true
		return m, nil
	}
	provider, model := m.planProvider, m.planModel()
	return m.runTaskMutation("AI is decomposing the plan with "+m.planProviderLabel()+"…", func() error {
		return m.tasks.DecomposePlanWithProvider(requirement, provider, model)
	})
}

func (m Model) createTask(title string) (tea.Model, tea.Cmd) {
	if title == "" {
		m.status, m.statusErr = "Task title is required", true
		return m, nil
	}
	task := models.Task{
		Title: title, Prompt: title, Priority: "normal",
		Status: "backlog", DependsOn: []string{}, MaxRetries: 3,
	}
	return m.runTaskMutation("Creating task…", func() error {
		return m.tasks.CreateTask(task)
	})
}

func (m Model) reload() (tea.Model, tea.Cmd) {
	if m.loader == nil || m.reloading {
		return m, nil
	}
	m.reloading = true
	m.status, m.statusErr = "Reloading database…", false
	return m, m.loadSnapshot()
}

func (m Model) loadSnapshot() tea.Cmd {
	if m.loader == nil {
		return nil
	}
	return func() tea.Msg {
		snapshot, err := m.loader(m.dbPath)
		return reloadMsg{snapshot: snapshot, err: err}
	}
}
