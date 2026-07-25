package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) View() tea.View {
	var content string
	if m.width <= 0 || m.height <= 0 {
		content = "Starting Claude Suite…"
	} else if m.width < 52 || m.height < 14 {
		content = exactFrame(centerText("◆ CLAUDE SUITE\n\nResize terminal to at least 52 × 14", m.width, m.height), m.width, m.height)
	} else {
		layout := m.layout()
		content = exactFrame(
			m.header(layout.header.width)+"\n"+
				m.body(layout)+"\n"+
				m.footer(layout.footer.width),
			m.width, m.height,
		)
	}
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "Claude Suite"
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

func (m Model) layout() appLayout {
	layout := appLayout{
		header: rect{0, 0, m.width, 1},
		footer: rect{0, max(0, m.height-1), m.width, 1},
	}
	bodyHeight := max(1, m.height-2)
	if m.width < 100 {
		layout.compact = true
		layout.navigation = rect{0, 1, m.width, min(3, bodyHeight)}
		layout.content = rect{0, 1 + layout.navigation.height, m.width, max(1, bodyHeight-layout.navigation.height)}
	} else {
		navigationWidth := 22
		layout.navigation = rect{0, 1, navigationWidth, bodyHeight}
		layout.content = rect{navigationWidth + 1, 1, max(1, m.width-navigationWidth-1), bodyHeight}
	}
	layout.canvas = rect{
		layout.content.x + 2,
		layout.content.y + 2,
		max(1, layout.content.width-4),
		max(1, layout.content.height-4),
	}
	return layout
}

func (m Model) header(width int) string {
	left := " " + prismaticText("CLAUDE SUITE") +
		mutedStyle.Render(" / ") + textStyle.Bold(true).Render(screenNames[m.screen])
	mode := successStyle.Bold(true).Render("READ ONLY")
	if m.tasks != nil {
		mode = warningStyle.Bold(true).Render("WRITE")
	}
	right := mutedStyle.Render(shortPath(m.snapshot.Workspace)) + "  " + mode
	return lipgloss.NewStyle().
		Background(colorSurface).
		Foreground(colorText).
		Bold(true).
		Render(fitLine(joinSides(left, right+" ", width), width))
}

func (m Model) body(layout appLayout) string {
	var content string
	border := colorBorder
	if !m.navFocus {
		border = colorFocus
	}
	page := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(1).
		Width(layout.content.width).
		Height(layout.content.height).
		Render(m.canvas(layout.canvas.width, layout.canvas.height))
	if layout.compact {
		navigation := fitBlock(
			m.compactNavigation(layout.navigation.width)+"\n"+
				mutedStyle.Render(strings.Repeat("─", layout.navigation.width)),
			layout.navigation.width, layout.navigation.height,
		)
		content = navigation + "\n" + page
	} else {
		navigation := m.sidebar(layout.navigation.width, layout.navigation.height)
		content = lipgloss.JoinHorizontal(lipgloss.Top,
			navigation,
			mutedStyle.Render("│"),
			page,
		)
	}
	if m.keyHelp {
		content = overlayBottomRight(content, m.keyHelpPanel(m.width, m.height-2), m.width, m.height-2)
	}
	return exactFrame(content, m.width, m.height-2)
}

func (m Model) sidebar(width, height int) string {
	lines := []string{"", mutedStyle.Render("  WORKSPACES"), ""}
	for index, name := range screenNames {
		rail := mutedStyle.Render("│")
		label := mutedStyle.Render(fmt.Sprintf("  %-14s", name))
		if screen(index) == m.screen {
			rail = screenStyle(screen(index)).Bold(true).Render("┃")
			label = screenStyle(screen(index)).Bold(true).Render(fmt.Sprintf("  %-14s", name))
		}
		if m.navFocus && index == m.navCursor {
			rail = focusStyle().Bold(true).Render("▌")
			label = lipgloss.NewStyle().
				Foreground(colorFocus).
				Background(colorSurface).
				Bold(true).
				Render(fmt.Sprintf("  %-14s", name))
		}
		state := " "
		switch screen(index) {
		case tasksScreen:
			if m.snapshot.TaskCounts["running"] > 0 {
				state = statusStyle("running").Render("●")
			} else if m.snapshot.TaskCounts["queued"]+m.snapshot.TaskCounts["backlog"] > 0 {
				state = warningStyle.Render("◌")
			}
		case operationsScreen:
			if len(m.liveEvents) > 0 {
				state = screenStyle(operationsScreen).Render("•")
			}
		}
		lines = append(lines, rail+label+state)
	}
	lines = append(lines,
		"",
		mutedStyle.Render("  ───────────────"),
		"",
		statusStyle("running").Render(fmt.Sprintf("  ● %d running", m.snapshot.TaskCounts["running"])),
		warningStyle.Render(fmt.Sprintf("  ◌ %d queued", m.snapshot.TaskCounts["queued"])),
		successStyle.Render(fmt.Sprintf("  ✓ %d complete", m.snapshot.TaskCounts["done"])),
		"",
		"  "+m.orchestratorStatusBadge(max(1, width-2)),
	)
	return fitBlock(strings.Join(lines, "\n"), width, height)
}

func (m Model) compactNavigation(width int) string {
	shortNames := []string{"Home", "Tasks", "Agents", "Flow", "Schedule", "Ops", "Settings"}
	active := int(m.screen)
	if m.navFocus {
		active = m.navCursor
	}
	start, visible := compactNavigationRange(active, width)
	var items []string
	if start > 0 {
		items = append(items, mutedStyle.Render("‹"))
	}
	for index := start; index < start+visible; index++ {
		label := shortNames[index]
		if m.navFocus && index == m.navCursor {
			items = append(items, lipgloss.NewStyle().
				Foreground(colorFocus).
				Background(colorSurface).
				Bold(true).
				Render(" "+label+" "))
		} else if index == int(m.screen) {
			items = append(items, screenStyle(screen(index)).Bold(true).Render("["+label+"]"))
		} else {
			items = append(items, mutedStyle.Render(label))
		}
	}
	if start+visible < len(shortNames) {
		items = append(items, mutedStyle.Render("›"))
	}
	return fitLine(strings.Join(items, "  "), width)
}

func (m Model) canvas(width, height int) string {
	if m.confirm != "" {
		return m.confirmationView(width, height)
	}
	if m.palette {
		return m.paletteView(width, height)
	}
	switch m.screen {
	case cockpitScreen:
		return m.cockpit(width, height)
	case tasksScreen:
		if m.inspect {
			return box("TASK INSPECTOR", m.inspector(), width, height, true)
		}
		return m.kanban(width, height)
	case agentsScreen:
		if m.inspect {
			return m.agentInspector(width, height)
		}
		return m.agents(width, height)
	case pipelineScreen:
		return m.pipeline(width, height)
	case schedulerScreen:
		return m.scheduler(width, height)
	case operationsScreen:
		if m.inspect {
			return m.operationInspector(width, height)
		}
		return m.operations(width, height)
	case settingsScreen:
		return m.settings(width, height)
	}
	if m.inspect {
		return box("INSPECTOR", m.inspector(), width, height, true)
	}
	if width < 92 {
		return box(m.listTitle(), m.list(), width, height, true)
	}
	leftWidth := width * 55 / 100
	rightWidth := width - leftWidth - layoutPanelGap
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		box(m.listTitle(), m.list(), leftWidth, height, true),
		strings.Repeat(" ", layoutPanelGap),
		box("INSPECTOR", m.inspector(), rightWidth, height, false),
	)
}

