package tui

import (
	"crypto/sha256"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

const legacySchema = `
CREATE TABLE agents (
	agent_id TEXT PRIMARY KEY, name TEXT NOT NULL,
	model TEXT NOT NULL DEFAULT 'claude-opus-4-5', system TEXT DEFAULT '',
	icon TEXT DEFAULT '🤖', session_id TEXT, status TEXT DEFAULT 'idle',
	tasks_done INTEGER DEFAULT 0, last_task TEXT DEFAULT '',
	last_error TEXT DEFAULT '', notes TEXT DEFAULT '',
	tokens_used INTEGER DEFAULT 0, token_limit INTEGER DEFAULT 200000
);
CREATE TABLE tasks (
	task_id TEXT PRIMARY KEY, title TEXT NOT NULL, description TEXT DEFAULT '',
	prompt TEXT DEFAULT '', priority TEXT DEFAULT 'normal',
	status TEXT DEFAULT 'backlog', assigned_to TEXT, depends_on TEXT DEFAULT '[]',
	retry_count INTEGER DEFAULT 0, max_retries INTEGER DEFAULT 3,
	result TEXT DEFAULT '', session_id TEXT, parent_id TEXT,
	created_at TEXT, started_at TEXT, finished_at TEXT
);
CREATE TABLE agent_memory (
	memory_id TEXT PRIMARY KEY, agent_id TEXT NOT NULL, task_id TEXT,
	session_id TEXT, role TEXT DEFAULT 'user', content TEXT NOT NULL,
	timestamp TEXT, tokens_used INTEGER DEFAULT 0
);`

func TestLoadReadOnlyReadsLegacyDatabaseWithoutMutation(t *testing.T) {
	path := createLegacyDatabase(t)
	beforeInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeContents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := LoadReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}

	afterInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	afterContents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(beforeContents) != sha256.Sum256(afterContents) {
		t.Fatal("read-only load changed database contents")
	}
	if beforeInfo.Size() != afterInfo.Size() || !beforeInfo.ModTime().Equal(afterInfo.ModTime()) {
		t.Fatal("read-only load changed database size or modification time")
	}
	if len(snapshot.Agents) != 1 || snapshot.Agents[0].Name != "Planner" {
		t.Fatalf("agents = %#v", snapshot.Agents)
	}
	if snapshot.Agents[0].Provider != "claude_cli" || snapshot.Agents[0].TokenRemaining != 198500 {
		t.Fatalf("mapped agent = %#v", snapshot.Agents[0])
	}
	if len(snapshot.Tasks) != 1 || snapshot.TaskCounts["backlog"] != 1 {
		t.Fatalf("tasks = %#v, counts = %#v", snapshot.Tasks, snapshot.TaskCounts)
	}
	if snapshot.Tasks[0].CreatedAt.IsZero() {
		t.Fatal("task timestamp was not parsed")
	}
	if len(snapshot.RecentMemory) != 1 || snapshot.RecentMemory[0].Content != "schema is compatible" {
		t.Fatalf("memory = %#v", snapshot.RecentMemory)
	}
	if snapshot.RecentMemory[0].Timestamp.IsZero() {
		t.Fatal("memory timestamp was not parsed")
	}
}

func TestLoadReadOnlyRejectsMissingDatabase(t *testing.T) {
	_, err := LoadReadOnly(filepath.Join(t.TempDir(), "missing.db"))
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadReadOnlyRejectsMalformedDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "malformed.db")
	if err := os.WriteFile(path, []byte("not sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadReadOnly(path); err == nil {
		t.Fatal("expected malformed database error")
	}
}

func TestLoadReadOnlyRejectsIncompatibleSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "incompatible.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE agents (agent_id TEXT PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = LoadReadOnly(path)
	if err == nil || !strings.Contains(err.Error(), "incompatible Claude Suite schema") {
		t.Fatalf("error = %v", err)
	}
}

func createLegacyDatabase(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent_manager.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(legacySchema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO agents VALUES
		('a1','Planner','claude-sonnet-4-5','system','icon',NULL,'idle',2,'plan task','','notes',1500,200000)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO tasks VALUES
		('t1','Read schema','inspect db','prompt','high','backlog','a1','[]',0,3,'',NULL,NULL,'2026-07-24T00:00:00Z',NULL,NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO agent_memory VALUES
		('m1','a1','t1',NULL,'assistant','schema is compatible','2026-07-24T00:00:01Z',42)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
