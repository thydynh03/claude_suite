package models

import "time"

// TaskAttempt is one failed run of a task: which attempt it was and what the
// error actually said, so a retry can be told what went wrong before it.
type TaskAttempt struct {
	AttemptID string    `json:"attempt_id"`
	TaskID    string    `json:"task_id"`
	AttemptNo int       `json:"attempt_no"`
	Error     string    `json:"error"`
	CreatedAt time.Time `json:"created_at"`
}
