package tui

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"claude_suite/backend/cli"
	"claude_suite/backend/models"
	"claude_suite/backend/version"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) paletteView(width, height int) string {
	lines := []string{prismaticText("COMMAND CENTER"), mutedStyle.Render("Jump to a workflow or run an action"), ""}
	visible := max(4, height-7)
	start := max(0, m.paletteAt-visible/2)
	if start+visible > len(paletteActions) {
		start = max(0, len(paletteActions)-visible)
	}
	for index := start; index < min(len(paletteActions), start+visible); index++ {
		action := paletteActions[index]
		prefix := "  "
		label := textStyle.Render(action.label)
		hint := mutedStyle.Render(action.hint)
		if index == m.paletteAt {
			prefix = "┃ "
			label = screenStyle(screen(index % len(screenNames))).Bold(true).Render(action.label)
		}
		if height < 25 {
			lines = append(lines, prefix+label+"  "+hint)
		} else {
			lines = append(lines, prefix+label, "  "+hint)
		}
	}
	if len(paletteActions) > visible {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("Showing %d–%d of %d", start+1, min(len(paletteActions), start+visible), len(paletteActions))))
	}
	lines = append(lines, "", mutedStyle.Render("j/k navigate  enter select  esc close"))
	modalWidth := min(66, max(42, width-8))
	modal := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(colorAccent).
		Padding(1, 2).
		Width(modalWidth).
		Render(strings.Join(lines, "\n"))
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}

type keyHelpEntry struct {
	key, action string
}

func (m Model) keyHelpPanel(availableWidth, availableHeight int) string {
	entries := m.contextualKeyHelp()
	panelWidth := min(46, max(32, availableWidth-4))
	panelHeight := min(availableHeight, len(entries)+6)
	innerWidth := max(1, panelWidth-4)
	innerHeight := max(1, panelHeight-2)

	lines := []string{
		screenStyle(m.screen).Bold(true).Render(strings.ToUpper(screenNames[m.screen]) + " KEYS"),
		mutedStyle.Render("Contextual shortcuts"),
		"",
	}
	maxEntries := max(0, innerHeight-5)
	if len(entries) > maxEntries {
		maxEntries = max(0, innerHeight-6)
	}
	for _, entry := range entries[:min(len(entries), maxEntries)] {
		key := accentStyle.Bold(true).Render(fmt.Sprintf("%-8s", entry.key))
		actionStyle := textStyle
		if m.tasks == nil && entry.key == "x" {
			actionStyle = mutedStyle
		}
		lines = append(lines, key+actionStyle.Render(entry.action))
	}
	if len(entries) > maxEntries {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("… %d more at larger size", len(entries)-maxEntries)))
	}
	lines = append(lines, "", mutedStyle.Render("? / esc  close"))

	content := fitBlock(strings.Join(lines, "\n"), innerWidth, innerHeight)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(screenStyle(m.screen).GetForeground()).
		Width(max(1, panelWidth-2)).
		Height(max(1, panelHeight-2)).
		Render(content)
}

