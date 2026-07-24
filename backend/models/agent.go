package models

import "time"

// Agent represents an AI Agent entity in Claude Suite
type Agent struct {
	AgentID        string    `json:"agent_id"`
	Name           string    `json:"name"`
	Role           string    `json:"role"`
	Provider       string    `json:"provider"` // "claude_cli" or "anti_cli"
	Model          string    `json:"model"`
	System         string    `json:"system"`
	Icon           string    `json:"icon"`
	SessionID      string    `json:"session_id"`
	Status         string    `json:"status"` // "idle", "running", "error"
	TasksDone      int       `json:"tasks_done"`
	LastTask       string    `json:"last_task"`
	LastError      string    `json:"last_error"`
	Notes          string    `json:"notes"`
	TokensUsed     int64     `json:"tokens_used"`
	TokenLimit     int64     `json:"token_limit"`
	TokenRemaining int64     `json:"token_remaining"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// TokenLimits contains default token limits for models
var TokenLimits = map[string]int64{
	"claude-opus-4-8":             1000000,
	"claude-sonnet-4-5":           1000000,
	"claude-haiku-4-5":            1000000,
	"gemini-3.6-flash-high":       2000000,
	"gemini-3.6-flash-medium":     2000000,
	"gemini-3.6-flash-low":        2000000,
	"gemini-3.5-flash-high":       1000000,
	"gemini-3.1-pro-high":         1000000,
	"claude-sonnet-4.6-thinking": 1000000,
	"claude-opus-4.6-thinking":   1000000,
}
