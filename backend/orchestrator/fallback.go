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
	errLower := strings.ToLower(errStr)
	keywords := []string{
		"token limit", "quota", "rate limit", "credit balance",
		"context_length_exceeded", "429", "out of tokens", "exhausted", "limit",
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
