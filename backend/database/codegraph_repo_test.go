package database

import (
	"path/filepath"
	"testing"
	"time"

	"claude_suite/backend/models"
)

func newGraphTestRepo(t *testing.T) *CodeGraphRepository {
	t.Helper()
	db, err := OpenAt(filepath.Join(t.TempDir(), "codegraph_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewCodeGraphRepository(db)
}

func TestUpsertNodesIsIdempotent(t *testing.T) {
	r := newGraphTestRepo(t)
	nodes := []models.CodeNode{
		{NodeID: "file:a.go", Kind: "file", Name: "a.go", FilePath: "a.go", Summary: "v1"},
	}
	if err := r.UpsertNodes("ws1", nodes); err != nil {
		t.Fatal(err)
	}
	nodes[0].Summary = "v2"
	if err := r.UpsertNodes("ws1", nodes); err != nil {
		t.Fatal(err)
	}

	got, err := r.GetNodes("ws1", []string{"file:a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Summary != "v2" {
		t.Fatalf("upsert did not update in place: %+v", got)
	}
}

// Deleting a file must take every edge touching its nodes with it — a
// dangling edge renders as a lie in the context pack.
func TestDeleteFileRemovesDanglingEdges(t *testing.T) {
	r := newGraphTestRepo(t)
	nodes := []models.CodeNode{
		{NodeID: "file:a.go", Kind: "file", Name: "a.go", FilePath: "a.go"},
		{NodeID: "function:a.go:Run", Kind: "function", Name: "Run", FilePath: "a.go"},
		{NodeID: "file:b.go", Kind: "file", Name: "b.go", FilePath: "b.go"},
	}
	edges := []models.CodeEdge{
		{Source: "file:a.go", Target: "function:a.go:Run", Kind: "contains", Weight: 1},
		{Source: "file:b.go", Target: "file:a.go", Kind: "imports", Weight: 0.7},
	}
	if err := r.ReplaceGraph("ws1", nodes, edges); err != nil {
		t.Fatal(err)
	}

	if err := r.DeleteFile("ws1", "a.go"); err != nil {
		t.Fatal(err)
	}

	remainingEdges, farNodes, err := r.Neighbors("ws1", []string{"file:b.go"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(remainingEdges) != 0 || len(farNodes) != 0 {
		t.Fatalf("edges touching the deleted file survived: %+v", remainingEdges)
	}

	left, err := r.GetNodes("ws1", []string{"file:b.go"})
	if err != nil || len(left) != 1 {
		t.Fatalf("unrelated node must survive: %v %+v", err, left)
	}
}

// Ported invariant (Understand-Anything #152): meta must never claim an
// analysis the fingerprint store cannot back.
func TestPutMetaRefusesWithoutFingerprints(t *testing.T) {
	r := newGraphTestRepo(t)
	err := r.PutMeta(&models.ProjectMeta{
		WorkspaceID:   "ws1",
		GitCommitHash: "abc",
		AnalyzedAt:    time.Now(),
		AnalyzedFiles: 42,
	})
	if err == nil {
		t.Fatal("PutMeta accepted 42 analyzed files over an empty fingerprint store")
	}

	if err := r.PutFingerprints("ws1", []models.FileFingerprint{
		{FilePath: "a.go", ContentHash: "h1"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := r.PutMeta(&models.ProjectMeta{
		WorkspaceID: "ws1", GitCommitHash: "abc", AnalyzedAt: time.Now(), AnalyzedFiles: 1,
	}); err != nil {
		t.Fatalf("PutMeta with fingerprints present must succeed: %v", err)
	}

	meta, err := r.GetMeta("ws1")
	if err != nil || meta == nil {
		t.Fatalf("meta round-trip failed: %v", err)
	}
	if meta.GitCommitHash != "abc" {
		t.Fatalf("meta commit = %q, want abc", meta.GitCommitHash)
	}
}