func (m Model) contextualKeyHelp() []keyHelpEntry {
	global := []keyHelpEntry{
		{"tab/S-tab", "Select page in navigation"},
		{"enter", "Open selected page or item"},
		{"esc", "Return one level"},
		{":", "Command center"},
		{"/", "Search current view"},
		{"r", "Reload database"},
		{"q", "Quit"},
	}
	var local []keyHelpEntry
	switch m.screen {
	case cockpitScreen:
		local = []keyHelpEntry{
			{"enter", "Edit and submit request"},
			{"v", "Toggle provider (Claude/Antigravity)"},
			{"m", "Cycle model"},
			{"f", "Select workspace files for context"},
			{"C", "Clear active session"},
		}
	case tasksScreen:
		local = []keyHelpEntry{
			{"h/l", "Select column"},
			{"j/k", "Select task"},
			{"enter", "Inspect task"},
			{"H/L", "Move task left/right"},
			{"a", "Create task"},
			{"d", "Delete task"},
			{"c", "Clear completed"},
			{"D", "AI Plan Builder"},
			{"v", "Toggle plan provider (Claude/Antigravity)"},
			{"m", "Cycle plan model"},
			{"A", "Cycle assigned agent"},
			{"R", "Export report"},
			{"x", m.orchestratorHelpAction()},
			{"X", m.autoApproveHelpAction()},
		}
	case agentsScreen:
		local = []keyHelpEntry{
			{"j/k", "Select agent"},
			{"enter", "Inspect agent"},
			{"n", "New agent (name prompt)"},
			{"d", "Delete agent"},
			{"R", "Reset agents to defaults"},
		}
	case pipelineScreen:
		local = []keyHelpEntry{
			{"x", m.orchestratorHelpAction()},
			{"p", "Run default 5-stage pipeline"},
		}
	case schedulerScreen:
		local = nil
	case operationsScreen:
		local = []keyHelpEntry{
			{"j/k", "Select event"},
			{"enter", "Inspect event"},
			{"x", m.orchestratorHelpAction()},
		}
	case settingsScreen:
		local = []keyHelpEntry{{"j/k", "Select settings section"}}
		switch m.settingsSection {
		case gitSection:
			local = append(local, keyHelpEntry{"r", "Refresh git status"}, keyHelpEntry{"b", "New branch"}, keyHelpEntry{"o", "Checkout branch"}, keyHelpEntry{"c", "Commit"}, keyHelpEntry{"V", "Revert commit"})
		case codeStudioSection:
			local = append(local, keyHelpEntry{"f", "Scan workspace files"}, keyHelpEntry{"J/K", "Move file cursor"}, keyHelpEntry{"enter", "Preview file"}, keyHelpEntry{"E", "Open in $EDITOR"}, keyHelpEntry{"S", "Stage file into cockpit context"})
		case browserSection:
			local = append(local, keyHelpEntry{"u", "Enter URL and run"}, keyHelpEntry{"t", "Toggle screenshot"})
		case integrationsSection:
			local = append(local, keyHelpEntry{"w", "Toggle webhook (confirm)"}, keyHelpEntry{"P", "Set webhook port"})
		case antigravitySection:
			local = append(local, keyHelpEntry{"a", "Set Antigravity key"})
		}
	}
	return append(local, global...)
}

func (m Model) orchestratorHelpAction() string {
	if m.tasks == nil {
		return "Orchestrator unavailable (read-only)"
	}
	if m.orchestratorRunning {
		return "Stop orchestrator"
	}
	return "Start orchestrator"
}

func (m Model) autoApproveHelpAction() string {
	if m.tasks == nil {
		return "Auto-approve unavailable (read-only)"
	}
	if m.autoApproveAll {
		return "Disable auto-approve all"
	}
	return "Enable auto-approve all"
}

func (m Model) cockpit(width, height int) string {
	if m.cockpitFilePicker {
		return m.cockpitFilePickerView(width, height)
	}
	running := m.snapshot.TaskCounts["running"]
	queued := m.snapshot.TaskCounts["queued"] + m.snapshot.TaskCounts["backlog"]
	done := m.snapshot.TaskCounts["done"]
	hero := screenStyle(cockpitScreen).Bold(true).Render("COCKPIT") + "\n" +
		textStyle.Bold(true).Render("What should the team build next?")
	prompt := m.cockpitPrompt
	if m.inputMode == 'p' {
		prompt = textStyle.Render(ansi.Hardwrap(m.input, max(1, width-6), false)) +
			focusStyle().Bold(true).Render("█")
	} else if prompt == "" {
		prompt = mutedStyle.Render("Press Enter and describe a feature, bug, or investigation.")
	} else {
		prompt = textStyle.Render(ansi.Hardwrap(truncateRunes(prompt, 1000), max(1, width-6), false))
	}
	state := successStyle.Render("READY")
	if m.tasks == nil {
		state = mutedStyle.Render("READ ONLY · run with --write to execute")
	} else if m.cockpitRunning {
		state = warningStyle.Render("RUNNING " + strings.ToUpper(m.cockpitProviderLabel()) + " CLI…")
	}
	composer := outlinedPanel("REQUEST · "+state, prompt+"\n\n"+
		mutedStyle.Render("Enter newline · Ctrl+Enter run · Esc cancel · v provider · m model · f files · C clear session"),
		width, min(8, max(6, height/3)), screenStyle(cockpitScreen).GetForeground())
	metrics := statusStyle("running").Bold(true).Render(fmt.Sprintf("● %d running", running)) + "  " +
		warningStyle.Render(fmt.Sprintf("◌ %d waiting", queued)) + "  " +
		successStyle.Render(fmt.Sprintf("✓ %d shipped", done))
	session := "none"
	if m.tasks != nil {
		if active := m.tasks.GetActiveSession(m.cockpitProvider); active != "" {
			session = active
		}
	}
	context := keyValue("Workspace", empty(m.snapshot.Workspace)) + "\n" +
		keyValue("Database", filepath.Base(m.dbPath)) + "\n" +
		keyValue("Team", fmt.Sprintf("%d agents", len(m.snapshot.Agents))) + "\n\n" +
		keyValue("Provider", m.cockpitProviderLabel()) + "\n" +
		keyValue("Model", m.cockpitModel()) + "\n" +
		keyValue("Session", session) + "\n" +
		keyValue("Files", fmt.Sprintf("%d selected", len(m.cockpitFiles)))
	result := m.cockpitResult
	if result == "" {
		result = mutedStyle.Render("Execution output will appear here.")
	} else {
		result = textStyle.Render(truncateRunes(result, 4000))
	}
	remaining := max(5, height-11)
	var lower string
	if width >= 76 {
		leftWidth := (width - 2) / 3
		lower = lipgloss.JoinHorizontal(lipgloss.Top,
			outlinedPanel("CONTEXT", context, leftWidth, remaining, colorBorder),
			strings.Repeat(" ", layoutPanelGap),
			outlinedPanel("RESULT", result, width-leftWidth-layoutPanelGap, remaining, colorBorder),
		)
	} else {
		lower = outlinedPanel("RESULT", result, width, remaining, colorBorder)
	}
	content := hero + "\n" + metrics + "\n\n" + composer + "\n" + lower
	return fitBlock(content, width, height)
}

