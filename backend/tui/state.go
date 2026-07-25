package tui

import (
	"fmt"
	"strings"
)

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
