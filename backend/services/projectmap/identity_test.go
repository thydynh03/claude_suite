package projectmap

import (
	"path/filepath"
	"testing"

	"claude_suite/backend/database"
)

// The identity the mapper stores under and the identity derived through the
// shared resolver must be the same id — otherwise the map, observations and
// (later) lessons for one folder split across workspace rows. This failed
// live before WorkspaceRemoteIdentity existed: a stray git repo in the user's
// home directory gave the temp fixture a remote-based identity while the test
// assumed the path-based one.
func TestMapperAndResolverAgreeOnTheWorkspaceID(t *testing.T) {
	db, err := database.OpenAt(filepath.Join(t.TempDir(), "identity.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	wsRepo := database.NewWorkspaceRepository(db)
	graph := database.NewCodeGraphRepository(db)
	m := NewMapper(graph, wsRepo)
	root := writeFixtureWorkspace(t)

	if _, err := m.FullBuild(root); err != nil {
		t.Fatal(err)
	}

	resolved := database.WorkspaceIDFor(root, WorkspaceRemoteIdentity(root))
	rows, err := wsRepo.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("workspaces = %d, want 1", len(rows))
	}
	if rows[0].WorkspaceID != resolved {
		t.Fatalf("mapper stored id %s but the resolver derives %s", rows[0].WorkspaceID, resolved)
	}
	if fps, _ := graph.GetFingerprints(resolved); len(fps) == 0 {
		t.Fatal("fingerprints not visible under the resolved id")
	}
}