func (m Model) confirmationView(width, height int) string {
	title, message := "DELETE TASK", "Delete the selected task?"
	if m.confirm == "clear-done" {
		title, message = "CLEAR COMPLETED", "Delete every completed task?"
	} else if m.confirm == "delete-agent" {
		title, message = "DELETE AGENT", "Delete the selected agent?"
	} else if m.confirm == "reset-agents" {
		title, message = "RESET AGENTS", "Reset the agent roster to defaults?"
	} else if m.confirm == "add-antikey" {
		title, message = "ADD ANTIGRAVITY KEY", "Add key "+m.pendingAntiKeyName+" to the pool? (value hidden)"
	} else if m.confirm == "git-create-branch" {
		title, message = "CREATE BRANCH", "Create and switch to branch \""+m.pendingGitBranch+"\"?"
	} else if m.confirm == "git-checkout-branch" {
		title, message = "CHECKOUT BRANCH", "Switch to branch \""+m.pendingGitBranch+"\"?"
	} else if m.confirm == "git-commit" {
		title, message = "COMMIT CHANGES", "Stage all changes and commit with message:\n\""+m.pendingGitMessage+"\"?"
	} else if m.confirm == "git-revert" {
		title, message = "REVERT COMMIT", "Revert commit "+m.pendingGitHash+"? This creates a new commit."
	} else if m.confirm == "toggle-webhook" {
		if m.webhookRunning {
			title, message = "STOP WEBHOOK", fmt.Sprintf("Stop the webhook listener on port %d?", m.webhookPort)
		} else {
			title, message = "START WEBHOOK", fmt.Sprintf("Start a local HTTP webhook listener on port %d?", m.webhookPort)
		}
	} else if m.confirm == "approval" {
		title = "APPROVAL REQUIRED"
		message = empty(m.approvalAgent) + " wants to run:\n" + empty(m.approvalTask)
	}
	warning := mutedStyle.Render("This action cannot be undone.")
	actions := dangerStyle.Bold(true).Render("y  confirm") + "    " + mutedStyle.Render("n  cancel")
	if m.confirm == "approval" {
		warning = warningStyle.Render("Review the task before allowing execution.")
		actions = successStyle.Bold(true).Render("y  approve") + "    " + dangerStyle.Render("n  reject")
	}
	content := dangerStyle.Bold(true).Render(title) + "\n\n" +
		textStyle.Render(message) + "\n" +
		warning + "\n\n" +
		actions
	modal := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("#FF708A")).
		Padding(1, 3).
		Width(min(52, max(34, width-10))).
		Render(content)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}

