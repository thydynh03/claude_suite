package cli

import (
	"sync"
)

// GlobalSessionPool maintains active persistent session IDs across tabs and features
type GlobalSessionPool struct {
	mu       sync.RWMutex
	sessions map[string]string // key: "claude", "anti", "quick_cli", agent_id
}

var globalPool = &GlobalSessionPool{
	sessions: make(map[string]string),
}

// GetGlobalSession retrieves an active persistent session ID by key
func GetGlobalSession(key string) string {
	globalPool.mu.RLock()
	defer globalPool.mu.RUnlock()
	return globalPool.sessions[key]
}

// SetGlobalSession sets or updates a persistent session ID for a key
func SetGlobalSession(key, sessionID string) {
	if key == "" {
		return
	}
	globalPool.mu.Lock()
	defer globalPool.mu.Unlock()
	if sessionID != "" {
		globalPool.sessions[key] = sessionID
	}
}

// ClearGlobalSession clears session ID for a key
func ClearGlobalSession(key string) {
	globalPool.mu.Lock()
	defer globalPool.mu.Unlock()
	delete(globalPool.sessions, key)
}
