package projectmap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"claude_suite/backend/database"
	"claude_suite/backend/models"
)

func newTestMapper(t *testing.T) *Mapper {
	t.Helper()
	db, err := database.OpenAt(filepath.Join(t.TempDir(), "mapper_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewMapper(database.NewCodeGraphRepository(db), database.NewWorkspaceRepository(db))
}

// writeFixtureWorkspace creates a tiny Go module + Svelte frontend so the
// graph shape (module nodes, contains edges, resolved imports) is assertable.
func writeFixtureWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	write := func(rel, content string) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("go.mod", "module example.com/fixture\n\ngo 1.22\n")
	write("main.go", `package main

import "example.com/fixture/internal/store"

func main() { store.Open() }
`)
	write("internal/store/store.go", `package store

func Open() {}
`)
	write("web/App.svelte", `<script>
import { rows } from './state.js';
function render() { return rows; }
</script>
`)
	write("web/state.js", "export const rows = [];\n")
	return root
}

func TestFullBuildProducesGraphAndResolvedImports(t *testing.T) {
	m := newTestMapper(t)
	root := writeFixtureWorkspace(t)

	report, err := m.FullBuild(root)
	if err != nil {
		t.Fatalf("full build: %v", err)
	}
	if report.Files != 5 {
		t.Fatalf("scanned files = %d, want 5", report.Files)
	}

	// The task-relevant slice must surface the store package for a task that
	// talks about it.
	task := &models.Task{Title: "Sửa store", Prompt: "Refactor internal/store/store.go: hàm Open cần lock"}
	pack := m.RenderPack(root, task, 6000)
	if !strings.Contains(pack, "file:internal/store/store.go") {
		t.Fatalf("pack missing the mentioned file:\n%s", pack)
	}
	if !strings.Contains(pack, "function:internal/store/store.go:Open") &&
		!strings.Contains(pack, "Open") {
		t.Fatalf("pack missing the store's function:\n%s", pack)
	}

	// Go internal import resolves to the module node.
	packMain := m.RenderPack(root, &models.Task{Title: "main.go", Prompt: "sửa main.go"}, 6000)
	if !strings.Contains(packMain, "--[imports]--> module:internal/store") {
		t.Fatalf("go import edge missing from main.go's slice:\n%s", packMain)
	}

	// project-map.md digest is written for the CLI and the Plan Builder.
	data, err := os.ReadFile(filepath.Join(root, ".claude-suite", "project-map.md"))
	if err != nil {
		t.Fatalf("project-map.md not written: %v", err)
	}
	if !strings.Contains(string(data), "internal/store") {
		t.Fatalf("digest missing directories:\n%s", data)
	}

	// Not a git repo: staleness must say unknown, never lie.
	if s := m.Staleness(root); s.Status != "unknown" {
		t.Fatalf("staleness in non-git dir = %q, want unknown", s.Status)
	}

	stats, err := m.Stats(root)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Nodes != report.Nodes || stats.Files != report.Files {
		t.Fatalf("stats disagree with the build report: %+v vs %+v", stats, report)
	}
}

func TestRenderPackWithoutMapIsSilent(t *testing.T) {
	m := newTestMapper(t)
	root := t.TempDir()
	task := &models.Task{Title: "anything", Prompt: "anything at all"}
	if pack := m.RenderPack(root, task, 6000); pack != "" {
		t.Fatalf("no map yet must render nothing, got:\n%s", pack)
	}
}

// Golden test against THIS repository: the map must find the real dispatcher
// function, and a deterministic full build must stay fast (plan accept
// criterion: < 10s on this codebase).
func TestFullBuildOnThisRepositoryFindsTheDispatcher(t *testing.T) {
	if testing.Short() {
		t.Skip("full-repo scan")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Skipf("repo root not found from test dir: %v", err)
	}

	m := newTestMapper(t)
	start := time.Now()
	report, err := m.FullBuild(root)
	if err != nil {
		t.Fatalf("full build on repo: %v", err)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("full build took %v, budget is 10s", d)
	}
	if report.Files < 50 {
		t.Fatalf("scanned only %d files from the real repo — scan is broken", report.Files)
	}

	task := &models.Task{
		Title:  "[CODE] Sửa logic matching trong dispatcher",
		Prompt: "FindMatchingAgent trong dispatcher đang chọn sai agent cho task DATABASE.",
	}
	pack := m.RenderPack(root, task, 6000)
	if !strings.Contains(pack, "function:backend/orchestrator/dispatcher.go:FindMatchingAgent") {
		t.Fatalf("pack for a dispatcher task missing FindMatchingAgent:\n%s", pack)
	}
}
