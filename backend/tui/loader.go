package tui

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"claude_suite/backend/models"

	_ "modernc.org/sqlite"
)

var readableColumns = map[string][]string{
	"agents": {
		"agent_id", "name", "model", "system", "icon", "session_id", "status",
		"tasks_done", "last_task", "last_error", "notes", "tokens_used", "token_limit",
	},
	"tasks": {
		"task_id", "title", "description", "prompt", "priority", "status",
		"assigned_to", "depends_on", "retry_count", "max_retries", "result",
		"session_id", "parent_id", "created_at", "started_at", "finished_at",
	},
	"agent_memory": {
		"memory_id", "agent_id", "task_id", "session_id", "role", "content",
		"timestamp", "tokens_used",
	},
}

// LoadReadOnly reads one existing Claude Suite database without creating or
// migrating files. It supports the legacy Python schema used by the current DB.
func LoadReadOnly(path string) (Snapshot, error) {
	db, absolute, err := openReadOnly(path)
	if err != nil {
		return Snapshot{}, err
	}
	defer db.Close()

	if err := validateReadableSchema(db); err != nil {
		return Snapshot{}, err
	}
	agents, err := loadAgents(db)
	if err != nil {
		return Snapshot{}, err
	}
	tasks, err := loadTasks(db)
	if err != nil {
		return Snapshot{}, err
	}
	memory, err := loadRecentMemory(db)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		Agents:       agents,
		Tasks:        tasks,
		TaskCounts:   taskCounts(tasks),
		AgentCounts:  agentCounts(agents),
		RecentMemory: memory,
		Workspace:    loadWorkspace(filepath.Dir(absolute)),
	}, nil
}

// sqliteURI builds a SQLite "file:" URI for an absolute path.
//
// url.URL{Path: absolute} is wrong on Windows: String() percent-encodes the
// backslashes and emits file://C:%5CUsers%5C..., which SQLite reads as an
// authority and rejects with "invalid uri authority". SQLite wants forward
// slashes behind a leading one — file:///C:/Users/... or file:///home/... —
// which is what prefixing the slash-converted path produces on both platforms.
func sqliteURI(absolute, query string) string {
	p := filepath.ToSlash(absolute)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p, RawQuery: query}).String()
}

func openReadOnly(path string) (*sql.DB, string, error) {
	if strings.TrimSpace(path) == "" {
		return nil, "", fmt.Errorf("database path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, "", fmt.Errorf("resolve database path: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", fmt.Errorf("database does not exist: %s", absolute)
		}
		return nil, "", fmt.Errorf("stat database: %w", err)
	}
	if info.IsDir() {
		return nil, "", fmt.Errorf("database path is a directory: %s", absolute)
	}

	// immutable=1 prevents SQLite from checkpointing or removing WAL/SHM
	// sidecars when the read-only TUI opens and closes the database.
	dsn := sqliteURI(absolute, "mode=ro&immutable=1")
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, "", fmt.Errorf("open database read-only: %w", err)
	}
	if _, err := db.Exec("PRAGMA query_only=ON"); err != nil {
		_ = db.Close()
		return nil, "", fmt.Errorf("enable query-only mode: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, "", fmt.Errorf("read database: %w", err)
	}
	return db, absolute, nil
}

func validateReadableSchema(db *sql.DB) error {
	var problems []string
	for table, required := range readableColumns {
		rows, err := db.Query("PRAGMA table_info(" + table + ")")
		if err != nil {
			problems = append(problems, fmt.Sprintf("cannot inspect %s: %v", table, err))
			continue
		}
		found := map[string]bool{}
		for rows.Next() {
			var cid, notNull, primaryKey int
			var name, kind string
			var defaultValue any
			if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
				_ = rows.Close()
				return fmt.Errorf("read %s schema: %w", table, err)
			}
			found[name] = true
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("close %s schema rows: %w", table, err)
		}
		var missing []string
		for _, column := range required {
			if !found[column] {
				missing = append(missing, column)
			}
		}
		if len(missing) != 0 {
			problems = append(problems, fmt.Sprintf("%s missing columns: %s", table, strings.Join(missing, ", ")))
		}
	}
	if len(problems) != 0 {
		sort.Strings(problems)
		return fmt.Errorf("incompatible Claude Suite schema: %s", strings.Join(problems, "; "))
	}
	return nil
}

