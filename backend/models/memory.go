package models

import "time"

// MemoryItem represents a stored conversation memory entry
type MemoryItem struct {
	MemoryID   string    `json:"memory_id"`
	AgentID    string    `json:"agent_id"`
	TaskID     string    `json:"task_id"`
	SessionID  string    `json:"session_id"`
	Role       string    `json:"role"` // "user", "assistant", "system"
	Content    string    `json:"content"`
	Timestamp  time.Time `json:"timestamp"`
	TokensUsed int       `json:"tokens_used"`
}
