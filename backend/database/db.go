package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	dbInstance *sql.DB
	dbOnce     sync.Once
)

// GetDBPath returns the absolute path to SQLite database file
func GetDBPath() string {
	baseDir := `e:\exe`
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		home, err := os.UserHomeDir()
		if err != nil {
			baseDir = "."
		} else {
			baseDir = filepath.Join(home, ".claude_suite")
		}
	}
	_ = os.MkdirAll(baseDir, 0755)
	return filepath.Join(baseDir, "agent_manager.db")
}

// InitDB initializes SQLite connection with WAL mode and runs migrations
func InitDB() (*sql.DB, error) {
	var initErr error
	dbOnce.Do(func() {
		dbPath := GetDBPath()
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			initErr = fmt.Errorf("failed to open sqlite db: %w", err)
			return
		}

		// Enable WAL mode & foreign keys
		_, _ = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`)

		if err := migrateSchema(db); err != nil {
			initErr = fmt.Errorf("failed to run migrations: %w", err)
			return
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
	`
	_, err := db.Exec(schema)
	return err
}
