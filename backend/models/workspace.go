package models

import "time"

// Workspace is the durable identity of one project folder. The ID follows the
// git remote when there is one, so memory survives re-clones and folder moves;
// without a remote it falls back to the normalized path.
type Workspace struct {
	WorkspaceID string `json:"workspace_id"`
	RootPath    string `json:"root_path"`
	RemoteURL   string `json:"remote_url"`
	// DistilledThrough is the learner's watermark: the highest observation
	// rowid already distilled into lessons. GC only deletes below it.
	DistilledThrough int64     `json:"distilled_through"`
	CreatedAt        time.Time `json:"created_at"`
	LastSeen         time.Time `json:"last_seen"`
}

// Observation is one captured event from an orchestrated run: a tool call, a
// lifecycle transition, or the context pack a task was given.
type Observation struct {
	ObsID       int64     `json:"obs_id"`
	WorkspaceID string    `json:"workspace_id"`
	TaskID      string    `json:"task_id"`
	AgentID     string    `json:"agent_id"`
	Event       string    `json:"event"` // task_start|tool_start|tool_complete|task_complete|task_failed|build_verify_failed|pack_injected
	Tool        string    `json:"tool"`
	Payload     string    `json:"payload"`
	Timestamp   time.Time `json:"ts"`
}
