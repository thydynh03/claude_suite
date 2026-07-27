package database

import (
	"database/sql"

	"agent_center/backend/models"

	"github.com/google/uuid"
)

// AttemptRepository records what each failed attempt of a task actually said.
// tasks.retry_count only counts failures; the error text used to exist solely
// in the moment, so every retry re-ran a byte-identical prompt with no idea
// what went wrong the last time.
type AttemptRepository struct {
	db *sql.DB
}

func NewAttemptRepository(db *sql.DB) *AttemptRepository {
	return &AttemptRepository{db: db}
}

// Add records one failed attempt. The attempt number is derived from the rows
// already present, not from tasks.retry_count: the manual Retry button resets
// that counter (ResetForRetry) while these rows survive, and the two must not
// desynchronize.
func (r *AttemptRepository) Add(taskID, errText string) error {
	_, err := r.db.Exec(
		`INSERT INTO task_attempts (attempt_id, task_id, attempt_no, error)
		 VALUES (?, ?, (SELECT COALESCE(MAX(attempt_no), 0) + 1 FROM task_attempts WHERE task_id = ?), ?)`,
		uuid.New().String(), taskID, taskID, errText,
	)
	return err
}

// ListByTask returns the task's failed attempts, newest first.
func (r *AttemptRepository) ListByTask(taskID string, limit int) ([]models.TaskAttempt, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.db.Query(
		`SELECT attempt_id, task_id, attempt_no, error, created_at
		 FROM task_attempts WHERE task_id = ? ORDER BY attempt_no DESC LIMIT ?`,
		taskID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attempts []models.TaskAttempt
	for rows.Next() {
		var a models.TaskAttempt
		var rawCreated interface{}
		if err := rows.Scan(&a.AttemptID, &a.TaskID, &a.AttemptNo, &a.Error, &rawCreated); err != nil {
			return nil, err
		}
		a.CreatedAt = parseTimeValue(rawCreated)
		attempts = append(attempts, a)
	}
	return attempts, rows.Err()
}
