package database

import (
	"database/sql"
	"fmt"
	"time"

	"claude_suite/backend/models"

	"github.com/google/uuid"
)

type AgentRepository struct {
	db *sql.DB
}

func NewAgentRepository(db *sql.DB) *AgentRepository {
	return &AgentRepository{db: db}
}

func (r *AgentRepository) GetAll() ([]models.Agent, error) {
	query := `SELECT agent_id, name, role, provider, model, system, icon, session_id, status, tasks_done, last_task, last_error, notes, tokens_used, token_limit, created_at, updated_at FROM agents ORDER BY created_at ASC`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []models.Agent
	for rows.Next() {
		var a models.Agent
		err := rows.Scan(
			&a.AgentID, &a.Name, &a.Role, &a.Provider, &a.Model, &a.System, &a.Icon,
			&a.SessionID, &a.Status, &a.TasksDone, &a.LastTask, &a.LastError, &a.Notes,
			&a.TokensUsed, &a.TokenLimit, &a.CreatedAt, &a.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		a.TokenRemaining = a.TokenLimit - a.TokensUsed
		if a.TokenRemaining < 0 {
			a.TokenRemaining = 0
		}
		agents = append(agents, a)
	}

	if len(agents) == 0 {
		// Auto-initialize defaults if empty
		_ = r.ResetToDefaults()
		return r.GetAll()
	}

	return agents, nil
}

func (r *AgentRepository) GetByID(agentID string) (*models.Agent, error) {
	query := `SELECT agent_id, name, role, provider, model, system, icon, session_id, status, tasks_done, last_task, last_error, notes, tokens_used, token_limit, created_at, updated_at FROM agents WHERE agent_id = ?`
	row := r.db.QueryRow(query, agentID)

	var a models.Agent
	err := row.Scan(
		&a.AgentID, &a.Name, &a.Role, &a.Provider, &a.Model, &a.System, &a.Icon,
		&a.SessionID, &a.Status, &a.TasksDone, &a.LastTask, &a.LastError, &a.Notes,
		&a.TokensUsed, &a.TokenLimit, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	a.TokenRemaining = a.TokenLimit - a.TokensUsed
	return &a, nil
}

func (r *AgentRepository) Create(a *models.Agent) error {
	if a.AgentID == "" {
		a.AgentID = uuid.New().String()
	}
	if a.TokenLimit == 0 {
		if limit, ok := models.TokenLimits[a.Model]; ok {
			a.TokenLimit = limit
		} else {
			a.TokenLimit = 1000000
		}
	}
	now := time.Now()
	a.CreatedAt = now
	a.UpdatedAt = now

	query := `INSERT INTO agents (agent_id, name, role, provider, model, system, icon, session_id, status, tasks_done, last_task, last_error, notes, tokens_used, token_limit, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := r.db.Exec(query, a.AgentID, a.Name, a.Role, a.Provider, a.Model, a.System, a.Icon, a.SessionID, a.Status, a.TasksDone, a.LastTask, a.LastError, a.Notes, a.TokensUsed, a.TokenLimit, a.CreatedAt, a.UpdatedAt)
	return err
}

func (r *AgentRepository) Update(a *models.Agent) error {
	a.UpdatedAt = time.Now()
	query := `UPDATE agents SET name=?, role=?, provider=?, model=?, system=?, icon=?, session_id=?, status=?, tasks_done=?, last_task=?, last_error=?, notes=?, tokens_used=?, token_limit=?, updated_at=? WHERE agent_id=?`
	_, err := r.db.Exec(query, a.Name, a.Role, a.Provider, a.Model, a.System, a.Icon, a.SessionID, a.Status, a.TasksDone, a.LastTask, a.LastError, a.Notes, a.TokensUsed, a.TokenLimit, a.UpdatedAt, a.AgentID)
	return err
}

func (r *AgentRepository) Delete(agentID string) error {
	_, err := r.db.Exec(`DELETE FROM agents WHERE agent_id = ?`, agentID)
	return err
}

func (r *AgentRepository) DeleteAll() error {
	_, err := r.db.Exec(`DELETE FROM agents`)
	return err
}

func (r *AgentRepository) AddTokens(agentID string, count int64) error {
	_, err := r.db.Exec(`UPDATE agents SET tokens_used = tokens_used + ?, updated_at = ? WHERE agent_id = ?`, count, time.Now(), agentID)
	return err
}

func (r *AgentRepository) ResetToDefaults() error {
	_ = r.DeleteAll()

	presets := []models.Agent{
		{Name: "CEO / Director of Engineering", Role: "Executive Leader & Strategic Orchestrator", Model: "claude-opus-4-8", Provider: "claude_cli", Icon: "👔", System: "You are the Executive Technical Leader."},
		{Name: "Business Analyst (BA)", Role: "Requirements Engineering & Business Domain Lead", Model: "claude-opus-4-8", Provider: "claude_cli", Icon: "📋", System: "You are a Senior Business Analyst."},
		{Name: "Technical Project Manager (PM)", Role: "Agile Project Manager & Scrum Master", Model: "claude-sonnet-4-5", Provider: "claude_cli", Icon: "📊", System: "You are a Technical Project Manager."},
		{Name: "Chief Architect & Tech Lead", Role: "Principal System Architect & Tech Lead", Model: "claude-opus-4-8", Provider: "claude_cli", Icon: "🏗️", System: "You are a Principal Software Architect."},
		{Name: "Senior Fullstack Developer", Role: "Staff Software Engineer & Fullstack Coder", Model: "claude-sonnet-4-5", Provider: "claude_cli", Icon: "💻", System: "You are a Senior Fullstack Engineer."},
		{Name: "Senior Code Reviewer & Auditor", Role: "Code Reviewer & Security Specialist", Model: "claude-opus-4-8", Provider: "claude_cli", Icon: "🔍", System: "You are a Code Reviewer & Security Auditor."},
		{Name: "Lead QA & Test Specialist", Role: "Quality Assurance & Test Automation Specialist", Model: "claude-sonnet-4-5", Provider: "claude_cli", Icon: "🧪", System: "You are a Quality Assurance Specialist."},
	}

	for _, p := range presets {
		p.AgentID = uuid.New().String()
		p.Status = "idle"
		if err := r.Create(&p); err != nil {
			return fmt.Errorf("failed to insert default agent %s: %w", p.Name, err)
		}
	}

	return nil
}
