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

// AppConfig represents global user settings
type AppConfig struct {
	Theme               string `json:"theme"` // "dark" or "light"
	AutoApprove         bool   `json:"auto_approve"`
	WebhookEnabled      bool   `json:"webhook_enabled"`
	WebhookPort         int    `json:"webhook_port"`
	OrchestratorEnabled bool   `json:"orchestrator_enabled"`
}