func (m Model) cockpitFilePickerView(width, height int) string {
	header := screenStyle(cockpitScreen).Bold(true).Render("SELECT WORKSPACE FILES") + "\n" +
		mutedStyle.Render("j/k move · space toggle · c clear · enter/esc done")
	if len(m.cockpitAvailFiles) == 0 {
		return fitBlock(header+"\n\n"+mutedStyle.Render("No files found in workspace, or workspace not loading."), width, height)
	}
	visible := max(1, height-6)
	cursor := min(m.cockpitFileCursor, len(m.cockpitAvailFiles)-1)
	start := max(0, cursor-visible/2)
	if start+visible > len(m.cockpitAvailFiles) {
		start = max(0, len(m.cockpitAvailFiles)-visible)
	}
	var lines []string
	for index := start; index < min(len(m.cockpitAvailFiles), start+visible); index++ {
		file := m.cockpitAvailFiles[index]
		mark := "[ ]"
		if indexOfString(m.cockpitFiles, file) >= 0 {
			mark = "[x]"
		}
		line := mark + " " + file
		if index == cursor {
			line = focusStyle().Bold(true).Render("▌ " + mark + " " + file)
		} else {
			line = "  " + textStyle.Render(mark+" "+file)
		}
		lines = append(lines, line)
	}
	content := header + "\n\n" + strings.Join(lines, "\n") + "\n\n" +
		mutedStyle.Render(fmt.Sprintf("%d selected", len(m.cockpitFiles)))
	return fitBlock(content, width, height)
}

func (m Model) agents(width, height int) string {
	agents := m.visibleAgents()
	header := screenStyle(agentsScreen).Bold(true).Render("Agent Roster") + "  " +
		mutedStyle.Render(fmt.Sprintf("%d specialists", len(agents)))
	if len(agents) == 0 {
		return fitBlock(header+"\n\n"+mutedStyle.Render("No agents found in this workspace."), width, height)
	}
	lines := []string{
		header,
		mutedStyle.Render("j/k select · Enter inspect"),
		"",
		mutedStyle.Render(fmt.Sprintf("%-2s %-18s %-12s %-18s %s", "", "NAME", "STATUS", "MODEL", "CURRENT TASK")),
		mutedStyle.Render(strings.Repeat("─", width)),
	}
	visible := max(1, height-6)
	cursor := min(m.cursor[agentsScreen], len(agents)-1)
	start := max(0, cursor-visible/2)
	if start+visible > len(agents) {
		start = max(0, len(agents)-visible)
	}
	for index := start; index < min(len(agents), start+visible); index++ {
		agent := agents[index]
		prefix := "  "
		nameStyle := textStyle
		if index == cursor {
			prefix = focusStyle().Bold(true).Render("▌ ")
			nameStyle = focusStyle().Bold(true)
		}
		line := prefix +
			nameStyle.Render(fmt.Sprintf("%-18s", truncatePlain(agent.Name, 18))) + " " +
			statusStyle(agent.Status).Render(fmt.Sprintf("%-12s", strings.ToUpper(empty(agent.Status)))) + " " +
			mutedStyle.Render(fmt.Sprintf("%-18s", truncatePlain(agent.Model, 18))) + " " +
			textStyle.Render(truncatePlain(empty(agent.LastTask), max(8, width-56)))
		lines = append(lines, line)
	}
	return fitBlock(strings.Join(lines, "\n"), width, height)
}

