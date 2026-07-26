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
		var rawCreated, rawUpdated interface{}
		var rawSessionID, rawLastTask, rawLastError, rawNotes interface{}
		err := rows.Scan(
			&a.AgentID, &a.Name, &a.Role, &a.Provider, &a.Model, &a.System, &a.Icon,
			&rawSessionID, &a.Status, &a.TasksDone, &rawLastTask, &rawLastError, &rawNotes,
			&a.TokensUsed, &a.TokenLimit, &rawCreated, &rawUpdated,
		)
		if err != nil {
			return nil, err
		}
		a.SessionID = parseString(rawSessionID)
		a.LastTask = parseString(rawLastTask)
		a.LastError = parseString(rawLastError)
		a.Notes = parseString(rawNotes)
		a.CreatedAt = parseTimeValue(rawCreated)
		a.UpdatedAt = parseTimeValue(rawUpdated)
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
	var rawCreated, rawUpdated interface{}
	var rawSessionID, rawLastTask, rawLastError, rawNotes interface{}
	err := row.Scan(
		&a.AgentID, &a.Name, &a.Role, &a.Provider, &a.Model, &a.System, &a.Icon,
		&rawSessionID, &a.Status, &a.TasksDone, &rawLastTask, &rawLastError, &rawNotes,
		&a.TokensUsed, &a.TokenLimit, &rawCreated, &rawUpdated,
	)
	if err != nil {
		return nil, err
	}
	a.SessionID = parseString(rawSessionID)
	a.LastTask = parseString(rawLastTask)
	a.LastError = parseString(rawLastError)
	a.Notes = parseString(rawNotes)
	a.CreatedAt = parseTimeValue(rawCreated)
	a.UpdatedAt = parseTimeValue(rawUpdated)
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

// Update writes the agent's PROFILE — the fields a person edits. Live state
// (status, tasks_done, tokens_used, last_task, last_error, session_id) is
// deliberately not here: the caller's copy of those is as old as the page it
// was loaded into, and saving a persona edit while a task ran used to roll
// the counters back to page-load values. Live state goes through the narrow
// updaters below, which is the whole reason they exist.
func (r *AgentRepository) Update(a *models.Agent) error {
	a.UpdatedAt = time.Now()
	query := `UPDATE agents SET name=?, role=?, provider=?, model=?, system=?, icon=?, notes=?, token_limit=?, updated_at=? WHERE agent_id=?`
	_, err := r.db.Exec(query, a.Name, a.Role, a.Provider, a.Model, a.System, a.Icon, a.Notes, a.TokenLimit, a.UpdatedAt, a.AgentID)
	return err
}

// SetStatus writes only the agent's live state.
//
// Update rewrites the whole row from a caller's copy, which is wrong while tasks
// run in parallel: two tasks can share an agent, each goroutine holds its own
// copy, and the last full-row write wins — silently reverting whatever the other
// recorded. These narrow updates touch one concern each, so concurrent tasks no
// longer clobber each other.
func (r *AgentRepository) SetStatus(agentID, status, lastTask, lastError string) error {
	_, err := r.db.Exec(
		`UPDATE agents SET status=?, last_task=?, last_error=?, updated_at=? WHERE agent_id=?`,
		status, lastTask, lastError, time.Now(), agentID,
	)
	return err
}

// CompleteTask marks the agent idle and counts one finished task in a single
// statement, so two tasks finishing together cannot lose a count between them.
func (r *AgentRepository) CompleteTask(agentID string) error {
	_, err := r.db.Exec(
		`UPDATE agents SET status='idle', tasks_done = tasks_done + 1, last_error='', updated_at=? WHERE agent_id=?`,
		time.Now(), agentID,
	)
	return err
}

// SetProviderModel records a provider switch — the smart fallback after a quota
// error — without disturbing the agent's live state. Written through Update, a
// concurrent task's stale copy would put the exhausted provider straight back.
func (r *AgentRepository) SetProviderModel(agentID, provider, model string) error {
	_, err := r.db.Exec(
		`UPDATE agents SET provider=?, model=?, updated_at=? WHERE agent_id=?`,
		provider, model, time.Now(), agentID,
	)
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
		{Name: "Tech Lead & Architect", Role: "Technical Leadership & System Architecture", Model: "claude-opus-4-8", Provider: "claude_cli", Icon: "architecture", System: "You are a Tech Lead & Principal Systems Architect."},
		{Name: "Product Manager (PdM)", Role: "Product Strategy & Feature Definition", Model: "claude-opus-4-8", Provider: "claude_cli", Icon: "adjust", System: "You are a Product Manager defining feature roadmap and product vision."},
		{Name: "Project Manager (PM)", Role: "Agile Execution & Task Orchestration", Model: "claude-sonnet-4-5", Provider: "claude_cli", Icon: "bar_chart", System: "You are a Technical Project Manager tracking project milestones."},
		{Name: "Business Analyst (BA)", Role: "Requirements Specification & Domain Analysis", Model: "claude-sonnet-4-5", Provider: "claude_cli", Icon: "assignment", System: "You are a Senior Business Analyst specifying acceptance criteria."},
		{Name: "Front-end Developer", Role: "UI/UX & Frontend Web Engineering", Model: "claude-sonnet-4-5", Provider: "claude_cli", Icon: "palette", System: "You are a Senior Frontend Developer specializing in Svelte, React, HTML5, CSS3."},
		{Name: "Back-end Developer", Role: "Core Business Logic & API/Database Engineering", Model: "claude-sonnet-4-5", Provider: "claude_cli", Icon: "settings", System: "You are a Senior Backend Engineer specializing in Go, Node.js, and SQL databases."},
		{Name: "DevOps Engineer", Role: "CI/CD, Build Systems & Deployment Automation", Model: "gemini-3.6-flash-high", Provider: "anti_cli", Icon: "rocket_launch", System: "You are a DevOps Specialist managing CI/CD pipelines and deployment scripts."},
		{Name: "QA/QC Specialist", Role: "Quality Control & Automated Testing", Model: "gemini-3.6-flash-high", Provider: "anti_cli", Icon: "science", System: "You are a QA/QC Engineer writing automated test suites and validating functionality."},
		{Name: "Web E2E Tester (Chrome CDP)", Role: "Automated Web Testing & Screenshot Verification", Model: "gemini-3.6-flash-high", Provider: "anti_cli", Icon: "language", System: "You are an Automated Web E2E Testing Agent controlling Chrome CDP to navigate web apps, extract DOM, and verify UI screenshot proofs."},
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
