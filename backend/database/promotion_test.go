package database

import (
	"path/filepath"
	"testing"

	"agent_center/backend/models"
)

// The ported ECC promotion rule: the same trigger active in ≥2 workspaces
// with avg confidence ≥0.8 earns a global copy — once.
func TestPromoteEligiblePromotesOnceAndOnlyTheProven(t *testing.T) {
	db, err := OpenAt(filepath.Join(t.TempDir(), "promotion_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	r := NewLessonRepository(db)

	add := func(ws, trigger string, conf float64) {
		if err := r.Add(&models.MemoryLesson{
			WorkspaceID: ws, Trigger: trigger, Action: "act on " + trigger,
			Confidence: conf, Status: "active", Kind: "mechanical",
		}); err != nil {
			t.Fatal(err)
		}
	}

	add("ws1", "run tests first", 0.85)
	add("ws2", "run tests first", 0.85) // proven in two workspaces
	add("ws1", "only here", 0.9)        // one workspace only
	add("ws1", "weak everywhere", 0.5)
	add("ws2", "weak everywhere", 0.5) // two workspaces, low confidence

	n, err := r.PromoteEligible()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("promoted %d, want exactly 1", n)
	}

	global, err := r.ListByStatus("", "active", 50)
	if err != nil {
		t.Fatal(err)
	}
	globals := 0
	for _, l := range global {
		if l.WorkspaceID == "" {
			globals++
			if l.Trigger != "run tests first" {
				t.Fatalf("wrong lesson promoted: %+v", l)
			}
		}
	}
	if globals != 1 {
		t.Fatalf("global lessons = %d, want 1", globals)
	}

	// Idempotent: running again promotes nothing new.
	n2, err := r.PromoteEligible()
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 0 {
		t.Fatalf("second run promoted %d, want 0", n2)
	}

	// A third workspace now sees the global lesson in its injectable set.
	injectable, err := r.ListActive("ws3", 0.7, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(injectable) != 1 || injectable[0].WorkspaceID != "" {
		t.Fatalf("global lesson must reach other workspaces: %+v", injectable)
	}
}

// The FTS index (when this build has FTS5) must stay in sync through the
// triggers and find nodes by summary words; without FTS the LIKE path serves.
func TestSearchNodesFindsBySummaryWithOrWithoutFTS(t *testing.T) {
	db, err := OpenAt(filepath.Join(t.TempDir(), "fts_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	r := NewCodeGraphRepository(db)

	nodes := []models.CodeNode{
		{NodeID: "file:a.go", Kind: "file", Name: "a.go", FilePath: "a.go", Summary: "orchestrates the dispatcher pool"},
		{NodeID: "file:b.go", Kind: "file", Name: "b.go", FilePath: "b.go", Summary: "renders markdown"},
	}
	if err := r.UpsertNodes("ws1", nodes); err != nil {
		t.Fatal(err)
	}

	got, err := r.SearchNodes("ws1", []string{"dispatcher"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].NodeID != "file:a.go" {
		t.Fatalf("search by summary word failed: %+v", got)
	}

	// Update must be reflected (FTS triggers fire on UPDATE too).
	nodes[0].Summary = "now about scheduling"
	if err := r.UpsertNodes("ws1", nodes[:1]); err != nil {
		t.Fatal(err)
	}
	stale, err := r.SearchNodes("ws1", []string{"dispatcher"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 0 {
		t.Fatalf("stale index: old summary still matches: %+v", stale)
	}
	fresh, err := r.SearchNodes("ws1", []string{"scheduling"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(fresh) != 1 {
		t.Fatalf("new summary not searchable: %+v", fresh)
	}

	// Delete must drop the index rows with the nodes.
	if err := r.DeleteFile("ws1", "a.go"); err != nil {
		t.Fatal(err)
	}
	gone, err := r.SearchNodes("ws1", []string{"scheduling"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(gone) != 0 {
		t.Fatalf("deleted node still searchable: %+v", gone)
	}
}