func taskColumnWindow(selected, width int) (start, visible int) {
	visible = 1
	if width >= 90 {
		visible = len(taskStatuses)
	} else if width >= 64 {
		visible = 3
	}
	start = max(0, selected-visible/2)
	if start+visible > len(taskStatuses) {
		start = len(taskStatuses) - visible
	}
	return start, visible
}

func taskCardWindow(total, cursor, visible int) (start, count int) {
	count = max(1, visible)
	start = max(0, cursor-count/2)
	if start+count > total {
		start = max(0, total-count)
	}
	return start, count
}

func (m Model) visibleAgents() []models.Agent {
	visible := make(map[string]bool, len(m.displayRows[agentsScreen]))
	for _, item := range m.displayRows[agentsScreen] {
		visible[item.id] = true
	}
	agents := make([]models.Agent, 0, len(visible))
	for _, agent := range m.snapshot.Agents {
		if visible[agent.AgentID] {
			agents = append(agents, agent)
		}
	}
	return agents
}

func (m Model) agentCard(agent models.Agent, selected bool, width, height int) string {
	status := empty(agent.Status)
	prefix := statusGlyph(status)
	if selected {
		prefix = "◆"
	}
	limit := agent.TokenLimit
	if limit <= 0 {
		limit = 200000
	}
	content := statusStyle(status).Bold(true).Render(prefix+" "+empty(agent.Name)) + "\n" +
		mutedStyle.Render(empty(agent.Role)) + "  " + screenStyle(agentsScreen).Render(empty(agent.Model)) + "\n\n" +
		progressBar(agent.TokensUsed, limit, max(8, width-12)) +
		mutedStyle.Render(fmt.Sprintf("  %d/%d tokens", agent.TokensUsed, limit)) + "\n" +
		mutedStyle.Render("now  ") + textStyle.Render(ansi.Truncate(empty(agent.LastTask), max(8, width-9), "…"))
	marker := colorBorder
	if selected {
		marker = screenStyle(agentsScreen).GetForeground()
	}
	return outlinedPanel("", content, width, height, marker)
}

func (m Model) agentInspector(width, height int) string {
	rows := m.rows()
	if len(rows) == 0 {
		return fitBlock(mutedStyle.Render("No agent selected"), width, height)
	}
	item := rows[m.cursor[agentsScreen]]
	return fitBlock(
		screenStyle(agentsScreen).Bold(true).Render("Agent dossier")+"\n"+
			mutedStyle.Render("esc back to roster")+"\n\n"+
			m.inspectorHeader(item)+"\n\n"+textStyle.Render(truncateRunes(item.detail, 6000)),
		width, height,
	)
}

func (m Model) operations(width, height int) string {
	rows := m.rows()
	header := screenStyle(operationsScreen).Bold(true).Render("Live Operations") + "  " +
		m.orchestratorStatusStyle().Render(m.orchestratorStatus())
	filters := mutedStyle.Render("severity legend  ") + successStyle.Render("SUCCESS") + "  " +
		warningStyle.Render("WARN") + "  " + dangerStyle.Render("ERROR")
	lines := []string{header, filters, divider(width)}
	if len(rows) == 0 {
		lines = append(lines, "", mutedStyle.Render("Waiting for agent activity…"))
	} else {
		visible := max(1, height-5)
		cursor := min(m.cursor[operationsScreen], len(rows)-1)
		start := max(0, cursor-visible/2)
		if start+visible > len(rows) {
			start = max(0, len(rows)-visible)
		}
		for index := start; index < min(len(rows), start+visible); index++ {
			item := rows[index]
			prefix := "  "
			if index == cursor {
				prefix = screenStyle(operationsScreen).Bold(true).Render("▌ ")
			}
			level := operationLevel(item)
			lines = append(lines, prefix+
				mutedStyle.Render(operationTime(item.meta))+"  "+
				statusStyle(level).Bold(true).Render(fmt.Sprintf("%-7s", strings.ToUpper(level)))+" "+
				textStyle.Render(ansi.Truncate(item.title, max(1, width-22), "…")))
		}
	}
	return fitBlock(strings.Join(lines, "\n"), width, height)
}