func loadAgents(db *sql.DB) ([]models.Agent, error) {
	rows, err := db.Query(`SELECT agent_id, name, model, COALESCE(system, ''),
		COALESCE(icon, ''), COALESCE(session_id, ''), COALESCE(status, 'idle'),
		COALESCE(tasks_done, 0), COALESCE(last_task, ''), COALESCE(last_error, ''),
		COALESCE(notes, ''), COALESCE(tokens_used, 0), COALESCE(token_limit, 200000)
		FROM agents ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()
	var agents []models.Agent
	for rows.Next() {
		var agent models.Agent
		if err := rows.Scan(
			&agent.AgentID, &agent.Name, &agent.Model, &agent.System, &agent.Icon,
			&agent.SessionID, &agent.Status, &agent.TasksDone, &agent.LastTask,
			&agent.LastError, &agent.Notes, &agent.TokensUsed, &agent.TokenLimit,
		); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		agent.Provider = "claude_cli"
		agent.TokenRemaining = agent.TokenLimit - agent.TokensUsed
		if agent.TokenRemaining < 0 {
			agent.TokenRemaining = 0
		}
		agents = append(agents, agent)
	}
	return agents, rows.Err()
}

func loadTasks(db *sql.DB) ([]models.Task, error) {
	rows, err := db.Query(`SELECT task_id, title, COALESCE(description, ''),
		COALESCE(prompt, ''), COALESCE(priority, 'normal'), COALESCE(status, 'backlog'),
		COALESCE(assigned_to, ''), COALESCE(depends_on, '[]'), COALESCE(retry_count, 0),
		COALESCE(max_retries, 3), COALESCE(result, ''), COALESCE(session_id, ''),
		COALESCE(parent_id, ''), COALESCE(created_at, ''), COALESCE(started_at, ''),
		COALESCE(finished_at, '') FROM tasks ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	var tasks []models.Task
	for rows.Next() {
		var task models.Task
		var dependsOn, createdAt, startedAt, finishedAt string
		if err := rows.Scan(
			&task.TaskID, &task.Title, &task.Description, &task.Prompt, &task.Priority,
			&task.Status, &task.AssignedTo, &dependsOn, &task.RetryCount,
			&task.MaxRetries, &task.Result, &task.SessionID, &task.ParentID,
			&createdAt, &startedAt, &finishedAt,
		); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		if err := json.Unmarshal([]byte(dependsOn), &task.DependsOn); err != nil {
			return nil, fmt.Errorf("decode dependencies for task %s: %w", task.TaskID, err)
		}
		task.CreatedAt = parseTime(createdAt)
		task.StartedAt = parseTime(startedAt)
		task.FinishedAt = parseTime(finishedAt)
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func loadRecentMemory(db *sql.DB) ([]models.MemoryItem, error) {
	rows, err := db.Query(`SELECT memory_id, agent_id, COALESCE(task_id, ''),
		COALESCE(session_id, ''), COALESCE(role, 'user'), content,
		COALESCE(timestamp, ''), COALESCE(tokens_used, 0)
		FROM agent_memory ORDER BY timestamp DESC LIMIT 8`)
	if err != nil {
		return nil, fmt.Errorf("list recent memory: %w", err)
	}
	defer rows.Close()
	var items []models.MemoryItem
	for rows.Next() {
		var item models.MemoryItem
		var timestamp string
		if err := rows.Scan(
			&item.MemoryID, &item.AgentID, &item.TaskID, &item.SessionID,
			&item.Role, &item.Content, &timestamp, &item.TokensUsed,
		); err != nil {
			return nil, fmt.Errorf("scan recent memory: %w", err)
		}
		item.Timestamp = parseTime(timestamp)
		items = append(items, item)
	}
	return items, rows.Err()
}

func parseTime(value string) time.Time {
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02T15:04:05.999999",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05",
	} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func taskCounts(tasks []models.Task) map[string]int {
	counts := map[string]int{}
	for _, status := range taskStatuses {
		counts[status] = 0
	}
	for _, task := range tasks {
		counts[task.Status]++
	}
	return counts
}

func agentCounts(agents []models.Agent) map[string]int {
	counts := map[string]int{}
	for _, status := range agentStatuses {
		counts[status] = 0
	}
	for _, agent := range agents {
		counts[agent.Status]++
	}
	return counts
}

func loadWorkspace(directory string) string {
	contents, err := os.ReadFile(filepath.Join(directory, "workspace_config.json"))
	if err != nil {
		return ""
	}
	var config struct {
		LastWorkspaceFolder string `json:"last_workspace_folder"`
	}
	if json.Unmarshal(contents, &config) != nil {
		return ""
	}
	return config.LastWorkspaceFolder
}
