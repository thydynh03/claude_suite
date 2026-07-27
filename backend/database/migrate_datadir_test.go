package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"agent_center/backend/paths"
)

// seedLegacyDB writes a WAL-mode database with rows still sitting in the -wal
// sidecar — the state a running app leaves behind, and the one a naive file
// copy silently loses.
func seedLegacyDB(t *testing.T, dir string, rows int) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, paths.DatabaseFile))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		t.Fatal(err)
	}
	if err := migrateSchema(db); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < rows; i++ {
		if _, err := db.Exec(
			`INSERT INTO tasks (task_id, title) VALUES (?, ?)`,
			filepath.Base(dir)+"-task-"+itoa(i), "Task "+itoa(i),
		); err != nil {
			t.Fatal(err)
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func countTasks(t *testing.T, dbPath string) int {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM tasks`).Scan(&n); err != nil {
		t.Fatalf("count tasks in %s: %v", dbPath, err)
	}
	return n
}

func TestAdoptLegacyDataDirCarriesWALContents(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "legacy")
	target := filepath.Join(root, "target")

	seedLegacyDB(t, legacy, 250)
	if err := os.WriteFile(filepath.Join(legacy, "ui_config.json"), []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(legacy, "roles"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "roles", "pm.md"), []byte("# PM"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv(paths.DataDirEnv, legacy)
	report, err := adoptFrom(legacy, target)
	if err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if report == nil {
		t.Fatal("nothing was adopted")
	}

	if got := countTasks(t, filepath.Join(target, paths.DatabaseFile)); got != 250 {
		t.Fatalf("migrated database has %d tasks, want 250 — WAL contents were lost", got)
	}
	if _, err := os.Stat(filepath.Join(target, "ui_config.json")); err != nil {
		t.Errorf("ui_config.json was not carried over: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "roles", "pm.md")); err != nil {
		t.Errorf("roles/ was not carried over: %v", err)
	}

	// The original must survive so a bad migration is recoverable.
	if got := countTasks(t, filepath.Join(legacy, paths.DatabaseFile)); got != 250 {
		t.Fatalf("legacy database now has %d tasks; it must be left untouched", got)
	}
}

// Running twice must not clobber data written after the first adoption.
func TestAdoptLegacyDataDirIsSkippedWhenTargetHasData(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "legacy")
	target := filepath.Join(root, "target")

	seedLegacyDB(t, legacy, 10)
	seedLegacyDB(t, target, 3)

	report, err := adoptFrom(legacy, target)
	if err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if report != nil {
		t.Fatal("adopted into a directory that already had a database")
	}
	if got := countTasks(t, filepath.Join(target, paths.DatabaseFile)); got != 3 {
		t.Fatalf("target has %d tasks, want its own 3 left alone", got)
	}
}

func TestAdoptLegacyDataDirNoopWithoutLegacy(t *testing.T) {
	root := t.TempDir()
	report, err := adoptFrom(filepath.Join(root, "does-not-exist"), filepath.Join(root, "target"))
	if err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if report != nil {
		t.Fatal("reported a migration when there was no legacy directory")
	}
}
