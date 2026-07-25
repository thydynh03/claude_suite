package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampCursor()
		return m, nil
	case reloadMsg:
		m.reloading = false
		if msg.err != nil {
			m.status, m.statusErr = msg.err.Error(), true
			return m, nil
		}
		m.snapshot = msg.snapshot
		m.rebuildRows()
		if len(m.rowsForTaskColumn(m.taskColumn)) == 0 {
			m.focusUsefulTaskColumn()
		}
		m.updatedAt = time.Now()
		m.status, m.statusErr = "Database reloaded", false
		m.clampCursor()
		return m, nil
	case reportMsg:
		if msg.err != nil {
			m.status, m.statusErr = msg.err.Error(), true
		} else {
			m.status, m.statusErr = "Report exported: "+msg.path, false
		}
		return m, nil
	case quickRunMsg:
		m.cockpitRunning = false
		m.cockpitResult = msg.output
		if msg.err != nil {
			m.status, m.statusErr = msg.err.Error(), true
		} else {
			m.status, m.statusErr = "Quick execution completed", false
		}
		return m, nil
	case filesLoadedMsg:
		if msg.err != nil {
			m.status, m.statusErr = msg.err.Error(), true
		} else {
			m.cockpitAvailFiles = msg.files
			m.cockpitFileCursor = 0
		}
		return m, nil
	case browserRunMsg:
		m.browserRunning = false
		if msg.err != nil {
			m.browserError = msg.err.Error()
			m.browserResult = nil
		} else {
			m.browserError = ""
			result := msg.result
			m.browserResult = &result
		}
		return m, nil
	case codeStudioPreviewMsg:
		m.codeStudioLoading = false
		m.codeStudioFile = msg.file
		if msg.err != nil {
			m.codeStudioPreview, m.codeStudioPreviewErr = "", msg.err.Error()
		} else {
			m.codeStudioPreview, m.codeStudioPreviewErr = msg.content, ""
		}
		return m, nil
	case codeStudioEditMsg:
		if msg.err != nil {
			m.status, m.statusErr = msg.err.Error(), true
		} else {
			m.status, m.statusErr = "Editor closed", false
		}
		return m, nil
	case gitLoadedMsg:
		m.gitLoading = false
		m.gitLoaded = true
		if msg.err != nil {
			m.gitError = msg.err.Error()
		} else {
			m.gitError = ""
			m.gitStatus = msg.status
			m.gitBranches = msg.branches
			m.gitLog = msg.log
		}
		return m, nil
	case runtimeEventMsg:
		event := RuntimeEvent(msg)
		switch event.Kind {
		case "approval":
			m.approvalAgent, m.approvalTask, m.approvalTaskID = event.Agent, event.Task, event.TaskID
			m.confirm = "approval"
			m.status, m.statusErr = event.Agent+" is waiting for approval", false
		case "board":
			m.status, m.statusErr = "Task board updated by orchestrator", false
			return m, tea.Batch(waitRuntimeEvent(m.tasks.Events()), m.loadSnapshot())
		case "log":
			m.liveEvents = append([]RuntimeEvent{event}, m.liveEvents...)
			if len(m.liveEvents) > 100 {
				m.liveEvents = m.liveEvents[:100]
			}
			m.rebuildRows()
			m.status, m.statusErr = event.Message, strings.EqualFold(event.Level, "error")
		}
		return m, waitRuntimeEvent(m.tasks.Events())
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			return m.updateMouseClick(msg.X, msg.Y)
		}
	case tea.MouseWheelMsg:
		if msg.Button == tea.MouseWheelUp {
			return m.updateMouseWheel(-1)
		}
		if msg.Button == tea.MouseWheelDown {
			return m.updateMouseWheel(1)
		}
	case tea.KeyPressMsg:
		if m.confirm != "" {
			return m.updateConfirmation(msg)
		}
		if m.palette {
			return m.updatePalette(msg)
		}
		if m.inputMode != 0 {
			return m.updateInput(msg)
		}
		if m.cockpitFilePicker {
			return m.updateFilePicker(msg)
		}
		if m.keyHelp {
			switch msg.String() {
			case "?", "esc", "q":
				m.keyHelp = false
			}
			return m, nil
		}
		if m.navFocus {
			return m.updateNavigation(msg)
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.keyHelp = true
		case "tab":
			// Content owns Tab only when a page implements multiple focusable
			// controls. Esc always returns to application navigation.
		case ":":
			m.palette, m.paletteAt = true, 0
		case "h", "left":
			if m.screen == tasksScreen && !m.inspect {
				m.moveTaskColumn(-1)
			}
		case "l", "right":
			if m.screen == tasksScreen && !m.inspect {
				m.moveTaskColumn(1)
			}
		case "j", "down":
			if m.screen == settingsScreen && !m.inspect {
				m.settingsCursor = min(len(settingsSectionNames)-1, m.settingsCursor+1)
				m.settingsSection = m.settingsCursor
				if m.settingsSection == gitSection && !m.gitLoaded && !m.gitLoading && m.tasks != nil {
					m.gitLoading = true
					return m, m.loadGitInfo()
				}
				if m.settingsSection == codeStudioSection && len(m.cockpitAvailFiles) == 0 && m.tasks != nil {
					return m, m.loadWorkspaceFiles()
				}
			} else if m.screen == tasksScreen && !m.inspect {
				m.moveTaskCard(1)
			} else if !m.inspect {
				m.move(1)
			}
		case "k", "up":
			if m.screen == settingsScreen && !m.inspect {
				m.settingsCursor = max(0, m.settingsCursor-1)
				m.settingsSection = m.settingsCursor
				if m.settingsSection == gitSection && !m.gitLoaded && !m.gitLoading && m.tasks != nil {
					m.gitLoading = true
					return m, m.loadGitInfo()
				}
				if m.settingsSection == codeStudioSection && len(m.cockpitAvailFiles) == 0 && m.tasks != nil {
					return m, m.loadWorkspaceFiles()
				}
			} else if m.screen == tasksScreen && !m.inspect {
				m.moveTaskCard(-1)
			} else if !m.inspect {
				m.move(-1)
			}
		case "g", "home":
			m.cursor[m.screen] = 0
		case "G", "end":
			m.cursor[m.screen] = max(0, len(m.rows())-1)
		case "enter":
			if m.screen == cockpitScreen && !m.cockpitRunning {
				m.inputMode, m.input = 'p', m.cockpitPrompt
			} else if m.screen == settingsScreen && m.settingsSection == codeStudioSection && m.tasks != nil {
				return m.previewCodeStudioFile()
			} else if !m.inspect && len(m.rows()) > 0 {
				m.inspect = true
			}
		case "H", "shift+left":
			if m.screen == tasksScreen {
				return m.moveSelectedTask(-1)
			}
		case "L", "shift+right":
			if m.screen == tasksScreen {
				return m.moveSelectedTask(1)
			}
		case "a":
			if m.screen == tasksScreen {
				m.inputMode, m.input = 'a', ""
			}
			if m.screen == settingsScreen && m.settingsSection == antigravitySection {
				m.inputMode, m.input = 'K', ""
			}
		case "e":
			if m.screen == tasksScreen {
				m.status, m.statusErr = "Desktop Task Board does not expose task editing", true
			}
		case "D":
			if m.screen == tasksScreen {
				m.inputMode, m.input = 'D', ""
			}
		case "R":
			if m.screen == tasksScreen {
				return m.exportReport()
			}
			if m.screen == agentsScreen && m.tasks != nil {
				m.confirm = "reset-agents"
			}
		case "x":
			if m.screen == tasksScreen || m.screen == pipelineScreen || m.screen == operationsScreen {
				return m.toggleOrchestrator()
			}
		case "X":
			if m.screen == tasksScreen || m.screen == pipelineScreen || m.screen == operationsScreen {
				return m.toggleAutoApprove()
			}
		case "A":
			if m.screen == tasksScreen {
				return m.cycleAssignedAgent()
			}
		case "p":
			if m.screen == pipelineScreen {
				return m.runPipeline()
			}
		case "v":
			if m.screen == cockpitScreen {
				if m.cockpitProvider == "anti" {
					m.cockpitProvider = "claude"
				} else {
					m.cockpitProvider = "anti"
				}
				m.cockpitModelIdx = 0
			} else if m.screen == tasksScreen {
				if m.planProvider == "anti" {
					m.planProvider = "claude"
				} else {
					m.planProvider = "anti"
				}
				m.planModelIdx = 0
			}
		case "m":
			if m.screen == cockpitScreen {
				models := m.cockpitModels()
				m.cockpitModelIdx = (m.cockpitModelIdx + 1) % len(models)
			} else if m.screen == tasksScreen {
				models := m.planModels()
				m.planModelIdx = (m.planModelIdx + 1) % len(models)
			}
		case "C":
			if m.screen == cockpitScreen && m.tasks != nil {
				_ = m.tasks.ClearActiveSession(m.cockpitProvider)
				m.status, m.statusErr = "Session cleared for "+m.cockpitProviderLabel(), false
			}
		case "f":
			if m.screen == cockpitScreen && m.tasks != nil {
				m.cockpitFilePicker = true
				if len(m.cockpitAvailFiles) == 0 {
					return m, m.loadWorkspaceFiles()
				}
			}
			if m.screen == settingsScreen && m.settingsSection == codeStudioSection && m.tasks != nil {
				m.codeStudioCursor = 0
				return m, m.loadWorkspaceFiles()
			}
		case "J", "shift+down":
			if m.screen == settingsScreen && m.settingsSection == codeStudioSection && len(m.cockpitAvailFiles) > 0 {
				m.codeStudioCursor = min(len(m.cockpitAvailFiles)-1, m.codeStudioCursor+1)
			}
		case "K", "shift+up":
			if m.screen == settingsScreen && m.settingsSection == codeStudioSection {
				m.codeStudioCursor = max(0, m.codeStudioCursor-1)
			}
		case "E":
			if m.screen == settingsScreen && m.settingsSection == codeStudioSection && m.tasks != nil {
				return m.openCodeStudioFile()
			}
		case "S":
			if m.screen == settingsScreen && m.settingsSection == codeStudioSection && m.codeStudioCursor < len(m.cockpitAvailFiles) {
				file := m.cockpitAvailFiles[m.codeStudioCursor]
				if idx := indexOfString(m.cockpitFiles, file); idx >= 0 {
					m.cockpitFiles = append(append([]string{}, m.cockpitFiles[:idx]...), m.cockpitFiles[idx+1:]...)
				} else {
					m.cockpitFiles = append(m.cockpitFiles, file)
				}
				m.status, m.statusErr = "Cockpit context updated ("+fmt.Sprintf("%d", len(m.cockpitFiles))+" files)", false
			}
		case "d":
			if m.screen == tasksScreen {
				if len(m.rows()) > 0 {
					m.confirm = "delete"
				}
			}
			if m.screen == agentsScreen && m.tasks != nil {
				if len(m.rows()) > 0 {
					m.confirm = "delete-agent"
				}
			}
		case "n":
			if m.screen == agentsScreen && m.tasks != nil {
				m.inputMode, m.input = 'N', ""
			}
		case "c":
			if m.screen == tasksScreen {
				m.confirm = "clear-done"
			}
			if m.screen == settingsScreen && m.settingsSection == gitSection && m.tasks != nil {
				m.inputMode, m.input = 'M', ""
			}
		case "b":
			if m.screen == settingsScreen && m.settingsSection == gitSection && m.tasks != nil {
				m.inputMode, m.input = 'B', ""
			}
		case "o":
			if m.screen == settingsScreen && m.settingsSection == gitSection && m.tasks != nil {
				m.inputMode, m.input = 'O', ""
			}
		case "V":
			if m.screen == settingsScreen && m.settingsSection == gitSection && m.tasks != nil {
				m.inputMode, m.input = 'V', ""
			}
		case "w":
			if m.screen == settingsScreen && m.settingsSection == integrationsSection && m.tasks != nil {
				m.confirm = "toggle-webhook"
			}
		case "P":
			if m.screen == settingsScreen && m.settingsSection == integrationsSection && m.tasks != nil {
				m.inputMode, m.input = 'P', fmt.Sprintf("%d", m.webhookPort)
			}
		case "u":
			if m.screen == settingsScreen && m.settingsSection == browserSection && m.tasks != nil && !m.browserRunning {
				m.inputMode, m.input = 'U', m.browserURL
			}
		case "t":
			if m.screen == settingsScreen && m.settingsSection == browserSection {
				m.browserScreenshot = !m.browserScreenshot
			}
		case "esc":
			if m.inspect {
				m.inspect = false
			} else {
				m.navFocus = true
				m.navCursor = int(m.screen)
			}
		case "[":
			if m.screen == tasksScreen {
				m.moveTaskColumn(-1)
			}
		case "]":
			if m.screen == tasksScreen {
				m.moveTaskColumn(1)
			}
		case "/":
			m.inputMode, m.input = '/', m.query
		case "r":
			if m.screen == settingsScreen && m.settingsSection == gitSection && m.tasks != nil {
				return m, m.loadGitInfo()
			}
			return m.reload()
		}
	}
	return m, nil
}

