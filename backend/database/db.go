package database

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"

	"claude_suite/backend/paths"

	_ "modernc.org/sqlite"
)

var (
	dbInstance *sql.DB
	dbOnce     sync.Once
)

// GetDBPath returns the absolute path to SQLite database file.
//
// Many callers use filepath.Dir(GetDBPath()) as "the data directory", so this
// deliberately stays a thin wrapper over paths.DataDir rather than growing its
// own idea of where state lives.
func GetDBPath() string {
	return filepath.Join(paths.EnsureDataDir(), paths.DatabaseFile)
}

// dsnWithPragmas builds a modernc.org/sqlite DSN that applies the connection
// settings to every pooled connection:
//
//	journal_mode=WAL   readers do not block the writer
//	busy_timeout=5000  a blocked writer waits instead of failing at once
//	foreign_keys=ON    the schema's references are enforced
func dsnWithPragmas(path string) string {
	return path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
}

// OpenAt opens the database at path, applies the connection pragmas and runs
// migrations. InitDB uses it for the app's shared instance; tests use it for a
// throwaway file under t.TempDir().
//
// Tests want a file rather than ":memory:" because database/sql pools
// connections and each new connection to ":memory:" gets its own empty
// database — which breaks the moment the orchestrator runs tasks in parallel.
func OpenAt(path string) (*sql.DB, error) {
	// The pragmas belong in the DSN, not in a db.Exec afterwards. database/sql
	// keeps a pool, and a pragma runs on whichever single connection served that
	// statement — every connection the pool opens later starts with the defaults,
	// busy_timeout among them. With several agents writing task rows at once,
	// those later connections fail immediately with SQLITE_BUSY instead of
	// waiting, and almost every write here discards its error.
	db, err := sql.Open("sqlite", dsnWithPragmas(path))
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db: %w", err)
	}

	if err := migrateSchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}
	return db, nil
}

// InitDB initializes SQLite connection with WAL mode and runs migrations
func InitDB() (*sql.DB, error) {
	var initErr error
	dbOnce.Do(func() {
		dbPath := GetDBPath()

		// Older builds hardcoded the developer's checkout as the data directory.
		// Adopt that state once, before anything opens the new database.
		if report, err := AdoptLegacyDataDir(filepath.Dir(dbPath)); err != nil {
			fmt.Printf("Data migration warning: %v\n", err)
		} else if report != nil {
			fmt.Printf("Adopted existing Claude Suite data from %s (kept as a backup there)\n", report.From)
		}

		db, err := OpenAt(dbPath)
		if err != nil {
			initErr = err
			return
		}

		// Run SQLite Integrity Check
		var checkRes string
		_ = db.QueryRow(`PRAGMA quick_check;`).Scan(&checkRes)
		if checkRes != "ok" && checkRes != "" {
			fmt.Printf("SQLite Integrity Check Notice: %s\n", checkRes)
		}

		dbInstance = db
	})

	return dbInstance, initErr
}

func migrateSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS agents (
		agent_id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT '',
		provider TEXT NOT NULL DEFAULT 'claude_cli',
		model TEXT NOT NULL,
		system TEXT NOT NULL DEFAULT '',
		icon TEXT NOT NULL DEFAULT '🤖',
		session_id TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'idle',
		tasks_done INTEGER NOT NULL DEFAULT 0,
		last_task TEXT NOT NULL DEFAULT '',
		last_error TEXT NOT NULL DEFAULT '',
		notes TEXT NOT NULL DEFAULT '',
		tokens_used INTEGER NOT NULL DEFAULT 0,
		token_limit INTEGER NOT NULL DEFAULT 1000000,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS tasks (
		task_id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		prompt TEXT NOT NULL DEFAULT '',
		priority TEXT NOT NULL DEFAULT 'normal',
		status TEXT NOT NULL DEFAULT 'backlog',
		assigned_to TEXT NOT NULL DEFAULT '',
		depends_on TEXT NOT NULL DEFAULT '[]',
		retry_count INTEGER NOT NULL DEFAULT 0,
		max_retries INTEGER NOT NULL DEFAULT 3,
		result TEXT NOT NULL DEFAULT '',
		session_id TEXT NOT NULL DEFAULT '',
		parent_id TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		started_at TIMESTAMP NULL,
		finished_at TIMESTAMP NULL
	);

	CREATE TABLE IF NOT EXISTS agent_memory (
		memory_id TEXT PRIMARY KEY,
		agent_id TEXT NOT NULL,
		task_id TEXT NOT NULL DEFAULT '',
		session_id TEXT NOT NULL DEFAULT '',
		role TEXT NOT NULL DEFAULT 'user',
		content TEXT NOT NULL DEFAULT '',
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		tokens_used INTEGER NOT NULL DEFAULT 0,
		FOREIGN KEY (agent_id) REFERENCES agents(agent_id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
	CREATE INDEX IF NOT EXISTS idx_tasks_priority ON tasks(priority);
	CREATE INDEX IF NOT EXISTS idx_agents_name ON agents(name);
	CREATE INDEX IF NOT EXISTS idx_agent_memory_agent_id ON agent_memory(agent_id);
	`
	_, err := db.Exec(schema)
	if err != nil {
		return err
	}

	// Auto-migrate schema columns for legacy SQLite database files
	_, _ = db.Exec(`ALTER TABLE agents ADD COLUMN role TEXT NOT NULL DEFAULT '';`)
	return nil
}
