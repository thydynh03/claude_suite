package database

import (
	"database/sql"
	"time"

	"claude_suite/backend/models"

	"github.com/google/uuid"
)

type MemoryRepository struct {
	db *sql.DB
}

func NewMemoryRepository(db *sql.DB) *MemoryRepository {
	return &MemoryRepository{db: db}
}

func (r *MemoryRepository) Add(item *models.MemoryItem) error {
	if item.MemoryID == "" {
		item.MemoryID = uuid.New().String()
	}
	if item.Timestamp.IsZero() {
		item.Timestamp = time.Now()
	}

	query := `INSERT INTO agent_memory (memory_id, agent_id, task_id, session_id, role, content, timestamp, tokens_used) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := r.db.Exec(query, item.MemoryID, item.AgentID, item.TaskID, item.SessionID, item.Role, item.Content, item.Timestamp, item.TokensUsed)
	if err != nil {
		return err
	}

	// Auto-GC if records exceed 1000
	go r.gc()

	return nil
}

func (r *MemoryRepository) GetByAgent(agentID string, limit int) ([]models.MemoryItem, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT memory_id, agent_id, task_id, session_id, role, content, timestamp, tokens_used FROM agent_memory WHERE agent_id = ? ORDER BY timestamp DESC LIMIT ?`
	rows, err := r.db.Query(query, agentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.MemoryItem
	for rows.Next() {
		var m models.MemoryItem
		if err := rows.Scan(&m.MemoryID, &m.AgentID, &m.TaskID, &m.SessionID, &m.Role, &m.Content, &m.Timestamp, &m.TokensUsed); err != nil {
			return nil, err
		}
		items = append(items, m)
	}

	return items, nil
}

func (r *MemoryRepository) gc() {
	var count int
	row := r.db.QueryRow(`SELECT COUNT(*) FROM agent_memory`)
	if err := row.Scan(&count); err == nil && count > 1000 {
		_, _ = r.db.Exec(`DELETE FROM agent_memory WHERE memory_id IN (SELECT memory_id FROM agent_memory ORDER BY timestamp ASC LIMIT ?)`, count-1000)
	}
}
