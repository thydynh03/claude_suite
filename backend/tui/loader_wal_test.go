package tui

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// A running app leaves recent writes in the -wal sidecar. The read-only loader
// has to see them: opening with immutable=1 makes SQLite skip the WAL, which
// shows deleted rows as still present and hides everything written since the
// last checkpoint.
func TestOpenReadOnlySeesUncheckpointedWALRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent_manager.db")

	writer, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// Closed via t.Cleanup so a failing assertion still releases the file and
	// TempDir teardown can remove it on Windows.
	t.Cleanup(func() { _ = writer.Close() })
	if _, err := writer.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(`CREATE TABLE tasks (task_id TEXT PRIMARY KEY, title TEXT)`); err != nil {
		t.Fatal(err)
	}
	// One checkpointed row, so the main file is not empty.
	if _, err := writer.Exec(`INSERT INTO tasks VALUES ('old', 'checkpointed')`); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	// These stay in the WAL, exactly like a live session's recent writes.
	for _, id := range []string{"a", "b", "c"} {
		if _, err := writer.Exec(`INSERT INTO tasks VALUES (?, 'in wal')`, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := writer.Exec(`DELETE FROM tasks WHERE task_id = 'old'`); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path + "-wal"); err != nil {
		t.Skipf("no WAL sidecar on this platform: %v", err)
	}

	db, _, err := openReadOnly(path)
	if err != nil {
		t.Fatalf("openReadOnly: %v", err)
	}
	defer db.Close()

	var count int
	if err := db.QueryRow(`SELECT count(*) FROM tasks`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 3 {
		t.Fatalf("read-only view has %d rows, want 3 — the WAL was not read", count)
	}

	var deleted int
	if err := db.QueryRow(`SELECT count(*) FROM tasks WHERE task_id = 'old'`).Scan(&deleted); err != nil {
		t.Fatalf("count deleted: %v", err)
	}
	if deleted != 0 {
		t.Fatal("a row deleted in the WAL is still visible to the read-only view")
	}

}
