package pipeline

import (
	"strings"
	"testing"
)

// The AI is asked to answer in a ```json fenced block, but models routinely wrap
// it in prose or drop the fence. parseJSONTasks has to survive all of it, and it
// is the only thing standing between a chatty model reply and the Kanban board.
func TestParseJSONTasksExtractsPayloadFromDifferentWrappings(t *testing.T) {
	payload := `{"tasks":[{"title":"Build login form","description":"desc","role_tag":"CODE"}]}`

	cases := []struct {
		name string
		raw  string
	}{
		{"fenced with json hint", "Here you go:\n```json\n" + payload + "\n```\nHope that helps!"},
		{"fenced without hint", "```\n" + payload + "\n```"},
		{"bare json", payload},
		{"prose around bare json", "Sure thing. " + payload + " Let me know."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tasks, err := parseJSONTasks(tc.raw, "build an app")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if len(tasks) != 1 {
				t.Fatalf("got %d tasks, want 1", len(tasks))
			}
			if !strings.Contains(tasks[0].Title, "Build login form") {
				t.Fatalf("title = %q, want it to contain the original title", tasks[0].Title)
			}
		})
	}
}

func TestParseJSONTasksRejectsUnusableInput(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"empty", ""},
		{"whitespace only", "   \n\t "},
		{"no json at all", "I could not decompose that requirement."},
		{"malformed json", "{\"tasks\": [ {\"title\": }]}"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseJSONTasks(tc.raw, "req"); err == nil {
				t.Fatal("expected an error, got nil")
			}
		})
	}
}

// role_tag drives which agent picks the task up, so an absent one must be
// inferred rather than left blank.
func TestParseJSONTasksInfersRoleTagFromText(t *testing.T) {
	cases := []struct {
		name  string
		title string
		want  string
	}{
		{"testing work", "Write unit test for parser", "QA"},
		{"vietnamese testing", "Kiểm thử luồng đăng nhập", "QA"},
		{"architecture work", "Design the module architecture", "ARCH"},
		{"review work", "Security audit of the API", "REVIEW"},
		{"anything else", "Implement the payment handler", "CODE"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := `{"tasks":[{"title":"` + tc.title + `","description":"d"}]}`
			tasks, err := parseJSONTasks(raw, "req")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if want := "[" + tc.want + "]"; !strings.HasPrefix(tasks[0].Title, want) {
				t.Fatalf("title = %q, want prefix %q", tasks[0].Title, want)
			}
		})
	}
}

func TestParseJSONTasksAppliesDefaults(t *testing.T) {
	raw := `{"tasks":[{"title":"Implement handler"}]}`
	tasks, err := parseJSONTasks(raw, "req")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	task := tasks[0]

	if task.Priority != "normal" {
		t.Errorf("priority = %q, want normal", task.Priority)
	}
	if task.Status != "backlog" {
		t.Errorf("status = %q, want backlog", task.Status)
	}
	// An empty description would leave the card blank on the board.
	if task.Description != "Implement handler" {
		t.Errorf("description = %q, want it to fall back to the title", task.Description)
	}
	// The agent prompt template is what tells the sub-agent what to actually do.
	if !strings.Contains(task.Prompt, "[ACTION]") {
		t.Errorf("prompt is missing the [ACTION] template:\n%s", task.Prompt)
	}
	if !strings.Contains(task.Prompt, "claude_cli") {
		t.Errorf("prompt should carry the default provider hint:\n%s", task.Prompt)
	}
}

// A title the model already tagged must not be tagged twice.
func TestParseJSONTasksKeepsExistingTitleTag(t *testing.T) {
	raw := `{"tasks":[{"title":"[QA] Already tagged","role_tag":"QA"}]}`
	tasks, err := parseJSONTasks(raw, "req")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if tasks[0].Title != "[QA] Already tagged" {
		t.Fatalf("title = %q, want it left untouched", tasks[0].Title)
	}
}

// SimpleSplit is the fallback when the model cannot be reached at all; an empty
// plan there would leave the user with nothing on the board.
func TestSimpleSplitAlwaysProducesTasks(t *testing.T) {
	builder := NewPlanBuilder(nil)
	tasks := builder.SimpleSplit("Build a todo app with auth")
	if len(tasks) == 0 {
		t.Fatal("SimpleSplit returned no tasks")
	}
	for i, task := range tasks {
		if strings.TrimSpace(task.Title) == "" {
			t.Errorf("task %d has an empty title", i)
		}
		if task.Status != "backlog" {
			t.Errorf("task %d status = %q, want backlog", i, task.Status)
		}
	}
}
