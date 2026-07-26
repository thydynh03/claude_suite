package database

import (
	"database/sql"
	"encoding/json"
	"time"

	"claude_suite/backend/models"

	"github.com/google/uuid"
)

type TaskRepository struct {
	db *sql.DB
}

func NewTaskRepository(db *sql.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

func (r *TaskRepository) GetAll() ([]models.Task, error) {
	query := `SELECT task_id, title, description, prompt, priority, status, assigned_to, depends_on, retry_count, max_retries, result, session_id, parent_id, created_at, started_at, finished_at FROM tasks ORDER BY created_at ASC`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []models.Task
	for rows.Next() {
		var t models.Task
		var dependsJson string
		var rawCreated, rawStarted, rawFinished interface{}

		err := rows.Scan(
			&t.TaskID, &t.Title, &t.Description, &t.Prompt, &t.Priority, &t.Status,
			&t.AssignedTo, &dependsJson, &t.RetryCount, &t.MaxRetries, &t.Result,
			&t.SessionID, &t.ParentID, &rawCreated, &rawStarted, &rawFinished,
		)
		if err != nil {
			return nil, err
		}

		_ = json.Unmarshal([]byte(dependsJson), &t.DependsOn)
		if t.DependsOn == nil {
			t.DependsOn = []string{}
		}

		t.CreatedAt = parseTimeValue(rawCreated)
		t.StartedAt = parseTimeValue(rawStarted)
		t.FinishedAt = parseTimeValue(rawFinished)

		tasks = append(tasks, t)
	}

	return tasks, nil
}

func (r *TaskRepository) Create(t *models.Task) error {
	if t.TaskID == "" {
		t.TaskID = uuid.New().String()
	}
	if t.Status == "" {
		t.Status = "backlog"
	}
	if t.Priority == "" {
		t.Priority = "normal"
	}
	if t.MaxRetries == 0 {
		t.MaxRetries = 3
	}

	now := time.Now()
	t.CreatedAt = now

	dependsBytes, _ := json.Marshal(t.DependsOn)

	query := `INSERT INTO tasks (task_id, title, description, prompt, priority, status, assigned_to, depends_on, retry_count, max_retries, result, session_id, parent_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := r.db.Exec(query, t.TaskID, t.Title, t.Description, t.Prompt, t.Priority, t.Status, t.AssignedTo, string(dependsBytes), t.RetryCount, t.MaxRetries, t.Result, t.SessionID, t.ParentID, t.CreatedAt)
	return err
}

func (r *TaskRepository) UpdateStatus(taskID, status string, result string, sessionID string) error {
	now := time.Now()
	var query string
	var err error

	if status == "running" {
		query = `UPDATE tasks SET status=?, started_at=? WHERE task_id=?`
		_, err = r.db.Exec(query, status, now, taskID)
	} else if status == "done" || status == "failed" {
		query = `UPDATE tasks SET status=?, result=?, session_id=?, finished_at=? WHERE task_id=?`
		_, err = r.db.Exec(query, status, result, sessionID, now, taskID)
	} else {
		query = `UPDATE tasks SET status=? WHERE task_id=?`
		_, err = r.db.Exec(query, status, taskID)
	}

	return err
}

// RecordAttemptFailure increments the task's persisted retry counter and reports
// the new count with the task's limit.
//
// The counter has to live in the database: the orchestrator hands each run a
// copy of the task, so incrementing the struct only touches a value that is
// thrown away when the goroutine ends. Until this existed, retry_count stayed at
// zero, max_retries never applied, and a task that always fails re-ran forever —
// once per scan, each one a real paid agent invocation.
func (r *TaskRepository) RecordAttemptFailure(taskID string) (retries int, maxRetries int, err error) {
	if _, err = r.db.Exec(`UPDATE tasks SET retry_count = retry_count + 1 WHERE task_id = ?`, taskID); err != nil {
		return 0, 0, err
	}
	err = r.db.QueryRow(
		`SELECT retry_count, max_retries FROM tasks WHERE task_id = ?`, taskID,
	).Scan(&retries, &maxRetries)
	return retries, maxRetries, err
}

// RequeueWithDependency sends a task back to the backlog, makes it depend on
// depID, and bumps its retry counter — used by the autonomous E2E fix loop so a
// failed browser test re-runs only after its generated fix task completes.
func (r *TaskRepository) RequeueWithDependency(taskID, depID string) error {
	deps, _ := json.Marshal([]string{depID})
	_, err := r.db.Exec(
		`UPDATE tasks SET status='backlog', assigned_to='', depends_on=?, retry_count=retry_count+1 WHERE task_id=?`,
		string(deps), taskID,
	)
	return err
}

// ResetRunningTasks moves any task stuck in "running" (e.g. from a previous
// session that was killed mid-execution) back to the backlog on startup.
func (r *TaskRepository) ResetRunningTasks() (int64, error) {
	res, err := r.db.Exec(`UPDATE tasks SET status='backlog', assigned_to='' WHERE status='running'`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ResetForRetry moves a finished/failed task back to the backlog and clears its
// retry counter, result and timestamps so the orchestrator re-dispatches it.
func (r *TaskRepository) ResetForRetry(taskID string) error {
	_, err := r.db.Exec(
		`UPDATE tasks SET status='backlog', retry_count=0, result='', last_error='', started_at=NULL, finished_at=NULL WHERE task_id=?`,
		taskID,
	)
	if err != nil {
		// Fallback for schemas without a last_error column.
		_, err = r.db.Exec(
			`UPDATE tasks SET status='backlog', retry_count=0, result='' WHERE task_id=?`,
			taskID,
		)
	}
	return err
}

// GetByID returns one task, or (nil, nil) when it does not exist — a missing
// dependency row is normal (deleted tasks), not an error.
func (r *TaskRepository) GetByID(taskID string) (*models.Task, error) {
	var t models.Task
	var dependsJson string
	var rawCreated, rawStarted, rawFinished interface{}

	err := r.db.QueryRow(
		`SELECT task_id, title, description, prompt, priority, status, assigned_to, depends_on, retry_count, max_retries, result, session_id, parent_id, created_at, started_at, finished_at FROM tasks WHERE task_id = ?`,
		taskID,
	).Scan(
		&t.TaskID, &t.Title, &t.Description, &t.Prompt, &t.Priority, &t.Status,
		&t.AssignedTo, &dependsJson, &t.RetryCount, &t.MaxRetries, &t.Result,
		&t.SessionID, &t.ParentID, &rawCreated, &rawStarted, &rawFinished,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	_ = json.Unmarshal([]byte(dependsJson), &t.DependsOn)
	if t.DependsOn == nil {
		t.DependsOn = []string{}
	}
	t.CreatedAt = parseTimeValue(rawCreated)
	t.StartedAt = parseTimeValue(rawStarted)
	t.FinishedAt = parseTimeValue(rawFinished)
	return &t, nil
}

func (r *TaskRepository) AssignTask(taskID string, assignedTo string) error {
	_, err := r.db.Exec(`UPDATE tasks SET assigned_to=? WHERE task_id=?`, assignedTo, taskID)
	return err
}

func (r *TaskRepository) Delete(taskID string) error {
	_, err := r.db.Exec(`DELETE FROM tasks WHERE task_id=?`, taskID)
	return err
}

func (r *TaskRepository) DeleteAll() error {
	_, err := r.db.Exec(`DELETE FROM tasks`)
	return err
}

func (r *TaskRepository) DeleteDone() error {
	_, err := r.db.Exec(`DELETE FROM tasks WHERE status='done'`)
	return err
}

// NextDispatchable returns the highest priority task whose dependencies are all satisfied ('done')
func (r *TaskRepository) NextDispatchable() (*models.Task, error) {
	allTasks, err := r.GetAll()
	if err != nil {
		return nil, err
	}

	// Map completed tasks
	doneMap := make(map[string]bool)
	for _, t := range allTasks {
		if t.Status == "done" {
			doneMap[t.TaskID] = true
		}
	}

	// Priority sorting weight: high=3, normal=2, low=1
	priorityWeight := func(p string) int {
		switch p {
		case "high":
			return 3
		case "normal":
			return 2
		case "low":
			return 1
		default:
			return 2
		}
	}

	var candidate *models.Task

	for _, t := range allTasks {
		if t.Status != "backlog" && t.Status != "queued" {
			continue
		}

		// Check if all prerequisites are done
		allDepsDone := true
		for _, depID := range t.DependsOn {
			if !doneMap[depID] {
				allDepsDone = false
				break
			}
		}

		if !allDepsDone {
			continue
		}

		if candidate == nil || priorityWeight(t.Priority) > priorityWeight(candidate.Priority) {
			taskCopy := t
			candidate = &taskCopy
		}
	}

	return candidate, nil
}

// Blocker names one task that another is waiting on, and what state it is in.
type Blocker struct {
	TaskID string `json:"task_id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// BlockedTask is a task sitting in the backlog that cannot be dispatched because
// at least one of its dependencies is not done.
type BlockedTask struct {
	TaskID   string    `json:"task_id"`
	Title    string    `json:"title"`
	Blockers []Blocker `json:"blockers"`
	// Dead is true when at least one blocker has failed. Those never clear on
	// their own: the board looks busy while nothing can ever run.
	Dead bool `json:"dead"`
}

// DispatchReadiness explains why the backlog is or is not moving.
//
// NextDispatchable answers "what runs next" and returns nil for every reason at
// once — empty backlog, unmet dependencies, a dependency that failed. The UI
// showed the same nothing in all three cases, so a board with one task waiting
// on two failed ones looked identical to a board with no work at all.
type DispatchReadiness struct {
	Ready   int           `json:"ready"`
	Blocked []BlockedTask `json:"blocked"`
	Running int           `json:"running"`
}

func (r *TaskRepository) DispatchReadiness() (DispatchReadiness, error) {
	var out DispatchReadiness

	allTasks, err := r.GetAll()
	if err != nil {
		return out, err
	}

	byID := make(map[string]models.Task, len(allTasks))
	for _, t := range allTasks {
		byID[t.TaskID] = t
	}

	for _, t := range allTasks {
		if t.Status == "running" {
			out.Running++
		}
		if t.Status != "backlog" && t.Status != "queued" {
			continue
		}

		blocked := BlockedTask{TaskID: t.TaskID, Title: t.Title}
		for _, depID := range t.DependsOn {
			dep, known := byID[depID]
			if known && dep.Status == "done" {
				continue
			}
			b := Blocker{TaskID: depID, Status: "missing"}
			if known {
				b.Title, b.Status = dep.Title, dep.Status
			}
			// A dependency that is gone is as dead as one that failed: nothing
			// will ever mark it done.
			if b.Status == "failed" || b.Status == "missing" {
				blocked.Dead = true
			}
			blocked.Blockers = append(blocked.Blockers, b)
		}

		if len(blocked.Blockers) == 0 {
			out.Ready++
			continue
		}
		out.Blocked = append(out.Blocked, blocked)
	}

	return out, nil
}
