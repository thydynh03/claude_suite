package tui

import (
	"strings"
	"testing"

	"agent_center/backend/models"
	"agent_center/backend/services"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestNavigationAndFiltering(t *testing.T) {
	model := testModel()
	model.taskColumn = 0
	model.switchScreen(tasksScreen)
	model, _ = update(model, key("l"))
	model, _ = update(model, key("l"))
	if model.screen != tasksScreen || model.taskColumn != 2 {
		t.Fatalf("screen=%d column=%d", model.screen, model.taskColumn)
	}
	if got := model.rows(); len(got) != 1 || got[0].id != "t2" {
		t.Fatalf("column rows=%#v", got)
	}
}

func TestNavigationFocusAndPageSwitching(t *testing.T) {
	model := testModel()
	model.navFocus = true
	model.navCursor = int(tasksScreen)
	model.taskColumn = 3
	model, _ = update(model, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	if !model.navFocus || model.navCursor != int(agentsScreen) {
		t.Fatalf("tab navFocus=%v cursor=%d", model.navFocus, model.navCursor)
	}
	model, _ = update(model, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
	model, _ = update(model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if model.navFocus || model.screen != tasksScreen || model.taskColumn != 3 {
		t.Fatalf("enter navFocus=%v screen=%d column=%d", model.navFocus, model.screen, model.taskColumn)
	}
}

func TestCompactNavigationKeepsPagesDiscoverable(t *testing.T) {
	model := testModel()
	model.switchScreen(operationsScreen)
	model, _ = update(model, tea.WindowSizeMsg{Width: 80, Height: 24})
	view := model.View().Content
	for _, label := range []string{"Schedule", "[Ops]", "Settings"} {
		if !strings.Contains(view, label) {
			t.Fatalf("compact navigation missing %q", label)
		}
	}
}

func TestNavigationFocusHasVisibleIndication(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}} {
		model := testModel()
		model.switchScreen(tasksScreen)
		model.navFocus = true
		model.navCursor = int(tasksScreen)
		model, _ = update(model, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := model.View().Content
		if !strings.Contains(view, screenNames[tasksScreen]) {
			t.Fatalf("%dx%d navigation focus is invisible", size[0], size[1])
		}
		model, _ = update(model, key("j"))
		if model.navCursor != int(agentsScreen) || model.screen != tasksScreen {
			t.Fatalf("%dx%d nav cursor=%d screen=%d", size[0], size[1], model.navCursor, model.screen)
		}
	}
}

func TestOrchestratorStatusUsesAStableSidebarBadge(t *testing.T) {
	model := testModel()
	model.tasks = &fakeTaskActions{}
	for _, test := range []struct {
		running bool
		label   string
	}{
		{false, "ORCHESTRATOR STOPPED"},
		{true, "ORCHESTRATOR ACTIVE"},
	} {
		model.orchestratorRunning = test.running
		badge := model.orchestratorStatusBadge(20)
		if !strings.Contains(badge, test.label) || lipgloss.Width(badge) != 20 {
			t.Fatalf("running=%v badge=%q width=%d", test.running, badge, lipgloss.Width(badge))
		}
	}
}

func TestEnterMovesFocusFromSidebarToContent(t *testing.T) {
	model := testModel()
	model.navFocus = true
	model.navCursor = int(agentsScreen)
	model, _ = update(model, tea.WindowSizeMsg{Width: 120, Height: 30})
	before := model.View().Content
	model, _ = update(model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view := model.View().Content
	if model.navFocus || model.screen != agentsScreen {
		t.Fatalf("navFocus=%v screen=%d", model.navFocus, model.screen)
	}
	if before == view || !strings.Contains(view, "Agent Roster") {
		t.Fatal("content focus boxes do not distinguish focused and unfocused regions")
	}
}

func TestLegacyPageShortcutsAreRemoved(t *testing.T) {
	model := testModel()
	model.navFocus = true
	model, _ = update(model, key("7"))
	if model.screen != cockpitScreen || model.navCursor != int(cockpitScreen) {
		t.Fatal("number key unexpectedly changed page")
	}
	model, _ = update(model, tea.KeyPressMsg(tea.Key{Code: 'k', Mod: tea.ModCtrl}))
	if model.palette {
		t.Fatal("ctrl+k unexpectedly opened command center")
	}
}

func TestInputModes(t *testing.T) {
	model := testModel()
	model.switchScreen(operationsScreen)
	model, _ = update(model, key("/"))
	model, _ = update(model, key("review"))
	model, _ = update(model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if model.query != "review" || len(model.rows()) != 1 {
		t.Fatalf("query=%q rows=%d", model.query, len(model.rows()))
	}
}

func TestFrameHasExactDimensions(t *testing.T) {
	for _, size := range [][2]int{{60, 18}, {80, 24}, {120, 30}} {
		model := testModel()
		model, _ = update(model, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		lines := strings.Split(model.View().Content, "\n")
		if len(lines) != size[1] {
			t.Fatalf("%dx%d lines=%d", size[0], size[1], len(lines))
		}
		for _, line := range lines {
			if lipgloss.Width(line) != size[0] {
				t.Fatalf("%dx%d line width=%d", size[0], size[1], lipgloss.Width(line))
			}
		}
	}
}

func TestEveryScreenHasDistinctVisualIdentityAtTargetSizes(t *testing.T) {
	identities := []string{
		"What should the team build next?",
		"Task Board",
		"Agent Roster",
		"Execution Pipeline",
		"Not available in the TUI yet",
		"Live Operations",
		"Service health",
	}
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 42}} {
		for target, identity := range identities {
			model := testModel()
			model.switchScreen(screen(target))
			model, _ = update(model, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			view := model.View().Content
			if !strings.Contains(view, identity) {
				t.Fatalf("%dx%d screen %d missing identity %q", size[0], size[1], target, identity)
			}
			lines := strings.Split(view, "\n")
			if len(lines) != size[1] {
				t.Fatalf("%dx%d screen %d lines=%d", size[0], size[1], target, len(lines))
			}
			for _, line := range lines {
				if lipgloss.Width(line) != size[0] {
					t.Fatalf("%dx%d screen %d line width=%d", size[0], size[1], target, lipgloss.Width(line))
				}
			}
			layout := model.layout()
			bodyWidth, bodyHeight := size[0], size[1]-2
			bodyLines := strings.Split(model.body(layout), "\n")
			if len(bodyLines) != bodyHeight {
				t.Fatalf("%dx%d screen %d body lines=%d want=%d", size[0], size[1], target, len(bodyLines), bodyHeight)
			}
			for _, line := range bodyLines {
				if lipgloss.Width(line) != bodyWidth {
					t.Fatalf("%dx%d screen %d body width=%d want=%d", size[0], size[1], target, lipgloss.Width(line), bodyWidth)
				}
			}
		}
	}
}

func TestTopBarAndPagesHaveVisibleStructuralLines(t *testing.T) {
	for target := range screenNames {
		model := testModel()
		model.switchScreen(screen(target))
		model, _ = update(model, tea.WindowSizeMsg{Width: 120, Height: 30})
		header := model.header(120)
		if len(strings.Split(header, "\n")) != 1 || lipgloss.Width(header) != 120 {
			t.Fatalf("screen %d top bar is not a single visible bar", target)
		}
		body := model.body(model.layout())
		if panels := strings.Count(body, "╭"); panels < 1 {
			t.Fatalf("screen %d is missing its content boundary", target)
		}
	}
}

func TestPageCanvasesUseTheirEntireAllocatedArea(t *testing.T) {
	for _, size := range [][2]int{{72, 14}, {92, 22}, {150, 34}} {
		for target := range screenNames {
			model := testModel()
			model.switchScreen(screen(target))
			canvas := model.canvas(size[0], size[1])
			lines := strings.Split(canvas, "\n")
			if len(lines) != size[1] {
				t.Fatalf("%dx%d screen %d canvas lines=%d", size[0], size[1], target, len(lines))
			}
			for _, line := range lines {
				if lipgloss.Width(line) != size[0] {
					t.Fatalf("%dx%d screen %d canvas width=%d", size[0], size[1], target, lipgloss.Width(line))
				}
			}
		}
	}
}

func TestAgentRosterHonorsSearchFilter(t *testing.T) {
	model := testModel()
	model.switchScreen(agentsScreen)
	model.query = "missing-agent"
	model.rebuildDisplayRows()
	model, _ = update(model, tea.WindowSizeMsg{Width: 120, Height: 30})
	view := model.View().Content
	if !strings.Contains(view, "No agents found") || strings.Contains(view, "Planner") {
		t.Fatal("agent roster ignored active search filter")
	}
}

func TestSettingsSectionsAreNavigable(t *testing.T) {
	model := testModel()
	model.switchScreen(settingsScreen)
	model, _ = update(model, tea.WindowSizeMsg{Width: 120, Height: 30})
	model, _ = update(model, key("j"))
	if model.settingsCursor != 1 || model.settingsSection != 1 ||
		!strings.Contains(model.View().Content, "Claude CLI") {
		t.Fatal("moving cursor did not display providers")
	}
	model, _ = update(model, key("j"))
	if model.settingsSection != 2 || !strings.Contains(model.View().Content, "task write enabled") &&
		!strings.Contains(model.View().Content, "read-only") {
		t.Fatal("execution section did not display")
	}
	model, _ = update(model, key("j"))
	if model.settingsSection != 3 ||
		!strings.Contains(model.View().Content, "Starting the webhook opens a local HTTP server") {
		t.Fatal("integrations section did not display")
	}
}

func TestEnterOpensInspectorAndEscReturnsOneLevel(t *testing.T) {
	model := testModel()
	model.switchScreen(agentsScreen)
	model, _ = update(model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if !model.inspect {
		t.Fatal("enter did not open inspector")
	}
	model, _ = update(model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if !model.inspect {
		t.Fatal("enter unexpectedly toggled inspector closed")
	}
	model, _ = update(model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	if model.inspect || model.navFocus {
		t.Fatal("first esc did not return to agent list")
	}
	model, _ = update(model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	if !model.navFocus {
		t.Fatal("second esc did not return to global navigation")
	}
}

func TestCommandPaletteNavigatesWithoutQuitting(t *testing.T) {
	model := testModel()
	model, _ = update(model, key(":"))
	if !model.palette {
		t.Fatal(": did not open command palette")
	}
	model, _ = update(model, key("j"))
	model, _ = update(model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if model.palette || model.screen != tasksScreen {
		t.Fatalf("palette=%v screen=%d", model.palette, model.screen)
	}
}

func TestCommandCenterExposesTaskActions(t *testing.T) {
	model := testModel()
	model.palette = true
	model.paletteAt = 8
	model, _ = update(model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if model.palette || model.screen != tasksScreen || model.inputMode != 'a' || model.navFocus {
		t.Fatalf("palette=%v screen=%d input=%q nav=%v", model.palette, model.screen, model.inputMode, model.navFocus)
	}
}

func TestCockpitRunsQuickCLIThroughSharedService(t *testing.T) {
	actions := &fakeTaskActions{}
	model := NewWithTaskActions(testModel().snapshot, "agent_manager.db", nil, nil, actions)
	model.navFocus = false
	model.switchScreen(cockpitScreen)
	model, _ = update(model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if model.inputMode != 'p' {
		t.Fatal("enter did not focus cockpit prompt")
	}
	model, _ = update(model, key("Audit the repository"))
	model, command := update(model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Mod: tea.ModCtrl}))
	if command == nil || !model.cockpitRunning {
		t.Fatal("cockpit did not start quick execution")
	}
	message := command()
	model, _ = update(model, message)
	if actions.quickPrompt != "Audit the repository" || model.cockpitRunning ||
		!strings.Contains(model.cockpitResult, "completed") {
		t.Fatalf("prompt=%q running=%v result=%q", actions.quickPrompt, model.cockpitRunning, model.cockpitResult)
	}
}

func TestCockpitReadOnlyExplainsWhyExecutionIsUnavailable(t *testing.T) {
	model := testModel()
	model.switchScreen(cockpitScreen)
	model, _ = update(model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = update(model, key("Run this"))
	model, command := update(model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Mod: tea.ModCtrl}))
	if command != nil || !model.statusErr || !strings.Contains(model.status, "--write") {
		t.Fatalf("command=%v status=%q", command, model.status)
	}
}

func TestCockpitEditorRendersInsideRequestPanel(t *testing.T) {
	model := testModel()
	model.switchScreen(cockpitScreen)
	model, _ = update(model, tea.WindowSizeMsg{Width: 120, Height: 30})
	model, _ = update(model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = update(model, key("Inline request"))
	view := model.View().Content
	if !strings.Contains(view, "Inline request") || !strings.Contains(view, "█") {
		t.Fatal("cockpit editor is not visible in Request panel")
	}
	if strings.Contains(view, "Claude request:") {
		t.Fatal("cockpit editor leaked into the bottom bar")
	}
}

func TestViewEnablesMouseNavigation(t *testing.T) {
	model := testModel()
	model, _ = update(model, tea.WindowSizeMsg{Width: 120, Height: 30})
	if model.View().MouseMode != tea.MouseModeCellMotion {
		t.Fatal("mouse cell motion is not enabled")
	}
}

func TestMouseClickNavigatesWideSidebar(t *testing.T) {
	model := testModel()
	model.navFocus = true
	model, _ = update(model, tea.WindowSizeMsg{Width: 120, Height: 30})
	layout := model.layout()
	model, _ = update(model, tea.MouseClickMsg{X: layout.navigation.x + 5, Y: layout.navigation.y + 3 + int(agentsScreen), Button: tea.MouseLeft})
	if model.screen != agentsScreen || model.navFocus {
		t.Fatalf("screen=%d navFocus=%v", model.screen, model.navFocus)
	}
}

func TestMouseClickNavigatesCompactTabs(t *testing.T) {
	model := testModel()
	model.navFocus = true
	model, _ = update(model, tea.WindowSizeMsg{Width: 80, Height: 24})
	layout := model.layout()
	targetX := -1
	for x := 0; x < layout.navigation.width; x++ {
		if model.compactNavigationHit(x, layout.navigation.width) == int(tasksScreen) {
			targetX = x
			break
		}
	}
	if targetX == -1 {
		t.Fatal("Tasks tab is not visible")
	}
	model, _ = update(model, tea.MouseClickMsg{X: layout.navigation.x + targetX, Y: layout.navigation.y, Button: tea.MouseLeft})
	if model.screen != tasksScreen || model.navFocus {
		t.Fatalf("screen=%d navFocus=%v", model.screen, model.navFocus)
	}
}

func TestMouseClickFocusesCockpitRequest(t *testing.T) {
	model := testModel()
	model.switchScreen(cockpitScreen)
	model, _ = update(model, tea.WindowSizeMsg{Width: 120, Height: 30})
	layout := model.layout()
	model, _ = update(model, tea.MouseClickMsg{X: layout.canvas.x + 2, Y: layout.canvas.y + 4, Button: tea.MouseLeft})
	if model.inputMode != 'p' {
		t.Fatal("clicking Request did not focus inline editor")
	}
}

func TestMouseClickOpensSettingsSection(t *testing.T) {
	model := testModel()
	model.switchScreen(settingsScreen)
	model, _ = update(model, tea.WindowSizeMsg{Width: 120, Height: 30})
	layout := model.layout()
	model, _ = update(model, tea.MouseClickMsg{X: layout.canvas.x + 2, Y: layout.canvas.y + 4, Button: tea.MouseLeft})
	if model.settingsSection != 1 || model.settingsCursor != 1 {
		t.Fatalf("section=%d cursor=%d", model.settingsSection, model.settingsCursor)
	}
}

func TestMouseWheelMovesSidebarSelection(t *testing.T) {
	model := testModel()
	model.navFocus = true
	model, _ = update(model, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if model.navCursor != 1 {
		t.Fatalf("nav cursor=%d", model.navCursor)
	}
}

func TestMouseClickSelectsCommandCenterAction(t *testing.T) {
	model := testModel()
	model.palette = true
	model, _ = update(model, tea.WindowSizeMsg{Width: 120, Height: 30})
	originX, originY, width, height := model.canvasGeometry()
	visible := min(len(paletteActions), max(4, height-7))
	extra := 0
	if len(paletteActions) > visible {
		extra = 1
	}
	modalHeight := 3 + visible + extra + 2 + 4
	modalWidth := min(66, max(42, width-8))
	left := originX + max(0, (width-modalWidth)/2)
	top := originY + max(0, (height-modalHeight)/2)
	model, _ = update(model, tea.MouseClickMsg{X: left + 3, Y: top + 6, Button: tea.MouseLeft})
	if model.palette || model.screen != tasksScreen || model.navFocus {
		t.Fatalf("palette=%v screen=%d nav=%v", model.palette, model.screen, model.navFocus)
	}
}

func TestCommandPaletteFitsCompactTerminal(t *testing.T) {
	model := testModel()
	model.palette = true
	model, _ = update(model, tea.WindowSizeMsg{Width: 80, Height: 24})
	lines := strings.Split(model.View().Content, "\n")
	if len(lines) != 24 {
		t.Fatalf("palette rendered %d lines", len(lines))
	}
	if !strings.Contains(model.View().Content, "Open Cockpit") {
		t.Fatal("compact palette clipped actions")
	}
}

func TestResponsiveKanbanShowsColumns(t *testing.T) {
	model := testModel()
	model.switchScreen(tasksScreen)
	model.taskColumn = 2
	for _, test := range []struct {
		width int
		want  []string
	}{
		{60, []string{"RUNNING"}},
		{80, []string{"QUEUED", "RUNNING", "DONE"}},
		{90, []string{"BACKLOG", "QUEUED", "RUNNING", "DONE", "FAILED"}},
	} {
		view := model.kanban(test.width, 24)
		for _, status := range test.want {
			if !strings.Contains(view, status) {
				t.Fatalf("%d-column canvas missing %s", test.width, status)
			}
		}
	}
}

func TestCockpitEditorSupportsMultilineAndCtrlEnterSubmit(t *testing.T) {
	actions := &fakeTaskActions{}
	model := testModel()
	model.tasks = actions
	model.switchScreen(cockpitScreen)
	model, _ = update(model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = update(model, key("first line"))
	model, _ = update(model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = update(model, key("second line"))
	if model.input != "first line\nsecond line" || model.inputMode != 'p' {
		t.Fatalf("input=%q mode=%q", model.input, model.inputMode)
	}
	updated, command := update(model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Mod: tea.ModCtrl}))
	if updated.inputMode != 0 || command == nil {
		t.Fatal("ctrl+enter did not submit Cockpit request")
	}
	message := command()
	quick, ok := message.(quickRunMsg)
	if !ok || quick.err != nil || actions.quickPrompt != "first line\nsecond line" {
		t.Fatalf("message=%#v prompt=%q", message, actions.quickPrompt)
	}
}

func TestQuestionMarkTogglesContextualKeyHelp(t *testing.T) {
	model := testModel()
	model.switchScreen(tasksScreen)
	model, _ = update(model, tea.WindowSizeMsg{Width: 120, Height: 30})
	model, command := update(model, key("?"))
	if command != nil || !model.keyHelp {
		t.Fatal("? did not open key help")
	}
	view := model.View().Content
	for _, text := range []string{"TASK BOARD KEYS", "Select column", "AI Plan Builder"} {
		if !strings.Contains(view, text) {
			t.Fatalf("key help missing %q", text)
		}
	}
	model, _ = update(model, key("?"))
	if model.keyHelp {
		t.Fatal("? did not close key help")
	}
}

func TestKeyHelpIsContextualAndEscCloses(t *testing.T) {
	model := testModel()
	model.switchScreen(agentsScreen)
	model, _ = update(model, tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = update(model, key("?"))
	view := model.View().Content
	if !strings.Contains(view, "AGENTS KEYS") || strings.Contains(view, "AI Plan Builder") {
		t.Fatal("agent help was not contextual")
	}
	model, _ = update(model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	if model.keyHelp {
		t.Fatal("esc did not close key help")
	}
}

func TestKeyHelpPreservesExactFrameDimensions(t *testing.T) {
	for _, size := range [][2]int{{52, 14}, {80, 24}, {120, 30}} {
		model := testModel()
		model.switchScreen(tasksScreen)
		model, _ = update(model, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		model, _ = update(model, key("?"))
		lines := strings.Split(model.View().Content, "\n")
		if len(lines) != size[1] {
			t.Fatalf("%dx%d lines=%d", size[0], size[1], len(lines))
		}
		for _, line := range lines {
			if lipgloss.Width(line) != size[0] {
				t.Fatalf("%dx%d key help line width=%d", size[0], size[1], lipgloss.Width(line))
			}
		}
		if !strings.Contains(model.View().Content, "close") {
			t.Fatalf("%dx%d key help clipped its close hint", size[0], size[1])
		}
	}
}

func TestKanbanMoveUsesSharedTaskService(t *testing.T) {
	actions := &fakeTaskActions{}
	snapshot := testModel().snapshot
	model := NewWithTaskActions(snapshot, "agent_manager.db", nil, func(string) (Snapshot, error) {
		return snapshot, nil
	}, actions)
	model.navFocus = false
	model.switchScreen(tasksScreen)
	updated, command := model.Update(key("L"))
	model = updated.(Model)
	if command == nil {
		t.Fatal("move did not return a mutation command")
	}
	message := command()
	if _, ok := message.(reloadMsg); !ok {
		t.Fatalf("mutation message = %T", message)
	}
	if actions.updatedID != "t2" || actions.updatedStatus != "done" {
		t.Fatalf("updated %q to %q", actions.updatedID, actions.updatedStatus)
	}
}

func TestCreateTaskUsesSharedTaskService(t *testing.T) {
	actions := &fakeTaskActions{}
	snapshot := testModel().snapshot
	model := NewWithTaskActions(snapshot, "agent_manager.db", nil, func(string) (Snapshot, error) {
		return snapshot, nil
	}, actions)
	model.navFocus = false
	model.switchScreen(tasksScreen)
	model, _ = update(model, key("a"))
	model, _ = update(model, key("Ship terminal board"))
	model, command := update(model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if command == nil {
		t.Fatal("create did not return a mutation command")
	}
	_ = command()
	if actions.createdTitle != "Ship terminal board" {
		t.Fatalf("created title=%q", actions.createdTitle)
	}
}

func TestRuntimeApprovalCanBeResolved(t *testing.T) {
	actions := &fakeTaskActions{events: make(chan RuntimeEvent, 1)}
	model := NewWithTaskActions(testModel().snapshot, "agent_manager.db", nil, nil, actions)
	updated, command := model.Update(runtimeEventMsg(RuntimeEvent{
		Kind: "approval", TaskID: "task-42", Agent: "Architect", Task: "Design system",
	}))
	model = updated.(Model)
	if model.confirm != "approval" || command == nil {
		t.Fatalf("confirm=%q command=%v", model.confirm, command)
	}
	model, _ = update(model, key("y"))
	if !actions.approval || model.confirm != "" {
		t.Fatalf("approval=%v confirm=%q", actions.approval, model.confirm)
	}
	// The decision must name the task it belongs to. An empty id makes the
	// orchestrator resolve every pending approval, so with maxConcurrency > 1
	// approving one task would authorize the others too.
	if actions.approvalTaskID != "task-42" {
		t.Fatalf("approval routed to taskID %q, want %q", actions.approvalTaskID, "task-42")
	}
}

func TestRuntimeLogAppearsInOperations(t *testing.T) {
	actions := &fakeTaskActions{events: make(chan RuntimeEvent, 1)}
	model := NewWithTaskActions(testModel().snapshot, "agent_manager.db", nil, nil, actions)
	updated, _ := model.Update(runtimeEventMsg(RuntimeEvent{
		Kind: "log", Level: "SUCCESS", Message: "Agent completed task",
	}))
	model = updated.(Model)
	model.switchScreen(operationsScreen)
	if len(model.rows()) == 0 || model.rows()[0].title != "Agent completed task" {
		t.Fatalf("live rows=%#v", model.rows())
	}
}

func TestDeleteRequiresConfirmation(t *testing.T) {
	model := testModel()
	model.switchScreen(tasksScreen)
	model, command := update(model, key("d"))
	if command != nil || model.confirm != "delete" {
		t.Fatal("delete did not open confirmation")
	}
	model, _ = update(model, key("n"))
	if model.confirm != "" {
		t.Fatal("confirmation did not close")
	}
}

func TestLargeDetailDoesNotBlockUpdate(t *testing.T) {
	model := testModel()
	model.snapshot.RecentMemory[0].Content = strings.Repeat("large ", 100000)
	model.rebuildRows()
	model.switchScreen(operationsScreen)
	model, _ = update(model, tea.WindowSizeMsg{Width: 120, Height: 30})
	for index := 0; index < 1000; index++ {
		model, _ = update(model, key("j"))
		model, _ = update(model, key("k"))
	}
	if !strings.Contains(truncateRunes(model.rows()[0].detail, 6000), "Preview truncated") {
		t.Fatal("large inspector content was not capped")
	}
}

func testModel() Model {
	model := New(Snapshot{
		Tasks: []models.Task{
			{TaskID: "t1", Title: "Design database", Status: "backlog"},
			{TaskID: "t2", Title: "Review UI", Status: "running"},
		},
		Agents: []models.Agent{{AgentID: "a1", Name: "Planner", Status: "idle"}},
		RecentMemory: []models.MemoryItem{
			{MemoryID: "m1", Role: "assistant", Content: "Review complete"},
			{MemoryID: "m2", Role: "user", Content: "Design request"},
		},
	}, "agent_manager.db", nil)
	model.navFocus = false
	return model
}

func update(model Model, msg tea.Msg) (Model, tea.Cmd) {
	updated, command := model.Update(msg)
	return updated.(Model), command
}

func key(value string) tea.KeyPressMsg {
	runes := []rune(value)
	code := rune(0)
	if len(runes) == 1 {
		code = runes[0]
	}
	return tea.KeyPressMsg(tea.Key{Code: code, Text: value})
}

type fakeTaskActions struct {
	updatedID, updatedStatus string
	createdTitle             string
	quickPrompt              string
	quickProvider            string
	quickModel               string
	quickFiles               []string
	planRequirement          string
	planProvider             string
	planModel                string
	assignedID, assignedTo   string
	autoApprove              bool
	approvalTaskID           string
	savedAgent               models.Agent
	deletedAgentID           string
	agentsReset              bool
	pipelineRan              bool
	gitBranchCreated         string
	gitBranchCheckedOut      string
	gitCommitMessage         string
	gitRevertHash            string
	webhookRunning           bool
	webhookPort              int
	browserURL               string
	browserScreenshot        bool
	running, approval        bool
	events                   chan RuntimeEvent
	session                  map[string]string
}

func (f *fakeTaskActions) CreateTask(task models.Task) error {
	f.createdTitle = task.Title
	return nil
}
func (f *fakeTaskActions) UpdateTaskStatus(id, status string) error {
	f.updatedID, f.updatedStatus = id, status
	return nil
}
func (f *fakeTaskActions) AssignTask(taskID, assignedTo string) error {
	f.assignedID, f.assignedTo = taskID, assignedTo
	return nil
}
func (*fakeTaskActions) DeleteTask(string) error { return nil }
func (*fakeTaskActions) DeleteDoneTasks() error  { return nil }
func (*fakeTaskActions) ClearAllTasks() error    { return nil }
func (f *fakeTaskActions) DecomposePlanWithProvider(requirement, provider, model string) error {
	f.planRequirement, f.planProvider, f.planModel = requirement, provider, model
	return nil
}
func (*fakeTaskActions) ExportKanbanReport() (string, error) {
	return "report.md", nil
}
func (f *fakeTaskActions) RunQuickCLI(prompt, provider, model string, files []string) (string, error) {
	f.quickPrompt, f.quickProvider, f.quickModel, f.quickFiles = prompt, provider, model, files
	return "completed: " + prompt, nil
}
func (f *fakeTaskActions) GetActiveSession(provider string) string {
	if f.session == nil {
		return ""
	}
	return f.session[provider]
}
func (f *fakeTaskActions) ClearActiveSession(provider string) error {
	if f.session != nil {
		delete(f.session, provider)
	}
	return nil
}
func (f *fakeTaskActions) ScanWorkspaceFiles() ([]string, error) { return nil, nil }
func (f *fakeTaskActions) ReadFileContent(relPath string) (string, error) {
	return "fake content: " + relPath, nil
}
func (f *fakeTaskActions) StartOrchestrator() bool     { f.running = true; return f.running }
func (f *fakeTaskActions) StopOrchestrator() bool      { f.running = false; return f.running }
func (f *fakeTaskActions) IsOrchestratorRunning() bool { return f.running }
func (f *fakeTaskActions) ResolveApproval(taskID string, approved bool) {
	f.approvalTaskID, f.approval = taskID, approved
}
func (f *fakeTaskActions) SetAutoApproveAll(enabled bool) bool {
	f.autoApprove = enabled
	return enabled
}
func (f *fakeTaskActions) GetAutoApproveAll() bool { return f.autoApprove }
func (f *fakeTaskActions) SaveAgent(agent models.Agent) error {
	f.savedAgent = agent
	return nil
}
func (f *fakeTaskActions) DeleteAgent(agentID string) error {
	f.deletedAgentID = agentID
	return nil
}
func (f *fakeTaskActions) ResetAgentsToDefaults() error {
	f.agentsReset = true
	return nil
}
func (f *fakeTaskActions) RunPipeline() error {
	f.pipelineRan = true
	return nil
}
func (f *fakeTaskActions) GetGitStatus() (map[string]interface{}, error) {
	return map[string]interface{}{"is_repo": false}, nil
}
func (f *fakeTaskActions) GetGitBranches() (*services.GitBranchInfo, error) {
	return &services.GitBranchInfo{}, nil
}
func (f *fakeTaskActions) GetGitLog(limit int) ([]services.GitCommitInfo, error) { return nil, nil }
func (f *fakeTaskActions) CreateGitBranch(name string) error {
	f.gitBranchCreated = name
	return nil
}
func (f *fakeTaskActions) CheckoutGitBranch(name string) error {
	f.gitBranchCheckedOut = name
	return nil
}
func (f *fakeTaskActions) CreateGitCommit(message string) error {
	f.gitCommitMessage = message
	return nil
}
func (f *fakeTaskActions) RevertGitCommit(hash string) error {
	f.gitRevertHash = hash
	return nil
}
func (f *fakeTaskActions) ToggleWebhook(port int) bool {
	f.webhookRunning = !f.webhookRunning
	f.webhookPort = port
	return f.webhookRunning
}
func (f *fakeTaskActions) IsWebhookRunning() bool { return f.webhookRunning }
func (f *fakeTaskActions) RunBrowserTask(url string, screenshot bool) (BrowserResult, error) {
	f.browserURL, f.browserScreenshot = url, screenshot
	return BrowserResult{URL: url, Success: true}, nil
}
func (f *fakeTaskActions) Events() <-chan RuntimeEvent { return f.events }
