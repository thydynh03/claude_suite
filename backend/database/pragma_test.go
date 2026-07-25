package database

import (
	"path/filepath"
	"sync"
	"testing"
)

// A pragma issued with db.Exec lands on one pooled connection. Every connection
// the pool opens afterwards starts with the defaults — busy_timeout back to 0,
// so a blocked writer fails at once rather than waiting. Forcing several
// connections open at the same time is what makes the difference visible.
func TestEveryPooledConnectionGetsTheBusyTimeout(t *testing.T) {
	db, err := OpenAt(filepath.Join(t.TempDir(), "pragma.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	const connections = 8

	// Hold several connections open together so the pool has to create them all.
	var wg sync.WaitGroup
	release := make(chan struct{})
	timeouts := make([]int, connections)

	for i := 0; i < connections; i++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			conn, err := db.Conn(t.Context())
			if err != nil {
				t.Errorf("connection %d: %v", slot, err)
				return
			}
			defer conn.Close()

			var timeout int
			if err := conn.QueryRowContext(t.Context(), "PRAGMA busy_timeout").Scan(&timeout); err != nil {
				t.Errorf("connection %d reading busy_timeout: %v", slot, err)
				return
			}
			timeouts[slot] = timeout
			<-release
		}(i)
	}

	// Give the goroutines a moment to each claim their own connection.
	for i := 0; i < connections; i++ {
		wg.Add(0)
	}
	close(release)
	wg.Wait()

	for slot, timeout := range timeouts {
		if timeout != 5000 {
			t.Errorf("connection %d has busy_timeout=%d, want 5000", slot, timeout)
		}
	}
}

func TestOpenAtEnablesWALAndForeignKeys(t *testing.T) {
	db, err := OpenAt(filepath.Join(t.TempDir(), "pragma.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	var journal string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if journal != "wal" {
		t.Errorf("journal_mode = %q, want wal", journal)
	}

	var foreignKeys int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("read foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", foreignKeys)
	}
}
