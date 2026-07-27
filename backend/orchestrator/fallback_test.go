package orchestrator

import (
	"testing"

	"agent_center/backend/models"
)

func TestFallbackHandler_IsQuotaExhausted(t *testing.T) {
	handler := NewFallbackHandler()
	agent := &models.Agent{
		Name:       "Test Agent",
		Provider:   "claude_cli",
		Model:      "claude-opus-4-8",
		TokenLimit: 1000000,
		TokensUsed: 0,
	}

	errQuota := "API Error 429: rate limit exceeded / quota exhausted"
	if !handler.IsQuotaExhausted(agent, errQuota) {
		t.Errorf("expected true for 429 quota error, got false")
	}

	errNormal := "file not found error"
	if handler.IsQuotaExhausted(agent, errNormal) {
		t.Errorf("expected false for standard file error, got true")
	}
}

func TestFallbackHandler_GetFallbackModel(t *testing.T) {
	handler := NewFallbackHandler()

	agentClaude := &models.Agent{Provider: "claude_cli", Model: "claude-opus-4-8"}
	p, m := handler.GetFallbackModel(agentClaude, "")
	if p != "anti_cli" || m != "gemini-3.6-flash-high" {
		t.Errorf("expected anti_cli fallback, got provider=%s model=%s", p, m)
	}

	agentAnti := &models.Agent{Provider: "anti_cli", Model: "gemini-3.6-flash-high"}
	p2, m2 := handler.GetFallbackModel(agentAnti, "")
	if p2 != "claude_cli" || m2 != "claude-sonnet-4-5" {
		t.Errorf("expected claude_cli fallback, got provider=%s model=%s", p2, m2)
	}
}
