package models

// PipelineStep represents one step in the 5-stage sequential pipeline
type PipelineStep struct {
	StepID      int    `json:"step_id"`
	StageName   string `json:"stage_name"`
	Role        string `json:"role"`
	AgentID     string `json:"agent_id"`
	Prompt      string `json:"prompt"`
	Output      string `json:"output"`
	Status      string `json:"status"` // "pending", "running", "completed", "error"
	DurationSec float64 `json:"duration_sec"`
}
