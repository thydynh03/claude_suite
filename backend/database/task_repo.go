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
		var startedAt, finishedAt sql.NullTime

		err := rows.Scan(
			&t.TaskID, &t.Title, &t.Description, &t.Prompt, &t.Priority, &t.Status,
			&t.AssignedTo, &dependsJson, &t.RetryCount, &t.MaxRetries, &t.Result,
			&t.SessionID, &t.ParentID, &t.CreatedAt, &startedAt, &finishedAt,
		)
		if err != nil {
			return nil, err
		}

		_ = json.Unmarshal([]byte(dependsJson), &t.DependsOn)
		if t.DependsOn == nil {
			t.DependsOn = []string{}
		}

		if startedAt.Valid {
			t.StartedAt = startedAt.Time
		}
		if finishedAt.Valid {
			t.FinishedAt = finishedAt.Time
		}

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
