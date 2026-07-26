package services

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A real repository with a real merge, because the point of the change is that
// the log carries enough structure to draw a graph — and only git can produce
// that structure honestly.
func TestGetLogCarriesParentsAndRefsSoAGraphCanBeDrawn(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	git("init", "-q", "-b", "main")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test")

	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	write("a.txt", "one")
	git("add", ".")
	git("commit", "-q", "-m", "first | with a pipe in the subject")

	git("checkout", "-q", "-b", "feature")
	write("b.txt", "two")
	git("add", ".")
	git("commit", "-q", "-m", "on the branch")

	git("checkout", "-q", "main")
	write("c.txt", "three")
	git("add", ".")
	git("commit", "-q", "-m", "on main")

	git("merge", "-q", "--no-ff", "feature", "-m", "merge feature")

	svc := &GitService{}
	commits, err := svc.GetLog(dir, 20)
	if err != nil {
		t.Fatalf("GetLog: %v", err)
	}
	if len(commits) != 4 {
		t.Fatalf("got %d commits, want 4: %+v", len(commits), commits)
	}

	byMessage := map[string]GitCommitInfo{}
	for _, c := range commits {
		byMessage[c.Message] = c
	}

	merge, ok := byMessage["merge feature"]
	if !ok {
		t.Fatalf("merge commit missing; parsed commits: %+v", commits)
	}
	if len(merge.Parents) != 2 {
		t.Errorf("merge has %d parents, want 2 — a graph cannot be drawn without them", len(merge.Parents))
	}

	// The old format split on "|", so a subject containing one was truncated.
	if _, ok := byMessage["first | with a pipe in the subject"]; !ok {
		t.Errorf("a subject containing a pipe was mangled: %+v", commits)
	}

	root, ok := byMessage["first | with a pipe in the subject"]
	if ok && len(root.Parents) != 0 {
		t.Errorf("root commit has parents %v, want none", root.Parents)
	}

	// --all: the branch head must appear even though it is not checked out.
	var sawFeatureRef bool
	for _, c := range commits {
		for _, ref := range c.Refs {
			if ref == "feature" {
				sawFeatureRef = true
			}
		}
	}
	if !sawFeatureRef {
		t.Errorf("no commit carries the 'feature' ref; refs seen: %+v", commits)
	}
}

func TestGetLogReturnsNothingForAnEmptyPath(t *testing.T) {
	svc := &GitService{}
	got, err := svc.GetLog("", 10)
	if err != nil || len(got) != 0 {
		t.Errorf("GetLog(\"\") = %v, %v; want empty and no error", got, err)
	}
}
