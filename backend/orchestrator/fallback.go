package orchestrator

import (
	"strings"

	"claude_suite/backend/models"
)

type FallbackHandler struct{}

func NewFallbackHandler() *FallbackHandler {
	return &FallbackHandler{}
}

func (f *FallbackHandler) IsQuotaExhausted(agent *models.Agent, errStr string) bool {
	if agent.TokenRemaining <= 0 {
		return true
	}
	modelLower := strings.ToLower(agent.Model)
	if strings.Contains(modelLower, "thinking") || strings.Contains(modelLower, "gemini") {
		return false // Already using Antigravity
	}

	errLower := strings.ToLower(errStr)
	keywords := []string{
		"token limit", "quota", "rate limit", "credit balance",
		"context_length_exceeded", "429", "out of tokens",
	}

	for _, kw := range keywords {
		if strings.Contains(errLower, kw) {
			return true
		}
	}
	return false
}

func (f *FallbackHandler) GetFallbackModel(agent *models.Agent) (string, string) {
	if agent.Provider == "claude_cli" {
		return "anti_cli", "gemini-3.6-flash-high"
	}
	return "claude_cli", "claude-sonnet-4-5"
}
