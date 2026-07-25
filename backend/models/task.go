package models

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

// Task represents a Kanban task or decomposed plan item
type Task struct {
	TaskID      string    `json:"task_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Prompt      string    `json:"prompt"`
	Priority    string    `json:"priority"`    // "high", "normal", "low"
	Status      string    `json:"status"`      // "backlog", "queued", "running", "done", "failed"
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

	for _, rule := range keywordTags {
		if rule.pattern.MatchString(combined) {
			tagSet[rule.tag] = true
		}
	}

	tags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		tags = append(tags, tag)
	}
	if len(tags) == 0 {
		return []string{"GENERAL"}
	}
	// Sorted so the same task always produces the same tag string; the dispatcher
	// joins these into the text it matches agents against.
	sort.Strings(tags)
	return tags
}

// keywordTags infers a tag when the task text contains a keyword as a whole word.
//
// Matching is deliberately word-boundary rather than substring. With a substring
// match, "Upgrade to the latest release" contained "test" and the task was
// dispatched to the QA agent; "decode" implied CODE and "explanation" implied
// PLAN. Common inflections are still matched, so "coding" and "tests" work.
var keywordTags = []struct {
	pattern *regexp.Regexp
	tag     string
}{
	{regexp.MustCompile(`\bcod(e|es|ed|ing)\b`), "CODE"},
	{regexp.MustCompile(`\btest(s|ed|ing)?\b`), "TEST"},
	{regexp.MustCompile(`\breview(s|ed|ing)?\b`), "REVIEW"},
	{regexp.MustCompile(`\bplan(s|ned|ning)?\b`), "PLAN"},
	{regexp.MustCompile(`\bdesign(s|ed|ing)?\b`), "ARCH"},
	{regexp.MustCompile(`\bdatabases?\b`), "DB"},
	{regexp.MustCompile(`\bbugs?\b`), "BUG"},
	{regexp.MustCompile(`\bdocs?\b|\bdocumentation\b`), "DOCS"},
}