func (m Model) updateNavigation(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab", "j", "down", "l", "right":
		m.navCursor = (m.navCursor + 1) % len(screenNames)
	case "shift+tab", "k", "up", "h", "left":
		m.navCursor = (m.navCursor + len(screenNames) - 1) % len(screenNames)
	case "enter":
		m.switchScreen(screen(m.navCursor))
		m.navFocus = false
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.keyHelp = true
	case ":":
		m.palette, m.paletteAt = true, 0
	}
	return m, nil
}

func (m Model) updateMouseClick(x, y int) (tea.Model, tea.Cmd) {
	if m.width <= 0 || m.height <= 0 || m.confirm != "" || m.keyHelp {
		return m, nil
	}
	if m.palette {
		return m.updatePaletteMouse(x, y)
	}
	layout := m.layout()
	if layout.navigation.contains(x, y) {
		m.navFocus = true
		if layout.compact {
			if target := m.compactNavigationHit(x-layout.navigation.x, layout.navigation.width); target >= 0 {
				m.navCursor = target
				m.switchScreen(screen(target))
				m.navFocus = false
			}
		} else if target := y - layout.navigation.y - 3; target >= 0 && target < len(screenNames) {
			m.navCursor = target
			m.switchScreen(screen(target))
			m.navFocus = false
		}
		return m, nil
	}
	if layout.content.contains(x, y) {
		m.navFocus = false
		return m.updatePageMouse(x-layout.canvas.x, y-layout.canvas.y)
	}
	return m, nil
}