func (m Model) kanban(width, height int) string {
	total := len(m.displayRows[tasksScreen])
	summary := screenStyle(tasksScreen).Bold(true).Render("Task Board") + "  " +
		mutedStyle.Render(fmt.Sprintf("%d tasks · h/l columns · j/k cards", total))
	rail := ""
	for index, status := range taskStatuses {
		count := len(m.rowsForTaskColumn(index))
		label := fmt.Sprintf("%s %d", statusGlyph(status), count)
		style := mutedStyle
		if index == m.taskColumn {
			style = statusStyle(status).Bold(true)
			label = "◆ " + strings.ToUpper(status) + fmt.Sprintf(" %d", count)
		}
		if rail != "" {
			rail += mutedStyle.Render("  ─  ")
		}
		rail += style.Render(label)
	}

	start, visible := taskColumnWindow(m.taskColumn, width)
	gap := (visible - 1) * layoutPanelGap
	columnWidth := max(12, (width-gap)/visible)
	columns := make([]string, 0, visible*2-1)
	for index := start; index < start+visible; index++ {
		if len(columns) > 0 {
			columns = append(columns, strings.Repeat(" ", layoutPanelGap))
		}
		columns = append(columns, m.kanbanColumn(index, columnWidth, max(1, height-3)))
	}
	board := lipgloss.JoinHorizontal(lipgloss.Top, columns...)
	return fitBlock(summary+"\n"+rail+"\n\n"+board, width, height)
}

func (m Model) kanbanColumn(column, width, height int) string {
	status := taskStatuses[column]
	var tasks []row
	for _, item := range m.displayRows[tasksScreen] {
		if item.status == status {
			tasks = append(tasks, item)
		}
	}
	active := column == m.taskColumn
	titleStyle := statusStyle(status).Bold(true)
	title := strings.ToUpper(status) + fmt.Sprintf("  %d", len(tasks))
	if active {
		title = "◆ " + title
	}
	lines := []string{titleStyle.Render(title), mutedStyle.Render(strings.Repeat("─", max(1, width-2))), ""}
	visibleCards := max(1, (height-4)/4)
	cursor := min(max(0, m.taskCursors[column]), max(0, len(tasks)-1))
	start, visibleCards := taskCardWindow(len(tasks), cursor, visibleCards)
	for index := start; index < min(len(tasks), start+visibleCards); index++ {
		item := tasks[index]
		prefix := "  "
		cardTitle := textStyle.Bold(true)
		if active && index == cursor {
			prefix = statusStyle(status).Bold(true).Render("▌") + " "
			cardTitle = titleStyle
		}
		lines = append(lines,
			prefix+cardTitle.Render(ansi.Truncate(item.title, max(1, width-4), "…")),
			"  "+priorityBadge(item.meta)+" "+mutedStyle.Render(ansi.Truncate(taskCardMeta(item.meta), max(1, width-8), "…")),
			"",
		)
	}
	if len(tasks) == 0 {
		lines = append(lines, mutedStyle.Render("  Empty"))
	}
	border := colorBorder
	if active {
		border = statusStyle(status).GetForeground()
	}
	return outlinedPanel("", strings.Join(lines, "\n"), width, height, border)
}
