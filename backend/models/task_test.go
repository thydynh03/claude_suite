package models

import (
	"reflect"
	"testing"
)

// ExtractTags feeds the orchestrator's agent dispatcher, so a wrong tag sends the
// task to the wrong sub-agent. These cases pin the difference between matching a
// keyword as a word and matching it anywhere in the text.
func TestExtractTagsMatchesWholeWordsOnly(t *testing.T) {
	cases := []struct {
		name  string
		title string
		want  []string
	}{
		{
			// Regression: "latest" contains "test", which dispatched this task to
			// the QA agent instead of a developer.
			name:  "latest does not imply a test task",
			title: "Upgrade to the latest Wails release",
			want:  []string{"GENERAL"},
		},
		{
			name:  "decode does not imply a coding task",
			title: "Decode the webhook payload",
			want:  []string{"GENERAL"},
		},
		{
			name:  "explanation does not imply a planning task",
			title: "Write the explanation section",
			want:  []string{"GENERAL"},
		},
		{
			name:  "debugger does not imply a bug task",
			title: "Attach the debugger",
			want:  []string{"GENERAL"},
		},
		{
			name:  "real keywords still match",
			title: "Write tests for the database layer",
			want:  []string{"DB", "TEST"},
		},
		{
			name:  "inflections still match",
			title: "Finish coding the review screen",
			want:  []string{"CODE", "REVIEW"},
		},
		{
			name:  "documentation still implies docs",
			title: "Update the documentation",
			want:  []string{"DOCS"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			task := &Task{Title: tc.title}
			if got := task.ExtractTags(); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ExtractTags(%q) = %v, want %v", tc.title, got, tc.want)
			}
		})
	}
}

// Bracketed role tags are the primary signal: the plan builder emits titles like
// "[QA] ..." and they must survive verbatim.
func TestExtractTagsReadsBracketedRoles(t *testing.T) {
	task := &Task{
		Title:  "[ARCH] Design the schema",
		Prompt: "[CODE][anti_cli] build it",
	}
	want := []string{"ANTI_CLI", "ARCH", "CODE"}
	if got := task.ExtractTags(); !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractTags() = %v, want %v", got, want)
	}
}

// The dispatcher joins these tags into the string it matches agents against, so
// the same task must always produce the same order.
func TestExtractTagsIsDeterministic(t *testing.T) {
	task := &Task{Title: "Review the database docs and write tests"}
	first := task.ExtractTags()
	for i := 0; i < 20; i++ {
		if got := task.ExtractTags(); !reflect.DeepEqual(got, first) {
			t.Fatalf("run %d returned %v, first run returned %v", i, got, first)
		}
	}
}

func TestExtractTagsFallsBackToGeneral(t *testing.T) {
	task := &Task{Title: "Ship it"}
	if got := task.ExtractTags(); !reflect.DeepEqual(got, []string{"GENERAL"}) {
		t.Errorf("ExtractTags() = %v, want [GENERAL]", got)
	}
}
