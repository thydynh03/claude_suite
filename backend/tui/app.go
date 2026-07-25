package tui

import (
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"claude_suite/backend/cli"
	"claude_suite/backend/models"
	"claude_suite/backend/services"
	"claude_suite/backend/version"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type screen int

const (
	cockpitScreen screen = iota
	tasksScreen
	agentsScreen
	pipelineScreen
	schedulerScreen
	operationsScreen
	settingsScreen
)

var screenNames = []string{"Cockpit", "Task Board", "Agents", "Pipeline", "Scheduler", "Operations", "Settings"}
var settingsSectionNames = []string{"Workspace", "Providers", "Execution", "Integrations", "Antigravity", "Git", "Code Studio", "Browser", "Diagnostics"}

const (
	integrationsSection = 3
	antigravitySection  = 4
	gitSection          = 5
	codeStudioSection   = 6
	browserSection      = 7
)

var taskFilters = []string{"all", "backlog", "queued", "running", "done", "failed"}
var taskStatuses = []string{"backlog", "queued", "running", "done", "failed"}
var agentStatuses = []string{"idle", "running", "done", "error"}

const (
	layoutPanelGap = 2
)

type reloadMsg struct {
	snapshot Snapshot
	err      error
}

type reportMsg struct {
	path string
	err  error
}

type quickRunMsg struct {
	output string
	err    error
}

type filesLoadedMsg struct {
	files []string
	err   error
}

type gitLoadedMsg struct {
	status   map[string]interface{}
	branches *services.GitBranchInfo
	log      []services.GitCommitInfo
	err      error
}

type browserRunMsg struct {
	result BrowserResult
	err    error
}

type codeStudioPreviewMsg struct {
	file    string
	content string
	err     error
}

type codeStudioEditMsg struct {
	err error
}

type runtimeEventMsg RuntimeEvent

type row struct {
	id, title, meta, detail, status string
}

type rect struct {
	x, y, width, height int
}

func (r rect) contains(x, y int) bool {
	return x >= r.x && x < r.x+r.width && y >= r.y && y < r.y+r.height
}

type appLayout struct {
	header, navigation, content, canvas, footer rect
	compact                                     bool
}

type Model struct {
	snapshot Snapshot
	dbPath   string
	loader   Loader
	tasks    TaskActions

	screen          screen
	navFocus        bool
	navCursor       int
	settingsSection int
	settingsCursor  int
	cursor          [7]int
	taskColumn      int
	taskCursors     [5]int
	taskFilter      int
	baseRows        [7][]row
	displayRows     [7][]row
	inspect         bool
	width           int
	height          int

	inputMode            rune
	input                string
	query                string
	palette              bool
	paletteAt            int
	keyHelp              bool
	confirm              string
	approvalAgent        string
	approvalTask         string
	approvalTaskID       string
	status               string
	statusErr            bool
	reloading            bool
	updatedAt            time.Time
	liveEvents           []RuntimeEvent
	orchestratorRunning  bool
	autoApproveAll       bool
	cockpitPrompt        string
	cockpitResult        string
	cockpitRunning       bool
	cockpitProvider      string
	cockpitModelIdx      int
	cockpitFiles         []string
	cockpitFilePicker    bool
	cockpitFileCursor    int
	cockpitAvailFiles    []string
	planProvider         string
	planModelIdx         int
	pendingAntiKeyName   string
	pendingAntiKeySecret string
	gitLoaded            bool
	gitLoading           bool
	gitError             string
	gitStatus            map[string]interface{}
	gitBranches          *services.GitBranchInfo
	gitLog               []services.GitCommitInfo
	pendingGitBranch     string
	pendingGitMessage    string
	pendingGitHash       string
	webhookPort          int
	webhookRunning       bool
	browserURL           string
	browserScreenshot    bool
	browserRunning       bool
	browserResult        *BrowserResult
	browserError         string
	codeStudioCursor     int
	codeStudioFile       string
	codeStudioPreview    string
	codeStudioPreviewErr string
	codeStudioLoading    bool
}

var planClaudeModels = []string{"claude-opus-4-8", "claude-sonnet-4-5"}
var planAntiModels = []string{"gemini-3.1-pro-high", "gemini-3.6-flash-high"}

func (m Model) planModels() []string {
	if m.planProvider == "anti" {
		return planAntiModels
	}
	return planClaudeModels
}

func (m Model) planModel() string {
	models := m.planModels()
	if m.planModelIdx < 0 || m.planModelIdx >= len(models) {
		return models[0]
	}
	return models[m.planModelIdx]
}

func (m Model) planProviderLabel() string {
	if m.planProvider == "anti" {
		return "Antigravity"
	}
	return "Claude"
}

var cockpitClaudeModels = []string{"claude-sonnet-4-5", "claude-opus-4-8"}
var cockpitAntiModels = []string{"gemini-3.6-flash-high", "gemini-3.1-pro-high"}

func (m Model) cockpitModels() []string {
	if m.cockpitProvider == "anti" {
		return cockpitAntiModels
	}
	return cockpitClaudeModels
}

func (m Model) cockpitModel() string {
	models := m.cockpitModels()
	if m.cockpitModelIdx < 0 || m.cockpitModelIdx >= len(models) {
		return models[0]
	}
	return models[m.cockpitModelIdx]
}

func (m Model) cockpitProviderLabel() string {
	if m.cockpitProvider == "anti" {
		return "Antigravity"
	}
	return "Claude"
}

func New(snapshot Snapshot, path string, loadErr error) Model {
	return NewWithLoader(snapshot, path, loadErr, nil)
}

func NewWithLoader(snapshot Snapshot, path string, loadErr error, loader Loader) Model {
	return NewWithTaskActions(snapshot, path, loadErr, loader, nil)
}

func NewWithTaskActions(snapshot Snapshot, path string, loadErr error, loader Loader, tasks TaskActions) Model {
	m := Model{
		snapshot:        snapshot,
		dbPath:          path,
		loader:          loader,
		tasks:           tasks,
		cockpitProvider: "claude",
		planProvider:    "claude",
		webhookPort:     9090,
		updatedAt:       time.Now(),
		navCursor:       int(cockpitScreen),
		navFocus:        true,
	}
	if tasks != nil {
		m.orchestratorRunning = tasks.IsOrchestratorRunning()
		m.autoApproveAll = tasks.GetAutoApproveAll()
		m.webhookRunning = tasks.IsWebhookRunning()
	}
	if loadErr != nil {
		m.status, m.statusErr = loadErr.Error(), true
	}
	m.rebuildRows()
	m.focusUsefulTaskColumn()
	return m
}

func (m Model) Init() tea.Cmd {
	if m.tasks == nil {
		return nil
	}
	return waitRuntimeEvent(m.tasks.Events())
}

func waitRuntimeEvent(events <-chan RuntimeEvent) tea.Cmd {
	if events == nil {
		return nil
	}
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return nil
		}
		return runtimeEventMsg(event)
	}
}

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
		path, err := m.tasks.ExportReport()
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
				return m.tasks.GitCreateBranch(branch)
			})
		case "git-checkout-branch":
			branch := m.pendingGitBranch
			m.pendingGitBranch = ""
			return m.runGitMutation("Checking out "+branch+"…", func() error {
				return m.tasks.GitCheckoutBranch(branch)
			})
		case "git-commit":
			message := m.pendingGitMessage
			m.pendingGitMessage = ""
			return m.runGitMutation("Committing…", func() error {
				return m.tasks.GitCommit(message)
			})
		case "git-revert":
			hash := m.pendingGitHash
			m.pendingGitHash = ""
			return m.runGitMutation("Reverting "+hash+"…", func() error {
				return m.tasks.GitRevert(hash)
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
		status, statusErr := m.tasks.GitStatus()
		branches, branchErr := m.tasks.GitBranches()
		log, logErr := m.tasks.GitLog(8)
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
		status, err := tasks.GitStatus()
		if err != nil {
			return gitLoadedMsg{err: err}
		}
		branches, err := tasks.GitBranches()
		if err != nil {
			return gitLoadedMsg{err: err}
		}
		log, err := tasks.GitLog(8)
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

func (m *Model) switchScreen(next screen) {
	m.screen, m.inspect = next, false
	m.inputMode, m.input = 0, ""
	m.navCursor = int(next)
	m.clampCursor()
}

func (m *Model) move(delta int) {
	m.cursor[m.screen] += delta
	m.clampCursor()
}

func (m *Model) moveTaskColumn(delta int) {
	m.taskColumn = min(len(taskStatuses)-1, max(0, m.taskColumn+delta))
	m.cursor[tasksScreen] = m.taskCursors[m.taskColumn]
	m.clampCursor()
}

func (m *Model) moveTaskCard(delta int) {
	rows := m.rows()
	if len(rows) == 0 {
		m.taskCursors[m.taskColumn] = 0
		m.cursor[tasksScreen] = 0
		return
	}
	next := min(len(rows)-1, max(0, m.taskCursors[m.taskColumn]+delta))
	m.taskCursors[m.taskColumn] = next
	m.cursor[tasksScreen] = next
}

func (m *Model) focusUsefulTaskColumn() {
	for _, column := range []int{2, 1, 0, 4, 3} {
		if len(m.rowsForTaskColumn(column)) > 0 {
			m.taskColumn = column
			m.cursor[tasksScreen] = m.taskCursors[column]
			return
		}
	}
}

func (m *Model) setTaskFilter(index int) {
	if index < 0 {
		index = len(taskFilters) - 1
	}
	m.taskFilter = index % len(taskFilters)
	m.rebuildDisplayRows()
	m.cursor[tasksScreen] = 0
	m.inspect = false
}

func (m *Model) clampCursor() {
	count := len(m.rows())
	if count == 0 {
		m.cursor[m.screen] = 0
		return
	}
	m.cursor[m.screen] = min(max(0, m.cursor[m.screen]), count-1)
	if m.screen == tasksScreen {
		m.taskCursors[m.taskColumn] = m.cursor[tasksScreen]
	}
}

func (m *Model) rebuildRows() {
	m.baseRows = [7][]row{}
	for index, event := range m.liveEvents {
		m.baseRows[operationsScreen] = append(m.baseRows[operationsScreen], row{
			id: fmt.Sprintf("runtime-%d", index), title: oneLine(event.Message),
			meta:   "LIVE · " + strings.ToUpper(empty(event.Level)),
			detail: event.Message, status: strings.ToLower(event.Level),
		})
	}
	for _, item := range m.snapshot.RecentMemory {
		m.baseRows[operationsScreen] = append(m.baseRows[operationsScreen], row{
			id: item.MemoryID, title: oneLine(item.Content),
			meta:   strings.ToUpper(empty(item.Role)) + " · " + formatTime(item.Timestamp),
			detail: item.Content + "\n\nAgent: " + empty(item.AgentID) + "\nTask: " + empty(item.TaskID),
			status: item.Role,
		})
	}
	for _, task := range m.snapshot.Tasks {
		m.baseRows[tasksScreen] = append(m.baseRows[tasksScreen], row{
			id: task.TaskID, title: empty(task.Title),
			meta:   strings.ToUpper(empty(task.Status)) + " · " + strings.ToUpper(empty(task.Priority)) + " · " + empty(task.AssignedTo),
			detail: taskDetail(task), status: task.Status,
		})
	}
	for _, agent := range m.snapshot.Agents {
		m.baseRows[agentsScreen] = append(m.baseRows[agentsScreen], row{
			id: agent.AgentID, title: empty(agent.Name),
			meta:   strings.ToUpper(empty(agent.Status)) + " · " + empty(agent.Model),
			detail: agentDetail(agent), status: agent.Status,
		})
	}
	m.rebuildDisplayRows()
}

func (m *Model) rebuildDisplayRows() {
	m.displayRows = [7][]row{}
	needle := strings.ToLower(m.query)
	for target, items := range m.baseRows {
		for _, item := range items {
			if needle != "" && !strings.Contains(strings.ToLower(item.title+" "+item.meta+" "+item.detail), needle) {
				continue
			}
			m.displayRows[target] = append(m.displayRows[target], item)
		}
	}
}

func (m Model) rows() []row {
	if m.screen == tasksScreen {
		return m.rowsForTaskColumn(m.taskColumn)
	}
	return m.displayRows[m.screen]
}

func (m Model) rowsForTaskColumn(column int) []row {
	status := taskStatuses[column]
	var rows []row
	for _, item := range m.displayRows[tasksScreen] {
		if item.status == status {
			rows = append(rows, item)
		}
	}
	return rows
}

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
