package tui

import (
	"os"
	"path/filepath"
	"testing"
)

// Opening a write-mode TUI session must seed the role definitions beside the
// database. The orchestrator reads <dataDir>/roles on every task run and
// silently skips missing files, so before this seeding existed a TUI-only
// install ran every agent without any role context.
func TestOpenSeedsRoleDefinitionsBesideTheDatabase(t *testing.T) {
	path := createLegacyDatabase(t)

	actions, err := OpenRepositoryTaskActions(path, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = actions.Close() }()

	rolesDir := filepath.Join(filepath.Dir(path), "roles")
	data, err := os.ReadFile(filepath.Join(rolesDir, "agents.md"))
	if err != nil {
		t.Fatalf("agents.md was not seeded: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("seeded agents.md is empty")
	}
}
