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

	CREATE TABLE IF NOT EXISTS task_attempts (
		attempt_id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		attempt_no INTEGER NOT NULL,
		error TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS workspaces (
		workspace_id TEXT PRIMARY KEY,
		root_path TEXT NOT NULL,
		remote_url TEXT NOT NULL DEFAULT '',
		distilled_through INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		last_seen TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS observations (
		obs_id INTEGER PRIMARY KEY AUTOINCREMENT,
		workspace_id TEXT NOT NULL,
		task_id TEXT NOT NULL DEFAULT '',
		agent_id TEXT NOT NULL DEFAULT '',
		event TEXT NOT NULL,
		tool TEXT NOT NULL DEFAULT '',
		payload TEXT NOT NULL DEFAULT '',
		ts TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS code_nodes (
		node_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		kind TEXT NOT NULL,
		name TEXT NOT NULL,
		file_path TEXT NOT NULL DEFAULT '',
		line_start INTEGER NOT NULL DEFAULT 0,
		line_end INTEGER NOT NULL DEFAULT 0,
		summary TEXT NOT NULL DEFAULT '',
		tags TEXT NOT NULL DEFAULT '[]',
		summary_stale INTEGER NOT NULL DEFAULT 0,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (workspace_id, node_id)
	);

	CREATE TABLE IF NOT EXISTS code_edges (
		workspace_id TEXT NOT NULL,
		source TEXT NOT NULL,
		target TEXT NOT NULL,
		kind TEXT NOT NULL,
		weight REAL NOT NULL DEFAULT 0.5,
		PRIMARY KEY (workspace_id, source, target, kind)
	);

	CREATE TABLE IF NOT EXISTS file_fingerprints (
		workspace_id TEXT NOT NULL,
		file_path TEXT NOT NULL,
		content_hash TEXT NOT NULL,
		structure_sig TEXT NOT NULL DEFAULT '',
		total_lines INTEGER NOT NULL DEFAULT 0,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (workspace_id, file_path)
	);

	CREATE TABLE IF NOT EXISTS project_meta (
		workspace_id TEXT PRIMARY KEY,
		git_commit_hash TEXT NOT NULL DEFAULT '',
		analyzed_at TIMESTAMP,
		analyzed_files INTEGER NOT NULL DEFAULT 0,
		schema_version TEXT NOT NULL DEFAULT '1.0.0'
	);

	-- trigger_text, not "trigger": TRIGGER is an SQLite keyword.
	CREATE TABLE IF NOT EXISTS memory_lessons (
		lesson_id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT '',
		kind TEXT NOT NULL DEFAULT 'distilled',
		trigger_text TEXT NOT NULL DEFAULT '',
		action TEXT NOT NULL,
		evidence TEXT NOT NULL DEFAULT '',
		confidence REAL NOT NULL DEFAULT 0.5,
		status TEXT NOT NULL DEFAULT 'pending',
		source_task_id TEXT NOT NULL DEFAULT '',
		use_count INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		last_used_at TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS regressions (
		regression_id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		title TEXT NOT NULL,
		symptom TEXT NOT NULL,
		root_cause TEXT NOT NULL DEFAULT '',
		fix_summary TEXT NOT NULL,
		files TEXT NOT NULL DEFAULT '[]',
		guard_check_id TEXT NOT NULL DEFAULT '',
		guard_status TEXT NOT NULL DEFAULT 'none',
		failed_task_id TEXT NOT NULL DEFAULT '',
		fixed_task_id TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'active',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
	CREATE INDEX IF NOT EXISTS idx_tasks_priority ON tasks(priority);
	CREATE INDEX IF NOT EXISTS idx_agents_name ON agents(name);
	CREATE INDEX IF NOT EXISTS idx_agent_memory_agent_id ON agent_memory(agent_id);
	CREATE INDEX IF NOT EXISTS idx_attempts_task ON task_attempts(task_id);
	CREATE INDEX IF NOT EXISTS idx_obs_ws ON observations(workspace_id, obs_id);
	CREATE INDEX IF NOT EXISTS idx_code_nodes_file ON code_nodes(workspace_id, file_path);
	CREATE INDEX IF NOT EXISTS idx_code_edges_source ON code_edges(workspace_id, source);
	CREATE INDEX IF NOT EXISTS idx_code_edges_target ON code_edges(workspace_id, target);
	CREATE INDEX IF NOT EXISTS idx_lessons_ws_status ON memory_lessons(workspace_id, status);
	CREATE INDEX IF NOT EXISTS idx_regressions_ws ON regressions(workspace_id, status);
	`
	_, err := db.Exec(schema)
	if err != nil {
		return err
	}

	// Auto-migrate schema columns for legacy SQLite database files
	_, _ = db.Exec(`ALTER TABLE agents ADD COLUMN role TEXT NOT NULL DEFAULT '';`)

	setupCodeNodeFTS(db)
	return nil
}

// setupCodeNodeFTS creates the optional full-text index over code_nodes.
// Best-effort on purpose: FTS5 ships in modernc.org/sqlite today, but if a
// build ever lacks it, the map must keep working — SearchNodes falls back to
// LIKE when the virtual table is absent. Triggers keep the index in sync so
// no repository code has to know it exists.
func setupCodeNodeFTS(db *sql.DB) {
	if _, err := db.Exec(
		`CREATE VIRTUAL TABLE IF NOT EXISTS code_nodes_fts USING fts5(workspace_id UNINDEXED, node_id UNINDEXED, content)`,
	); err != nil {
		return
	}

	_, _ = db.Exec(`
	CREATE TRIGGER IF NOT EXISTS code_nodes_fts_ai AFTER INSERT ON code_nodes BEGIN
		INSERT INTO code_nodes_fts(workspace_id, node_id, content)
		VALUES (new.workspace_id, new.node_id, new.name || ' ' || new.file_path || ' ' || new.summary || ' ' || new.tags);
	END;
	CREATE TRIGGER IF NOT EXISTS code_nodes_fts_ad AFTER DELETE ON code_nodes BEGIN
		DELETE FROM code_nodes_fts WHERE workspace_id = old.workspace_id AND node_id = old.node_id;
	END;
	CREATE TRIGGER IF NOT EXISTS code_nodes_fts_au AFTER UPDATE ON code_nodes BEGIN
		DELETE FROM code_nodes_fts WHERE workspace_id = old.workspace_id AND node_id = old.node_id;
		INSERT INTO code_nodes_fts(workspace_id, node_id, content)
		VALUES (new.workspace_id, new.node_id, new.name || ' ' || new.file_path || ' ' || new.summary || ' ' || new.tags);
	END;`)

	// Backfill rows created before the index existed (no-op once synced).
	_, _ = db.Exec(`
	INSERT INTO code_nodes_fts(workspace_id, node_id, content)
	SELECT n.workspace_id, n.node_id, n.name || ' ' || n.file_path || ' ' || n.summary || ' ' || n.tags
	FROM code_nodes n
	WHERE NOT EXISTS (
		SELECT 1 FROM code_nodes_fts f WHERE f.workspace_id = n.workspace_id AND f.node_id = n.node_id
	)`)
}
