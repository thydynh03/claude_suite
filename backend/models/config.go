package models

// WorkspaceConfig manages persisted project workspace history
type WorkspaceConfig struct {
	LastWorkspaceFolder string   `json:"last_workspace_folder"`
	RecentWorkspaces    []string `json:"recent_workspaces"`
}

// AppConfig represents global user settings
type AppConfig struct {
	Theme               string `json:"theme"` // "dark" or "light"
	AutoApprove         bool   `json:"auto_approve"`
	WebhookEnabled      bool   `json:"webhook_enabled"`
	WebhookPort         int    `json:"webhook_port"`
	OrchestratorEnabled bool   `json:"orchestrator_enabled"`
}
