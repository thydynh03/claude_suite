package defaults_test

import (
	"os"
	"path/filepath"
	"testing"

	"agent_center/backend/database"
	"agent_center/backend/defaults"
	"agent_center/backend/paths"
)

// What a downloaded build meets on first launch: no data directory, no
// database, no roles, nothing to migrate from.
func TestFreshInstallProducesUsableState(t *testing.T) {
	fresh := filepath.Join(t.TempDir(), "AgentCenter")
	t.Setenv(paths.DataDirEnv, fresh)
	t.Setenv(paths.LegacyDataDirEnv, "")
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	dir := paths.EnsureDataDir()
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("data dir was not created: %v", err)
	}

	// Nothing to adopt must be a quiet no-op, not an error.
	report, err := database.AdoptLegacyDataDir(dir)
	if err != nil {
		t.Fatalf("adoption on a clean machine failed: %v", err)
	}
	if report != nil {
		t.Fatalf("adopted from %q on a machine with no previous install", report.From)
	}

	// SQLite creates the file and migrateSchema creates the tables.
	dbPath := filepath.Join(dir, paths.DatabaseFile)
	db, err := database.OpenAt(dbPath)
	if err != nil {
		t.Fatalf("open fresh database: %v", err)
	}
	defer db.Close()

	for _, table := range []string{"agents", "tasks", "agent_memory"} {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatalf("table %s is not usable on a fresh install: %v", table, err)
		}
		if n != 0 {
			t.Errorf("fresh %s table has %d rows, want 0", table, n)
		}
	}

	// An empty agents table is expected — the orchestrator seeds defaults on the
	// first dispatch. Roles, however, have to be on disk for the role picker.
	written, err := defaults.SeedRoles(filepath.Join(dir, "roles"))
	if err != nil {
		t.Fatalf("seed roles: %v", err)
	}
	if len(written) == 0 {
		t.Fatal("no roles were written on a fresh install")
	}

	entries, err := os.ReadDir(filepath.Join(dir, "roles"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(written) {
		t.Fatalf("roles dir holds %d files, seeded %d", len(entries), len(written))
	}

	// The bundled definitions are the real ones, not one-line placeholders.
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() < 200 {
			t.Errorf("role %s is only %d bytes; the real definition did not ship", entry.Name(), info.Size())
		}
	}
}

// A second launch must not overwrite a role the user edited, and must still
// deliver roles added by a later release.
func TestSeedRolesPreservesUserEdits(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "roles")
	if _, err := defaults.SeedRoles(dir); err != nil {
		t.Fatal(err)
	}

	edited := filepath.Join(dir, "claude.md")
	if err := os.WriteFile(edited, []byte("MY OWN RULES"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Simulate a role that only exists in a newer build.
	if err := os.Remove(filepath.Join(dir, "devops.md")); err != nil {
		t.Fatal(err)
	}

	written, err := defaults.SeedRoles(dir)
	if err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(edited)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "MY OWN RULES" {
		t.Fatal("a user-edited role was overwritten on the next launch")
	}
	if len(written) != 1 || written[0] != "devops.md" {
		t.Fatalf("re-seed wrote %v, want just the missing devops.md", written)
	}
}
