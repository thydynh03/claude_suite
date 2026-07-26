package cli

import "fmt"

// inlineSystemPrompt merges a system prompt into the user prompt for CLIs that
// have no separate system-prompt channel. Both runners route through here: the
// Antigravity runner used to accept `system` and silently drop it, so every
// role definition the orchestrator injected never reached a Gemini agent.
func inlineSystemPrompt(system, prompt string) string {
	if system == "" {
		return prompt
	}
	return fmt.Sprintf("[SYSTEM PROMPT: %s]\n\n%s", system, prompt)
}