func (m Model) operationInspector(width, height int) string {
	rows := m.rows()
	if len(rows) == 0 {
		return fitBlock(mutedStyle.Render("No event selected"), width, height)
	}
	item := rows[m.cursor[operationsScreen]]
	return fitBlock(
		screenStyle(operationsScreen).Bold(true).Render("Event detail")+"  "+
			statusStyle(operationLevel(item)).Render(strings.ToUpper(operationLevel(item)))+"\n"+
			mutedStyle.Render("esc back to stream")+"\n\n"+
			textStyle.Bold(true).Render(item.title)+"\n\n"+
			textStyle.Render(truncateRunes(item.detail, 6000)),
		width, height,
	)
}

func (m Model) pipeline(width, height int) string {
	stages := []struct {
		name, note, status string
	}{
		{"Plan", "Requirements and dependencies", "done"},
		{"Queue", fmt.Sprintf("%d tasks waiting", m.snapshot.TaskCounts["queued"]+m.snapshot.TaskCounts["backlog"]), "queued"},
		{"Execute", fmt.Sprintf("%d agents working", m.snapshot.AgentCounts["running"]), ternaryStatus(m.orchestratorRunning, "running", "idle")},
		{"Verify", fmt.Sprintf("%d tasks completed", m.snapshot.TaskCounts["done"]), "done"},
	}
	header := screenStyle(pipelineScreen).Bold(true).Render("Execution Pipeline") + "  " +
		m.orchestratorStatusStyle().Render(m.orchestratorStatus())
	if width >= 86 {
		stageWidth := max(15, (width-9)/4)
		var blocks []string
		for index, stage := range stages {
			if index > 0 {
				blocks = append(blocks, statusStyle(stage.status).Render("──▶"))
			}
			blocks = append(blocks, pipelineStage(stage.name, stage.note, stage.status, stageWidth, height-5))
		}
		return fitBlock(header+"\n"+mutedStyle.Render("x starts or stops the shared orchestrator")+"\n\n"+
			lipgloss.JoinHorizontal(lipgloss.Center, blocks...), width, height)
	}
	lines := []string{header, mutedStyle.Render("x starts or stops execution"), ""}
	for index, stage := range stages {
		lines = append(lines,
			statusStyle(stage.status).Bold(true).Render(statusGlyph(stage.status)+"  "+stage.name),
			"   "+mutedStyle.Render(stage.note),
		)
		if index < len(stages)-1 {
			lines = append(lines, "   "+mutedStyle.Render("│"), "   "+mutedStyle.Render("▼"))
		}
	}
	return fitBlock(strings.Join(lines, "\n"), width, height)
}

func pipelineStage(name, note, status string, width, height int) string {
	content := statusStyle(status).Bold(true).Render(statusGlyph(status)+" "+name) + "\n\n" +
		mutedStyle.Render(ansi.Truncate(note, max(1, width-3), "…"))
	return outlinedPanel(strings.ToUpper(name), content, width, max(5, height), statusStyle(status).GetForeground())
}

func (m Model) scheduler(width, height int) string {
	content := screenStyle(schedulerScreen).Bold(true).Render("Scheduler") + "\n" +
		mutedStyle.Render("Time-based automation") + "\n\n" +
		warningStyle.Bold(true).Render("Not available in the TUI yet") + "\n\n" +
		textStyle.Render("The desktop scheduler service is not connected to this terminal frontend.") + "\n" +
		mutedStyle.Render("No schedules are being created, simulated, or displayed here.")
	return fitBlock(content, width, height)
}

