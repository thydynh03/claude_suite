package tui

import (
	"fmt"
	"image/color"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"agent_center/backend/models"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) listTitle() string {
	title := strings.ToUpper(screenNames[m.screen])
	if m.screen == tasksScreen {
		title += "  " + accentStyle.Render("["+taskFilters[m.taskFilter]+"]")
	}
	if m.query != "" {
		title += "  " + warningStyle.Render("/"+m.query)
	}
	return title
}

func (m Model) list() string {
	rows := m.rows()
	if len(rows) == 0 {
		return mutedStyle.Render("\n  No matching items")
	}
	visible := max(1, m.height-10)
	cursor := m.cursor[m.screen]
	start := max(0, cursor-visible/2)
	if start+visible > len(rows) {
		start = max(0, len(rows)-visible)
	}
	var lines []string
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("%d items", len(rows))), "")
	for index := start; index < min(len(rows), start+visible); index++ {
		item := rows[index]
		prefix, titleStyle := "  ", textStyle
		if index == cursor {
			prefix, titleStyle = "┃ ", accentStyle.Bold(true)
		}
		lines = append(lines,
			prefix+statusStyle(item.status).Render(statusGlyph(item.status))+" "+titleStyle.Render(item.title),
			"  "+mutedStyle.Render(item.meta),
		)
	}
	return strings.Join(lines, "\n")
}

func (m Model) inspector() string {
	rows := m.rows()
	if len(rows) == 0 {
		return mutedStyle.Render("\n  Nothing selected")
	}
	item := rows[m.cursor[m.screen]]
	detail := truncateRunes(item.detail, 6000)
	title := accentStyle.Bold(true).Render(item.title)
	meta := statusStyle(item.status).Render(item.meta)
	return title + "\n" + meta + "\n\n" + textStyle.Render(detail)
}

func (m Model) footer(width int) string {
	status := m.status
	style := mutedStyle
	if status == "" {
		status = "Updated " + m.updatedAt.Format("15:04:05")
	} else if m.statusErr {
		style = dangerStyle
	} else {
		style = successStyle
	}
	if m.reloading {
		status = "◌ " + status
	}
	if m.inputMode != 0 && m.inputMode != 'p' {
		prompt := string(m.inputMode)
		if m.inputMode == 'a' {
			prompt = "New task: "
		} else if m.inputMode == 'D' {
			prompt = fmt.Sprintf("Plan requirement [%s/%s]: ", m.planProviderLabel(), m.planModel())
		} else if m.inputMode == 'p' {
			prompt = "Claude request: "
		} else if m.inputMode == 'N' {
			prompt = "New agent name: "
		} else if m.inputMode == 'K' {
			prompt = "Antigravity key (name:secret): "
		} else if m.inputMode == 'B' {
			prompt = "New branch name: "
		} else if m.inputMode == 'O' {
			prompt = "Checkout branch: "
		} else if m.inputMode == 'M' {
			prompt = "Commit message: "
		} else if m.inputMode == 'V' {
			prompt = "Revert commit hash: "
		} else if m.inputMode == 'P' {
			prompt = "Webhook port: "
		} else if m.inputMode == 'U' {
			prompt = "Browser agent URL: "
		}
		line := fitLine(screenStyle(m.screen).Render(prompt)+m.input+"_", width)
		return lipgloss.NewStyle().Background(colorSurface).Render(line)
	}
	focusHint := "Tab browse · Enter open · : commands · ? keys"
	if m.navFocus {
		focusHint = "Tab/Shift-Tab select · Enter open · : commands"
	} else {
		focusHint = "Esc sidebar · : commands · ? keys · q quit"
	}
	line := joinSides(" "+style.Render(status), mutedStyle.Render(focusHint)+" ", width)
	barStyle := lipgloss.NewStyle().Background(colorSurface)
	return barStyle.Render(fitLine(line, width))
}

