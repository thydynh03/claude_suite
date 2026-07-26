package database

import (
	"database/sql"

	"claude_suite/backend/models"
)

// ObservationRepository is the raw capture store: one row per event from an
// orchestrated run. Callers scrub and truncate payloads before Add — this
// layer stores what it is given.
type ObservationRepository struct {
	db *sql.DB
}

func NewObservationRepository(db *sql.DB) *ObservationRepository {
	return &ObservationRepository{db: db}
}

func (r *ObservationRepository) Add(o *models.Observation) error {
	_, err := r.db.Exec(
		`INSERT INTO observations (workspace_id, task_id, agent_id, event, tool, payload) VALUES (?, ?, ?, ?, ?, ?)`,
		o.WorkspaceID, o.TaskID, o.AgentID, o.Event, o.Tool, o.Payload,
	)
	return err
}

// AddBatch inserts many observations in one transaction — the async capture
// writer's flush path. Per-row INSERTs on the hot log path would contend with
// the worker pool's task writes (busy_timeout is 5s).
func (r *ObservationRepository) AddBatch(obs []models.Observation) error {
	if len(obs) == 0 {
		return nil
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`INSERT INTO observations (workspace_id, task_id, agent_id, event, tool, payload) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, o := range obs {
		if _, err := stmt.Exec(o.WorkspaceID, o.TaskID, o.AgentID, o.Event, o.Tool, o.Payload); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListByTask returns a task's observations, oldest first.
func (r *ObservationRepository) ListByTask(taskID string, limit int) ([]models.Observation, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.Query(
		`SELECT obs_id, workspace_id, task_id, agent_id, event, tool, payload, ts
		 FROM observations WHERE task_id = ? ORDER BY obs_id ASC LIMIT ?`,
		taskID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanObservations(rows)
}

// TailUndistilled returns up to limit observations above the workspace's
// distilled watermark (oldest first) and the highest rowid in the result, so
// the learner can advance the watermark only after a successful distillation.
func (r *ObservationRepository) TailUndistilled(workspaceID string, limit int) ([]models.Observation, int64, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := r.db.Query(
		`SELECT o.obs_id, o.workspace_id, o.task_id, o.agent_id, o.event, o.tool, o.payload, o.ts
		 FROM observations o
		 JOIN workspaces w ON w.workspace_id = o.workspace_id
		 WHERE o.workspace_id = ? AND o.obs_id > w.distilled_through
		 ORDER BY o.obs_id ASC LIMIT ?`,
		workspaceID, limit,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	obs, err := scanObservations(rows)
	if err != nil {
		return nil, 0, err
	}
	var maxRowid int64
	for _, o := range obs {
		if o.ObsID > maxRowid {
			maxRowid = o.ObsID
		}
	}
	return obs, maxRowid, nil
}

// GC keeps at most keep rows per workspace, deleting only below the distilled
// watermark — an undistilled observation is never lost to the cap.
func (r *ObservationRepository) GC(workspaceID string, keep int) error {
	if keep <= 0 {
		keep = 5000
	}
	_, err := r.db.Exec(
		`DELETE FROM observations WHERE workspace_id = ?1
		   AND obs_id <= (SELECT distilled_through FROM workspaces WHERE workspace_id = ?1)
		   AND obs_id NOT IN (
			 SELECT obs_id FROM observations WHERE workspace_id = ?1 ORDER BY obs_id DESC LIMIT ?2
		   )`,
		workspaceID, keep,
	)
	return err
}

func scanObservations(rows *sql.Rows) ([]models.Observation, error) {
	var out []models.Observation
	for rows.Next() {
		var o models.Observation
		var rawTs interface{}
		if err := rows.Scan(&o.ObsID, &o.WorkspaceID, &o.TaskID, &o.AgentID, &o.Event, &o.Tool, &o.Payload, &rawTs); err != nil {
			return nil, err
		}
		o.Timestamp = parseTimeValue(rawTs)
		out = append(out, o)
	}
	return out, rows.Err()
}