func (m Model) settings(width, height int) string {
	var navLines []string
	navLines = append(navLines, screenStyle(settingsScreen).Bold(true).Render("Settings"), mutedStyle.Render("j/k select section"), "")
	for index, name := range settingsSectionNames {
		rail := mutedStyle.Render("│")
		label := mutedStyle.Render(name)
		if index == m.settingsCursor {
			rail = focusStyle().Bold(true).Render("▌")
			label = focusStyle().Bold(true).Render(name)
		}
		navLines = append(navLines, rail+" "+label)
	}
	nav := strings.Join(navLines, "\n")
	detail := m.settingsDetailView(m.settingsCursor)
	if width < 76 {
		return fitBlock(nav+"\n"+divider(width)+"\n"+detail, width, height)
	}
	navWidth := 20
	return lipgloss.JoinHorizontal(lipgloss.Top,
		fitBlock(nav, navWidth, height),
		mutedStyle.Render("│"),
		fitBlock("  "+detail, width-navWidth-1, height),
	)
}

func (m Model) settingsDetailView(section int) string {
	switch section {
	case 1:
		return textStyle.Bold(true).Render("Providers") + "\n" +
			mutedStyle.Render("AI runtimes available to the shared backend") + "\n\n" +
			serviceLine("Claude CLI", true, "shared runner") + "\n" +
			serviceLine("Antigravity", true, "shared runner") + "\n\n" +
			warningStyle.Render("Provider editing is managed by the desktop app.")
	case 2:
		return textStyle.Bold(true).Render("Execution") + "\n" +
			mutedStyle.Render("Task repository and orchestrator state") + "\n\n" +
			serviceLine("Database", true, m.databaseMode()) + "\n" +
			serviceLine("Orchestrator", m.tasks != nil, m.orchestratorStatus()) + "\n\n" +
			keyValue("Mode", ternaryStatus(m.tasks != nil, "task write enabled", "read-only")) + "\n" +
			mutedStyle.Render("Use x on Task Board, Pipeline, or Operations to toggle execution.")
	case 3:
		webhookNote := fmt.Sprintf("port %d · w toggle (confirm) · P set port", m.webhookPort)
		if m.tasks == nil {
			webhookNote = "unavailable (read-only)"
		}
		return textStyle.Bold(true).Render("Integrations") + "\n" +
			mutedStyle.Render("External automation adapters") + "\n\n" +
			serviceLine("Scheduler", false, "adapter not connected") + "\n" +
			serviceLine("Webhook", m.webhookRunning, webhookNote) + "\n\n" +
			warningStyle.Render("Starting the webhook opens a local HTTP server; scheduler stays read-only.")
	case antigravitySection:
		return m.antigravityView()
	case gitSection:
		return m.gitView()
	case codeStudioSection:
		return m.codeStudioView()
	case browserSection:
		return m.browserView()
	case 8:
		return m.diagnosticsView()
	default:
		return textStyle.Bold(true).Render("Workspace") + "\n" +
			mutedStyle.Render("Paths and local application state") + "\n\n" +
			keyValue("Current", empty(m.snapshot.Workspace)) + "\n" +
			keyValue("Database", m.dbPath) + "\n" +
			keyValue("Access", m.databaseMode()) + "\n\n" +
			textStyle.Bold(true).Render("Service health") + "\n\n" +
			serviceLine("Database", true, m.databaseMode()) + "\n" +
			serviceLine("Execution", m.tasks != nil, m.orchestratorStatus())
	}
}

func (m Model) antigravityView() string {
	keys := cli.GlobalAntiPool.GetKeys()
	current := cli.GlobalAntiPool.GetCurrentAccount()
	currentName := "none"
	if current != nil {
		currentName = current.Name
	}
	lines := []string{
		textStyle.Bold(true).Render("Antigravity Key Pool"),
		mutedStyle.Render("OAuth accounts and API keys used for fallback rotation"),
		"",
		keyValue("Active keys", fmt.Sprintf("%d", len(keys))),
		keyValue("Current account", currentName),
		"",
		textStyle.Bold(true).Render("Accounts"),
		"",
	}
	for _, k := range keys {
		lines = append(lines, statusStyle(ternaryStatus(k.Status == "active", "done", "failed")).Render(strings.ToUpper(k.Status))+
			"  "+textStyle.Render(k.Name)+"  "+mutedStyle.Render("("+k.Type+", "+maskSecret(secretValue(k))+")"))
	}
	lines = append(lines, "", mutedStyle.Render("Press a to add a key or OAuth token (value masked, requires confirm)."))
	return strings.Join(lines, "\n")
}