func taskDetail(task models.Task) string {
	text := task.Description
	if formatted := formatStructuredPrompt(task.Prompt); formatted != "" {
		text += "\n\n" + formatted
	}
	if task.Result != "" {
		text += "\n\nRESULT\n" + task.Result
	}
	details := empty(text) + "\n\nTask: " + empty(task.TaskID) +
		"\nAssigned: " + empty(task.AssignedTo) +
		"\nSession: " + empty(task.SessionID) +
		"\nRetries: " + fmt.Sprintf("%d/%d", task.RetryCount, task.MaxRetries) +
		"\nCreated: " + formatTime(task.CreatedAt)
	if task.ParentID != "" {
		details += "\nParent: " + task.ParentID
	}
	if len(task.DependsOn) > 0 {
		details += "\nDepends on: " + strings.Join(task.DependsOn, ", ")
	}
	return details
}

type structuredPrompt struct {
	action, requirements, targetFiles, expectedOutput string
	ok                                                bool
}

func parseStructuredPrompt(prompt string) structuredPrompt {
	type tagPos struct {
		idx  int
		name string
	}
	tags := []tagPos{
		{strings.Index(prompt, "[ACTION]"), "action"},
		{strings.Index(prompt, "[REQUIREMENTS]"), "requirements"},
		{strings.Index(prompt, "[TARGET FILES]"), "files"},
		{strings.Index(prompt, "[EXPECTED OUTPUT]"), "output"},
	}
	for _, tag := range tags {
		if tag.idx < 0 {
			return structuredPrompt{}
		}
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i].idx < tags[j].idx })
	result := structuredPrompt{ok: true}
	for i, tag := range tags {
		end := len(prompt)
		if i+1 < len(tags) {
			end = tags[i+1].idx
		}
		segment := prompt[tag.idx:end]
		if colon := strings.Index(segment, ":"); colon >= 0 {
			segment = segment[colon+1:]
		}
		value := strings.TrimSpace(segment)
		switch tag.name {
		case "action":
			result.action = value
		case "requirements":
			result.requirements = value
		case "files":
			result.targetFiles = value
		case "output":
			result.expectedOutput = value
		}
	}
	return result
}

func formatStructuredPrompt(prompt string) string {
	parsed := parseStructuredPrompt(prompt)
	if !parsed.ok {
		return ""
	}
	label := accentStyle.Bold(true)
	return label.Render("[ACTION]") + " " + textStyle.Render(parsed.action) + "\n\n" +
		label.Render("[REQUIREMENTS]") + " " + textStyle.Render(parsed.requirements) + "\n\n" +
		label.Render("[TARGET FILES]") + " " + textStyle.Render(parsed.targetFiles) + "\n\n" +
		label.Render("[EXPECTED OUTPUT]") + " " + textStyle.Render(parsed.expectedOutput)
}

func agentDetail(agent models.Agent) string {
	limit := agent.TokenLimit
	if limit <= 0 {
		limit = 200000
	}
	return "Role: " + empty(agent.Role) +
		"\nModel: " + empty(agent.Model) +
		fmt.Sprintf("\nTokens: %d / %d\nTasks completed: %d", agent.TokensUsed, limit, agent.TasksDone) +
		"\n\nCURRENT WORK\n" + empty(agent.LastTask) +
		"\n\nNOTES\n" + empty(agent.Notes) +
		"\n\nLAST ERROR\n" + empty(agent.LastError)
}

var (
	prismColors = []color.Color{
		lipgloss.Color("#FF70C9"),
		lipgloss.Color("#C084FC"),
		lipgloss.Color("#7C9CFF"),
		lipgloss.Color("#58D5FF"),
		lipgloss.Color("#5EE0A0"),
		lipgloss.Color("#F2C66D"),
		lipgloss.Color("#FF8A70"),
	}
	colorAccent          = lipgloss.Color("#C084FC")
	colorFocus           = lipgloss.Color("#58D5FF")
	colorText            = lipgloss.Color("#F8F5FF")
	colorMuted           = lipgloss.Color("#918A9E")
	colorBorder          = lipgloss.Color("#45404E")
	colorSurface         = lipgloss.Color("#2B2633")
	colorInactiveSurface = lipgloss.Color("#211E26")
	accentStyle          = lipgloss.NewStyle().Foreground(colorAccent)
	textStyle            = lipgloss.NewStyle().Foreground(colorText)
	mutedStyle           = lipgloss.NewStyle().Foreground(colorMuted)
	successStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#5EE0A0"))
	warningStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#F2C66D"))
	dangerStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF708A"))
)