func (m Model) updatePaletteMouse(x, y int) (tea.Model, tea.Cmd) {
	originX, originY, width, height := m.canvasGeometry()
	visible := min(len(paletteActions), max(4, height-7))
	start := max(0, m.paletteAt-visible/2)
	if start+visible > len(paletteActions) {
		start = max(0, len(paletteActions)-visible)
	}
	extra := 0
	if len(paletteActions) > visible {
		extra = 1
	}
	modalHeight := 3 + visible + extra + 2 + 4
	modalWidth := min(66, max(42, width-8))
	left := originX + max(0, (width-modalWidth)/2)
	top := originY + max(0, (height-modalHeight)/2)
	actionTop := top + 5
	if x < left || x >= left+modalWidth || y < actionTop || y >= actionTop+visible {
		return m, nil
	}
	m.paletteAt = start + y - actionTop
	return m.activatePalette()
}

func (m Model) canvasGeometry() (x, y, width, height int) {
	canvas := m.layout().canvas
	return canvas.x, canvas.y, canvas.width, canvas.height
}

func (m Model) updatePageMouse(x, y int) (tea.Model, tea.Cmd) {
	switch m.screen {
	case cockpitScreen:
		if y >= 4 && !m.cockpitRunning {
			m.inputMode, m.input = 'p', m.cockpitPrompt
		}
	case settingsScreen:
		if x < 20 {
			index := y - 3
			if index >= 0 && index < len(settingsSectionNames) {
				m.settingsCursor = index
				m.settingsSection = index
			}
		}
	case tasksScreen:
		if m.inspect {
			return m, nil
		}
		canvasWidth := m.pageCanvasWidth()
		start, visible := taskColumnWindow(m.taskColumn, canvasWidth)
		columnWidth := max(12, (canvasWidth-(visible-1)*layoutPanelGap)/visible)
		column := min(visible-1, max(0, x/(columnWidth+layoutPanelGap)))
		m.taskColumn = start + column
		if y >= 6 {
			rows := m.rowsForTaskColumn(m.taskColumn)
			cardStart, _ := taskCardWindow(
				len(rows),
				m.taskCursors[m.taskColumn],
				max(1, (max(1, m.layout().canvas.height-3)-4)/4),
			)
			card := cardStart + (y-6)/3
			if card < len(rows) {
				m.taskCursors[m.taskColumn] = card
				m.cursor[tasksScreen] = card
			}
		}
	case agentsScreen:
		if y >= 5 {
			index := y - 5
			if index < len(m.rows()) {
				m.cursor[agentsScreen] = index
			}
		}
	case operationsScreen:
		if y >= 3 {
			index := y - 3
			if index < len(m.rows()) {
				m.cursor[operationsScreen] = index
			}
		}
	}
	return m, nil
}

