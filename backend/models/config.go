package models

// WorkspaceConfig manages persisted project workspace history
type WorkspaceConfig struct {
	LastWorkspaceFolder string   `json:"last_workspace_folder"`
	RecentWorkspaces    []string `json:"recent_workspaces"`
}

// IntegrationsConfig holds outbound integration settings (persisted to disk).
type IntegrationsConfig struct {
	OutboundWebhookURL  string `json:"outbound_webhook_url"` // POSTed on task completion (Slack/Discord/custom)
	MCPConnectionString string `json:"mcp_connection_string"`
}

// UIConfig persists first-run UI state, such as whether the onboarding tour has
// already been shown (so it appears once after the app is downloaded/installed).
type UIConfig struct {
	OnboardingSeen    bool   `json:"onboarding_seen"`
	OnboardingVersion string `json:"onboarding_version"`
}

// MemoryConfig tunes the memory/context-pack subsystem (persisted to disk).
// Zero-value semantics matter here: ContextPackMaxChars 0 is the deliberate
// kill switch, so loading always starts from DefaultMemoryConfig and lets the
// file override — a missing field keeps its default, a present 0 is respected.
type MemoryConfig struct {
	ContextPackMaxChars int  `json:"context_pack_max_chars"` // 0 = tắt tiêm pack hoàn toàn
	AutoSummarize       bool `json:"auto_summarize"`         // drip LLM summaries sau incremental update
	SessionResume       bool `json:"session_resume"`         // ghi SessionID về agent để --resume/--conversation
	LessonPromotion     bool `json:"lesson_promotion"`       // promote lesson xuất hiện ở ≥2 workspace lên global
}

// AppConfig represents global user settings
type AppConfig struct {
	Theme               string `json:"theme"` // "dark" or "light"
	AutoApprove         bool   `json:"auto_approve"`
	WebhookEnabled      bool   `json:"webhook_enabled"`
	WebhookPort         int    `json:"webhook_port"`
	OrchestratorEnabled bool   `json:"orchestrator_enabled"`
}