func screenStyle(value screen) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(prismColors[int(value)%len(prismColors)])
}

func focusStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(colorFocus)
}

func prismaticText(text string) string {
	runes := []rune(text)
	var result strings.Builder
	for index, char := range runes {
		result.WriteString(lipgloss.NewStyle().
			Bold(true).
			Foreground(prismColors[index%len(prismColors)]).
			Render(string(char)))
	}
	return result.String()
}

func metricCard(label string, value int, style lipgloss.Style, width int) string {
	content := style.Bold(true).Render(label) + "\n" + style.Bold(true).Render(fmt.Sprintf("%d", value))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(style.GetForeground()).
		Align(lipgloss.Center).
		Width(max(10, width-2)).
		Render(content)
}

func statusStyle(status string) lipgloss.Style {
	switch strings.ToLower(status) {
	case "done", "idle", "assistant", "success", "info":
		return successStyle
	case "running", "queued", "warn", "warning", "send":
		return warningStyle
	case "failed", "error":
		return dangerStyle
	default:
		return mutedStyle
	}
}

func statusGlyph(status string) string {
	switch strings.ToLower(status) {
	case "done":
		return "✓"
	case "running":
		return "●"
	case "queued":
		return "◌"
	case "failed", "error":
		return "×"
	case "assistant":
		return "◆"
	default:
		return "○"
	}
}

func box(title, content string, width, height int, focused bool) string {
	border := colorBorder
	if focused {
		border = colorAccent
		title = "◆ " + title
	}
	innerWidth := max(1, width-4)
	innerHeight := max(1, height-4)
	body := fitBlock(accentStyle.Bold(true).Render(title)+"\n\n"+content, innerWidth, innerHeight)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Width(max(1, width-2)).
		Height(max(1, height-2)).
		Render(body)
}

func outlinedPanel(title, content string, width, height int, border color.Color) string {
	width, height = max(4, width), max(3, height)
	innerWidth, innerHeight := max(1, width-4), max(1, height-2)
	if title != "" {
		titleLine := lipgloss.NewStyle().Foreground(border).Bold(true).Render(" " + title + " ")
		content = titleLine + "\n" + content
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		Width(width).
		Height(height).
		Render(fitBlock(content, innerWidth, innerHeight))
}

func focusRegion(title, content string, width, height int, focused bool, accent color.Color) string {
	width, height = max(4, width), max(3, height)
	border := colorBorder
	titleForeground := colorMuted
	titleBackground := colorInactiveSurface
	marker := "○"
	if focused {
		border = accent
		titleForeground = colorText
		titleBackground = colorSurface
		marker = "◆"
	}
	innerWidth, innerHeight := max(1, width-2), max(1, height-2)
	titleLine := lipgloss.NewStyle().
		Foreground(titleForeground).
		Background(titleBackground).
		Bold(focused).
		Render(fitLine(" "+marker+" "+title, innerWidth))
	body := fitBlock(content, innerWidth, max(1, innerHeight-1))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Width(width).
		Height(height).
		Render(titleLine + "\n" + body)
}

func exactFrame(content string, width, height int) string {
	lines := strings.Split(content, "\n")
	out := make([]string, height)
	for index := range out {
		if index < len(lines) {
			out[index] = fitLine(lines[index], width)
		} else {
			out[index] = strings.Repeat(" ", width)
		}
	}
	return strings.Join(out, "\n")
}

func fitBlock(content string, width, height int) string {
	return exactFrame(content, width, height)
}

func overlayBottomRight(base, overlay string, width, height int) string {
	baseLines := strings.Split(exactFrame(base, width, height), "\n")
	overlayLines := strings.Split(overlay, "\n")
	overlayHeight := min(height, len(overlayLines))
	overlayWidth := min(width, lipgloss.Width(overlay))
	startY := max(0, height-overlayHeight)
	startX := max(0, width-overlayWidth)
	for index := 0; index < overlayHeight; index++ {
		line := fitLine(overlayLines[index], overlayWidth)
		target := startY + index
		baseLines[target] = ansi.Cut(baseLines[target], 0, startX) +
			line +
			ansi.Cut(baseLines[target], startX+overlayWidth, width)
		baseLines[target] = fitLine(baseLines[target], width)
	}
	return strings.Join(baseLines, "\n")
}

func fitLine(line string, width int) string {
	line = ansi.Truncate(line, width, "")
	if gap := width - lipgloss.Width(line); gap > 0 {
		line += strings.Repeat(" ", gap)
	}
	return line
}

func joinSides(left, right string, width int) string {
	left = ansi.Truncate(left, max(1, width-lipgloss.Width(right)-1), "")
	return fitLine(left+strings.Repeat(" ", max(1, width-lipgloss.Width(left)-lipgloss.Width(right)))+right, width)
}

func centerText(text string, width, height int) string {
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, text)
}

func shortPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return "no workspace"
	}
	return filepath.Base(path)
}

func truncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "\n\nPreview truncated."
}

func oneLine(text string) string { return strings.Join(strings.Fields(text), " ") }
func truncatePlain(text string, width int) string {
	return ansi.Truncate(strings.Join(strings.Fields(text), " "), max(1, width), "…")
}
func taskCardMeta(meta string) string {
	parts := strings.Split(meta, " · ")
	if len(parts) > 1 {
		return strings.Join(parts[1:], " · ")
	}
	return meta
}

func priorityBadge(meta string) string {
	parts := strings.Split(meta, " · ")
	if len(parts) < 2 {
		return mutedStyle.Render("·")
	}
	priority := strings.ToLower(parts[1])
	style := mutedStyle
	switch priority {
	case "high", "urgent", "critical":
		style = dangerStyle
	case "normal", "medium":
		style = warningStyle
	case "low":
		style = successStyle
	}
	return style.Bold(true).Render(strings.ToUpper(priority[:min(len(priority), 1)]))
}

func progressBar(value, limit int64, width int) string {
	width = max(4, width)
	if limit <= 0 {
		limit = 1
	}
	filled := min(width, max(0, int(value*int64(width)/limit)))
	return successStyle.Render(strings.Repeat("━", filled)) +
		mutedStyle.Render(strings.Repeat("─", width-filled))
}

func divider(width int) string {
	return mutedStyle.Render(strings.Repeat("─", max(1, width)))
}

func keyValue(key, value string) string {
	return mutedStyle.Render(fmt.Sprintf("%-12s", key)) + textStyle.Render(value)
}

func serviceLine(name string, connected bool, detail string) string {
	glyph := dangerStyle.Render("○")
	if connected {
		glyph = successStyle.Render("●")
	}
	return glyph + "  " + textStyle.Bold(true).Render(fmt.Sprintf("%-12s", name)) + mutedStyle.Render(detail)
}

func ternaryStatus(condition bool, yes, no string) string {
	if condition {
		return yes
	}
	return no
}

func operationLevel(item row) string {
	level := strings.ToLower(item.status)
	switch level {
	case "assistant":
		return "success"
	case "user":
		return "info"
	case "":
		return "info"
	default:
		return level
	}
}

func operationTime(meta string) string {
	if strings.HasPrefix(meta, "LIVE") {
		return time.Now().Format("15:04")
	}
	parts := strings.Split(meta, " · ")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return "--:--"
}

func (m Model) inspectorHeader(item row) string {
	return statusStyle(item.status).Bold(true).Render(statusGlyph(item.status)+" "+item.title) + "\n" +
		mutedStyle.Render(item.meta)
}

func empty(text string) string {
	if strings.TrimSpace(text) == "" {
		return "-"
	}
	return text
}
func formatTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Format("01-02 15:04")
}
