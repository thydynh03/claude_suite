package models

import (
	"regexp"
	"strings"
	"time"
)

// Task represents a Kanban task or decomposed plan item
type Task struct {
	TaskID      string    `json:"task_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Prompt      string    `json:"prompt"`
	Priority    string    `json:"priority"`   // "high", "normal", "low"
	Status      string    `json:"status"`     // "backlog", "queued", "running", "done", "failed"
	AssignedTo  string    `json:"assigned_to"` // Agent ID or Role
	DependsOn   []string  `json:"depends_on"`  // Slice of prerequisite task_ids
	RetryCount  int       `json:"retry_count"`
	MaxRetries  int       `json:"max_retries"`
	Result      string    `json:"result"`
	SessionID   string    `json:"session_id"`
	ParentID    string    `json:"parent_id"`
	CreatedAt   time.Time `json:"created_at"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at"`
}

// ExtractTags automatically parses tags from title, description, and prompt
func (t *Task) ExtractTags() []string {
	combined := strings.ToLower(t.Title + " " + t.Description + " " + t.Prompt)
	re := regexp.MustCompile(`\[([a-z0-9_]+)\]`)
	matches := re.FindAllStringSubmatch(combined, -1)

	tagSet := make(map[string]bool)
	for _, m := range matches {
		if len(m) > 1 {
			tagSet[strings.ToUpper(m[1])] = true
		}
	}

	// Keyword inference
	keywords := map[string]string{
		"code":     "CODE",
		"test":     "TEST",
		"review":   "REVIEW",
		"plan":     "PLAN",
		"design":   "ARCH",
		"database": "DB",
		"bug":      "BUG",
		"doc":      "DOCS",
	}

	for kw, tag := range keywords {
		if strings.Contains(combined, kw) {
			tagSet[tag] = true
		}
	}

	tags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		tags = append(tags, tag)
	}
	if len(tags) == 0 {
		tags = append(tags, "GENERAL")
	}
	return tags
}
