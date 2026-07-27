package orchestrator

import (
	"strings"

	"agent_center/backend/cli"
	"agent_center/backend/models"
)

type FallbackHandler struct{}

func NewFallbackHandler() *FallbackHandler {
	return &FallbackHandler{}
}

func (f *FallbackHandler) IsQuotaExhausted(agent *models.Agent, errStr string) bool {
	if agent.TokenLimit > 0 && agent.TokensUsed >= agent.TokenLimit {
		return true
	}
	errLower := strings.ToLower(errStr)
	keywords := []string{
		"token limit", "quota", "rate limit", "credit balance",
		"context_length_exceeded", "429", "out of tokens", "exhausted", "quota limit", "daily limit",
	}

	for _, kw := range keywords {
		if strings.Contains(errLower, kw) {
			return true
		}
	}
	return false
}

// GetFallbackModel picks where a quota-exhausted run should go next.
//
// failedAccountID is the pool account the failing run reported using. Passing
// it through means the rotation lands on the exhausted account — if the
// runner's own retry loop already rotated the pool off it, this is a no-op
// instead of a second rotation that punished whichever healthy account the
// pool had just moved to.
func (f *FallbackHandler) GetFallbackModel(agent *models.Agent, failedAccountID string) (string, string) {
	if agent.Provider == "anti_cli" {
		nextKeyName := cli.GlobalAntiPool.RotateNextKey(failedAccountID)
		if nextKeyName != "" {
			// Stay on anti_cli with rotated key
			return "anti_cli", "gemini-3.6-flash-high"
		}
		// Pool exhausted -> fallback to Claude
		return "claude_cli", "claude-sonnet-4-5"
	}
	return "anti_cli", "gemini-3.6-flash-high"
}
