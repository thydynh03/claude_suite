package projectmap

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"claude_suite/backend/cli"
	"claude_suite/backend/database"
	"claude_suite/backend/testsupport"
)

func TestRefreshSummariesAppliesValidUpdatesAndNeverWedges(t *testing.T) {
	db, err := database.OpenAt(filepath.Join(t.TempDir(), "summarize_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	graph := database.NewCodeGraphRepository(db)
	m := NewMapper(graph, database.NewWorkspaceRepository(db))
	root := writeFixtureWorkspace(t)
	if _, err := m.FullBuild(root); err != nil {
		t.Fatal(err)
	}
	ws := fixtureWorkspaceID(root)

	before, _ := graph.StaleSummaryCount(ws)
	if before == 0 {
		t.Fatal("full build must leave file summaries stale for enrichment")
	}

	runner := &testsupport.FakeRunner{
		OnceBehaviour: func(prompt, model, system string) *cli.RunResult {
			// Answer for main.go, invent a bogus id, skip the rest.
			return &cli.RunResult{Success: true, Output: fmt.Sprintf(`{
				"summaries": [
					{"id": "file:main.go", "summary": "Entry point: mở store rồi thoát.", "tags": ["entry", "GO "]},
					{"id": "file:does-not-exist.go", "summary": "hallucinated", "tags": []}
				]
			}`)}
		},
	}
	m.SetSummarizer(runner, "claude-haiku-4-5")

	n, err := m.RefreshSummaries(root, 10)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if n != before {
		t.Fatalf("refreshed %d files, want all %d stale ones", n, before)
	}

	after, _ := graph.StaleSummaryCount(ws)
	if after != 0 {
		t.Fatalf("stale count = %d after refresh, want 0 — skipped files must not wedge the loop", after)
	}

	nodes, err := graph.GetNodes(ws, []string{"file:main.go", "file:does-not-exist.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("hallucinated node must not be created: %+v", nodes)
	}
	if !strings.Contains(nodes[0].Summary, "Entry point") {
		t.Fatalf("summary not applied: %q", nodes[0].Summary)
	}
	if len(nodes[0].Tags) == 0 || nodes[0].Tags[1] != "go" {
		t.Fatalf("tags must be trimmed and lowercased: %+v", nodes[0].Tags)
	}
}

func TestRefreshSummariesWithoutARunnerErrsCleanly(t *testing.T) {
	m := newTestMapper(t)
	root := writeFixtureWorkspace(t)
	if _, err := m.FullBuild(root); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RefreshSummaries(root, 1); err == nil {
		t.Fatal("refresh without a configured summarizer must error, not no-op silently")
	}
}
