package projectmap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent_center/backend/database"
	"agent_center/backend/models"
)

func newIncrementalHarness(t *testing.T) (*Mapper, *database.CodeGraphRepository, string) {
	t.Helper()
	db, err := database.OpenAt(filepath.Join(t.TempDir(), "incremental_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	graph := database.NewCodeGraphRepository(db)
	m := NewMapper(graph, database.NewWorkspaceRepository(db))
	root := writeFixtureWorkspace(t)
	if _, err := m.FullBuild(root); err != nil {
		t.Fatalf("full build: %v", err)
	}
	return m, graph, root
}

func rewrite(t *testing.T, root, rel, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A structural edit (renamed function) must patch the graph before the next
// task dispatches — the map is allowed to lag by at most the current task.
// fixtureWorkspaceID derives the id the same way the mapper does. The fixture
// may sit inside an unrelated ancestor repo (a stray `git init` in the user's
// home swallows the temp dir — observed live), so tests must never assume the
// path-only identity.
func fixtureWorkspaceID(root string) string {
	return database.WorkspaceIDFor(root, WorkspaceRemoteIdentity(root))
}

func TestIncrementalUpdatePatchesAStructuralChange(t *testing.T) {
	m, graph, root := newIncrementalHarness(t)
	ws := fixtureWorkspaceID(root)

	rewrite(t, root, "internal/store/store.go", "package store\n\nfunc OpenDB() {}\n")
	if err := m.IncrementalUpdate(root, []string{"internal/store/store.go"}); err != nil {
		t.Fatalf("incremental update: %v", err)
	}

	if nodes, _ := graph.GetNodes(ws, []string{"function:internal/store/store.go:OpenDB"}); len(nodes) != 1 {
		t.Fatal("renamed function's node was not created")
	}
	if nodes, _ := graph.GetNodes(ws, []string{"function:internal/store/store.go:Open"}); len(nodes) != 0 {
		t.Fatal("stale node for the old function name survived")
	}

	task := &models.Task{Title: "store", Prompt: "OpenDB trong internal/store/store.go"}
	pack := m.RenderPack(root, task, 6000)
	if !strings.Contains(pack, "function:internal/store/store.go:OpenDB") {
		t.Fatalf("pack does not reflect the patched graph:\n%s", pack)
	}
}

// A comment-only edit is COSMETIC: the fingerprint hash moves so the next
// comparison is NONE, but the stored nodes are untouched.
func TestIncrementalUpdateCosmeticOnlyMovesTheHash(t *testing.T) {
	m, graph, root := newIncrementalHarness(t)
	ws := fixtureWorkspaceID(root)

	before, err := graph.GetFingerprints(ws)
	if err != nil {
		t.Fatal(err)
	}

	rewrite(t, root, "internal/store/store.go", "package store\n\n// giải thích thêm\nfunc Open() {}\n")
	if err := m.IncrementalUpdate(root, []string{"internal/store/store.go"}); err != nil {
		t.Fatalf("incremental update: %v", err)
	}

	after, err := graph.GetFingerprints(ws)
	if err != nil {
		t.Fatal(err)
	}
	fp := after["internal/store/store.go"]
	if fp.ContentHash == before["internal/store/store.go"].ContentHash {
		t.Fatal("content hash did not move although the file changed")
	}
	if fp.StructureSig != before["internal/store/store.go"].StructureSig {
		t.Fatal("structure signature moved on a comment-only edit — COSMETIC detection is broken")
	}
	if nodes, _ := graph.GetNodes(ws, []string{"function:internal/store/store.go:Open"}); len(nodes) != 1 {
		t.Fatal("cosmetic edit must leave the function node in place")
	}
}

func TestIncrementalUpdateRemovesDeletedFiles(t *testing.T) {
	m, graph, root := newIncrementalHarness(t)
	ws := fixtureWorkspaceID(root)

	// Sanity: this id must actually hold the built map, or every assertion
	// below passes vacuously against an empty workspace.
	if fps, _ := graph.GetFingerprints(ws); len(fps) == 0 {
		t.Fatal("fixture workspace id does not match the mapper's — assertions would be vacuous")
	}

	if err := os.Remove(filepath.Join(root, "web", "state.js")); err != nil {
		t.Fatal(err)
	}
	if err := m.IncrementalUpdate(root, []string{"web/state.js"}); err != nil {
		t.Fatalf("incremental update: %v", err)
	}

	if nodes, _ := graph.GetNodes(ws, []string{"file:web/state.js"}); len(nodes) != 0 {
		t.Fatal("deleted file's node survived")
	}
	fps, _ := graph.GetFingerprints(ws)
	if _, still := fps["web/state.js"]; still {
		t.Fatal("deleted file's fingerprint survived")
	}
}

// Without a map, the hook must stay silent — the first build is an explicit
// user action, not a per-task surprise.
func TestIncrementalUpdateWithoutAMapIsANoOp(t *testing.T) {
	db, err := database.OpenAt(filepath.Join(t.TempDir(), "noop_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	graph := database.NewCodeGraphRepository(db)
	m := NewMapper(graph, database.NewWorkspaceRepository(db))
	root := writeFixtureWorkspace(t)

	if err := m.IncrementalUpdate(root, []string{"main.go"}); err != nil {
		t.Fatalf("no-map update must not error: %v", err)
	}
	ws := fixtureWorkspaceID(root)
	if nodes, edges, files, _ := countsOf(graph, ws); nodes+edges+files != 0 {
		t.Fatalf("no-map update wrote graph data: %d/%d/%d", nodes, edges, files)
	}
}

func countsOf(graph *database.CodeGraphRepository, ws string) (int, int, int, error) {
	return graph.Counts(ws)
}
