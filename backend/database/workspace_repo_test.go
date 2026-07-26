package database

import (
	"path/filepath"
	"testing"

	"claude_suite/backend/models"
)

func newWorkspaceTestRepo(t *testing.T) (*WorkspaceRepository, *ObservationRepository) {
	t.Helper()
	db, err := OpenAt(filepath.Join(t.TempDir(), "workspace_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewWorkspaceRepository(db), NewObservationRepository(db)
}

// Every spelling of the same repository must produce the same identity, and
// credentials must never survive into it.
func TestNormalizeRemoteURLUnifiesSpellings(t *testing.T) {
	want := "github.com/user/repo"
	cases := []string{
		"git@github.com:User/Repo.git",
		"https://github.com/user/repo",
		"https://github.com/User/Repo.git",
		"ssh://github.com/user/repo",
		"https://user:ghp_secret123@github.com/User/Repo.git",
	}
	for _, in := range cases {
		if got := NormalizeRemoteURL(in); got != want {
			t.Errorf("NormalizeRemoteURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWorkspaceIDFollowsTheRemoteAcrossClones(t *testing.T) {
	a := WorkspaceIDFor(`C:\work\clone-one`, "git@github.com:User/Repo.git")
	b := WorkspaceIDFor(`D:\elsewhere\clone-two`, "https://github.com/user/repo")
	if a != b {
		t.Fatalf("two clones of the same remote got different ids: %q vs %q", a, b)
	}
	if len(a) != 12 {
		t.Fatalf("id length = %d, want 12", len(a))
	}
}

// Windows filesystems are case-insensitive: the same folder typed two ways
// must not fork the project's memory.
func TestWorkspaceIDWithoutRemoteIsCaseInsensitive(t *testing.T) {
	a := WorkspaceIDFor(`E:\Exe`, "")
	b := WorkspaceIDFor(`e:\exe`, "")
	if a != b {
		t.Fatalf("case variants of one path got different ids: %q vs %q", a, b)
	}
	other := WorkspaceIDFor(`E:\other`, "")
	if a == other {
		t.Fatal("different paths must get different ids")
	}
}

// A workspace that gains a remote mid-project would change identity and
// silently fork its memory. EnsureWorkspace must re-key the accumulated rows
// onto the new identity instead.
func TestEnsureWorkspaceRekeysWhenARemoteAppears(t *testing.T) {
	wsRepo, obsRepo := newWorkspaceTestRepo(t)
	root := `C:\work\project`

	before, err := wsRepo.EnsureWorkspace(root, "")
	if err != nil {
		t.Fatalf("ensure without remote: %v", err)
	}
	if err := obsRepo.Add(&models.Observation{
		WorkspaceID: before.WorkspaceID, TaskID: "t1", Event: "pack_injected", Payload: "old memory",
	}); err != nil {
		t.Fatalf("add observation: %v", err)
	}

	after, err := wsRepo.EnsureWorkspace(root, "git@github.com:user/project.git")
	if err != nil {
		t.Fatalf("ensure with remote: %v", err)
	}
	if after.WorkspaceID == before.WorkspaceID {
		t.Fatal("identity did not change although a remote appeared")
	}

	moved, err := obsRepo.ListByTask("t1", 10)
	if err != nil {
		t.Fatalf("list observations: %v", err)
	}
	if len(moved) != 1 || moved[0].WorkspaceID != after.WorkspaceID {
		t.Fatalf("observation was not re-keyed: %+v", moved)
	}

	if _, err := wsRepo.Get(before.WorkspaceID); err == nil {
		t.Fatal("old identity row must be gone after re-keying")
	}
	list, err := wsRepo.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("workspaces = %d, want exactly 1 after re-key", len(list))
	}
}

func TestEnsureWorkspaceIsIdempotent(t *testing.T) {
	wsRepo, _ := newWorkspaceTestRepo(t)
	a, err := wsRepo.EnsureWorkspace(`C:\p`, "git@github.com:u/p.git")
	if err != nil {
		t.Fatal(err)
	}
	b, err := wsRepo.EnsureWorkspace(`C:\p`, "git@github.com:u/p.git")
	if err != nil {
		t.Fatal(err)
	}
	if a.WorkspaceID != b.WorkspaceID {
		t.Fatalf("id changed between calls: %q vs %q", a.WorkspaceID, b.WorkspaceID)
	}
	list, _ := wsRepo.List()
	if len(list) != 1 {
		t.Fatalf("workspaces = %d, want 1", len(list))
	}
}