func secretValue(k cli.AntiAccountKey) string {
	if k.Type == "oauth_token" {
		return k.OAuthToken
	}
	return k.APIKey
}

func maskSecret(value string) string {
	if value == "" {
		return "unset"
	}
	if len(value) <= 4 {
		return "••••"
	}
	return "••••" + value[len(value)-4:]
}

func (m Model) gitView() string {
	header := textStyle.Bold(true).Render("Git")
	if m.tasks == nil {
		return header + "\n\n" + warningStyle.Render("Git controls unavailable: start with --write.")
	}
	if m.gitLoading {
		return header + "\n\n" + mutedStyle.Render("Loading git status…")
	}
	if !m.gitLoaded {
		return header + "\n\n" + mutedStyle.Render("Press j/k to load, r to refresh.")
	}
	if m.gitError != "" {
		return header + "\n\n" + dangerStyle.Render(m.gitError)
	}
	lines := []string{
		header,
		mutedStyle.Render("r refresh · b new branch · o checkout · c commit · V revert"),
		"",
	}
	isRepo, _ := m.gitStatus["is_repo"].(bool)
	if !isRepo {
		lines = append(lines, mutedStyle.Render("Workspace is not a git repository."))
		return strings.Join(lines, "\n")
	}
	branch, _ := m.gitStatus["branch"].(string)
	changed, _ := m.gitStatus["changed_files"].(int)
	clean, _ := m.gitStatus["clean"].(bool)
	lines = append(lines,
		keyValue("Branch", branch),
		keyValue("Changed files", fmt.Sprintf("%d", changed)),
		keyValue("Clean", ternaryStatus(clean, "yes", "no")),
		"",
		textStyle.Bold(true).Render("Branches"),
		"",
	)
	if m.gitBranches != nil {
		for _, b := range m.gitBranches.Branches {
			marker := "  "
			if b == m.gitBranches.Current {
				marker = focusStyle().Bold(true).Render("▌ ")
			}
			lines = append(lines, marker+textStyle.Render(b))
		}
	}
	lines = append(lines, "", textStyle.Bold(true).Render("Recent commits"), "")
	for _, entry := range m.gitLog {
		lines = append(lines, mutedStyle.Render(entry.Hash)+" "+textStyle.Render(oneLine(entry.Message))+"  "+mutedStyle.Render(entry.Author+" · "+entry.Date))
	}
	return strings.Join(lines, "\n")
}

func (m Model) codeStudioView() string {
	header := textStyle.Bold(true).Render("Code Studio")
	if m.tasks == nil {
		return header + "\n\n" + warningStyle.Render("Code Studio unavailable: start with --write.")
	}
	lines := []string{
		header,
		mutedStyle.Render("f scan workspace · J/K move · enter preview · E open in $EDITOR · S stage into cockpit"),
		"",
	}
	if len(m.cockpitAvailFiles) == 0 {
		lines = append(lines, mutedStyle.Render("Press f to scan the workspace for files."))
		return strings.Join(lines, "\n")
	}
	visible := 12
	start := max(0, m.codeStudioCursor-visible/2)
	if start+visible > len(m.cockpitAvailFiles) {
		start = max(0, len(m.cockpitAvailFiles)-visible)
	}
	for index := start; index < min(len(m.cockpitAvailFiles), start+visible); index++ {
		file := m.cockpitAvailFiles[index]
		marker := "  "
		if index == m.codeStudioCursor {
			marker = focusStyle().Bold(true).Render("▌ ")
		}
		staged := ""
		if indexOfString(m.cockpitFiles, file) >= 0 {
			staged = mutedStyle.Render(" [staged]")
		}
		lines = append(lines, marker+textStyle.Render(file)+staged)
	}
	lines = append(lines, "", textStyle.Bold(true).Render("Preview"), "")
	if m.codeStudioLoading {
		lines = append(lines, mutedStyle.Render("Loading…"))
	} else if m.codeStudioPreviewErr != "" {
		lines = append(lines, dangerStyle.Render(m.codeStudioPreviewErr))
	} else if m.codeStudioFile != "" {
		lines = append(lines, mutedStyle.Render(m.codeStudioFile), "", textStyle.Render(truncateRunes(m.codeStudioPreview, 2000)))
	} else {
		lines = append(lines, mutedStyle.Render("No file previewed yet."))
	}
	return strings.Join(lines, "\n")
}

