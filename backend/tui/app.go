package tui

import (
	"time"

	"agent_center/backend/services"

	tea "charm.land/bubbletea/v2"
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