func (m Model) updateMouseWheel(delta int) (tea.Model, tea.Cmd) {
	if m.palette {
		m.paletteAt = min(len(paletteActions)-1, max(0, m.paletteAt+delta))
		return m, nil
	}
	if m.navFocus {
		m.navCursor = (m.navCursor + delta + len(screenNames)) % len(screenNames)
		return m, nil
	}
	if m.screen == settingsScreen {
		m.settingsCursor = min(len(settingsSectionNames)-1, max(0, m.settingsCursor+delta))
		m.settingsSection = m.settingsCursor
	} else if m.screen == tasksScreen && !m.inspect {
		m.moveTaskCard(delta)
	} else if !m.inspect {
		m.move(delta)
	}
	return m, nil
}

func (m Model) pageCanvasWidth() int {
	return m.layout().canvas.width
}

func compactNavigationRange(active, width int) (start, visible int) {
	visible = 3
	if width >= 76 {
		visible = 5
	}
	start = max(0, active-visible/2)
	if start+visible > len(screenNames) {
		start = len(screenNames) - visible
	}
	return start, visible
}

func (m Model) compactNavigationHit(x, width int) int {
	shortNames := []string{"Home", "Tasks", "Agents", "Flow", "Schedule", "Ops", "Settings"}
	active := int(m.screen)
	if m.navFocus {
		active = m.navCursor
	}
	start, visible := compactNavigationRange(active, width)
	position := 0
	if start > 0 {
		position = 3 // ‹ plus the two-space item gap.
	}
	for index := start; index < start+visible; index++ {
		itemWidth := utf8.RuneCountInString(shortNames[index])
		if index == active || index == int(m.screen) {
			itemWidth += 2
		}
		if x >= position && x < position+itemWidth {
			return index
		}
		position += itemWidth + layoutPanelGap
	}
	return -1
}