func (m Model) browserView() string {
	header := textStyle.Bold(true).Render("Browser Agent")
	if m.tasks == nil {
		return header + "\n\n" + warningStyle.Render("Browser agent unavailable: start with --write.")
	}
	lines := []string{
		header,
		mutedStyle.Render("u enter URL & run · t toggle screenshot"),
		"",
		keyValue("URL", empty(m.browserURL)),
		keyValue("Screenshot", ternaryStatus(m.browserScreenshot, "on", "off")),
		"",
	}
	if m.browserRunning {
		lines = append(lines, warningStyle.Render("Running headless Chrome… this can take up to 35s."))
		return strings.Join(lines, "\n")
	}
	if m.browserError != "" {
		lines = append(lines, dangerStyle.Render(m.browserError))
		return strings.Join(lines, "\n")
	}
	if m.browserResult == nil {
		lines = append(lines, mutedStyle.Render("No run yet."))
		return strings.Join(lines, "\n")
	}
	result := m.browserResult
	lines = append(lines,
		keyValue("Title", empty(result.Title)),
		keyValue("Success", ternaryStatus(result.Success, "yes", "no")),
	)
	if result.ScreenshotPath != "" {
		lines = append(lines, keyValue("Screenshot", result.ScreenshotPath))
	}
	if result.Error != "" {
		lines = append(lines, keyValue("Error", result.Error))
	}
	lines = append(lines, "", textStyle.Bold(true).Render("Page text"), "",
		textStyle.Render(truncateRunes(result.TextContent, 1200)))
	if len(result.Logs) > 0 {
		lines = append(lines, "", textStyle.Bold(true).Render("Log"), "")
		for _, entry := range result.Logs {
			lines = append(lines, mutedStyle.Render(entry))
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) diagnosticsView() string {
	cliPaths := cli.GetDetectedCLIPaths()
	return textStyle.Bold(true).Render("Diagnostics") + "\n" +
		mutedStyle.Render("Version, paths, and CLI resolution") + "\n\n" +
		keyValue("App version", version.GetVersion()) + "\n" +
		keyValue("Database", m.dbPath) + "\n" +
		keyValue("Workspace", empty(m.snapshot.Workspace)) + "\n\n" +
		textStyle.Bold(true).Render("CLI resolver") + "\n\n" +
		keyValue("Claude", cliPaths["claude"]) + "\n" +
		keyValue("Antigravity", cliPaths["antigravity"]) + "\n\n" +
		textStyle.Bold(true).Render("Runtime") + "\n\n" +
		keyValue("Go runtime", runtime.Version()) + "\n" +
		keyValue("OS/Arch", runtime.GOOS+"/"+runtime.GOARCH) + "\n" +
		keyValue("Goroutines", fmt.Sprintf("%d", runtime.NumGoroutine())) + "\n\n" +
		serviceLine("Orchestrator", m.tasks != nil, m.orchestratorStatus())
}

func (m Model) orchestratorStatus() string {
	if m.tasks == nil {
		return "orchestrator unavailable"
	}
	if m.orchestratorRunning {
		return "● orchestrator active"
	}
	return "○ orchestrator stopped"
}

func (m Model) orchestratorStatusStyle() lipgloss.Style {
	if m.orchestratorRunning {
		return successStyle
	}
	return mutedStyle
}

func (m Model) orchestratorStatusBadge(width int) string {
	label := "ORCHESTRATOR STOPPED"
	foreground := lipgloss.Color("#F2C66D")
	background := lipgloss.Color("#4A3D22")
	if m.tasks == nil {
		label = "ORCHESTRATOR OFFLINE"
		foreground = colorMuted
		background = lipgloss.Color("#34303A")
	} else if m.orchestratorRunning {
		label = "ORCHESTRATOR ACTIVE"
		foreground = lipgloss.Color("#5EE0A0")
		background = lipgloss.Color("#183D31")
	}
	return lipgloss.NewStyle().
		Foreground(foreground).
		Background(background).
		Bold(true).
		Align(lipgloss.Center).
		Width(width).
		Render(label)
}

func (m Model) databaseMode() string {
	if m.tasks != nil {
		return "task repository connected"
	}
	return "read-only connected"
}
