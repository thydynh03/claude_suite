package cli

import (
	"strings"
	"testing"
)

// Both runners must deliver the system prompt through the prompt text — the
// Antigravity CLI has no system flag, and it used to drop `system` entirely,
// so role definitions never reached Gemini agents.
func TestInlineSystemPromptCarriesTheSystemText(t *testing.T) {
	got := inlineSystemPrompt("You are a Senior Backend Engineer.", "Fix the bug.")

	if !strings.Contains(got, "[SYSTEM PROMPT: You are a Senior Backend Engineer.]") {
		t.Errorf("system prompt missing from merged prompt: %q", got)
	}
	if !strings.HasSuffix(got, "Fix the bug.") {
		t.Errorf("user prompt must come last: %q", got)
	}
}

func TestInlineSystemPromptWithoutSystemIsUnchanged(t *testing.T) {
	if got := inlineSystemPrompt("", "Fix the bug."); got != "Fix the bug." {
		t.Errorf("empty system must leave the prompt untouched, got %q", got)
	}
}
