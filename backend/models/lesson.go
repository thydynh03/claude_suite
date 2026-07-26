package models

import "time"

// MemoryLesson is one distilled, injectable piece of cross-task knowledge —
// the "instinct" shape ported from ECC. Action is ONE line; that line is
// exactly what reaches a sub-agent's prompt, so an injected payload gets a
// sentence of reach, not a page.
type MemoryLesson struct {
	LessonID    string `json:"lesson_id"`
	WorkspaceID string `json:"workspace_id"` // "" = global (promoted)
	// Kind: mechanical lessons are derived deterministically from run history
	// and activate automatically; distilled lessons are LLM-written and stay
	// pending until a human approves them.
	Kind         string    `json:"kind"` // mechanical|distilled
	Trigger      string    `json:"trigger"`
	Action       string    `json:"action"`
	Evidence     string    `json:"evidence"`
	Confidence   float64   `json:"confidence"`
	Status       string    `json:"status"` // pending|active|archived
	SourceTaskID string    `json:"source_task_id"`
	UseCount     int       `json:"use_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	LastUsedAt   time.Time `json:"last_used_at"`
}

// Regression is one fixed bug: what broke, how it was fixed, and (once a
// human approves one) the falsifier check that guards it in
// .claude-suite/checks.json.
type Regression struct {
	RegressionID string   `json:"regression_id"`
	WorkspaceID  string   `json:"workspace_id"`
	Title        string   `json:"title"`
	Symptom      string   `json:"symptom"`
	RootCause    string   `json:"root_cause"`
	FixSummary   string   `json:"fix_summary"`
	Files        []string `json:"files"`
	GuardCheckID string   `json:"guard_check_id"`
	GuardStatus  string   `json:"guard_status"` // none|proposed|approved
	FailedTaskID string   `json:"failed_task_id"`
	FixedTaskID  string   `json:"fixed_task_id"`
	Status       string   `json:"status"` // active|archived
	CreatedAt    time.Time `json:"created_at"`
}
