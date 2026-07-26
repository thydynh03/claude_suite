package services

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// gitFixtureRepo initializes a real repo with one committed file. Skips when
// git is unavailable (never the case in CI, which runs AutoSnapshot paths too).
func gitFixtureRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	return root
}

func TestChangedFilesListsModifiedAndUntracked(t *testing.T) {
	g := NewGitService()
	root := gitFixtureRepo(t)

	if got := g.ChangedFiles(root); len(got) != 0 {
		t.Fatalf("clean tree reported changes: %v", got)
	}

	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a // đổi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := g.ChangedFiles(root)
	seen := map[string]bool{}
	for _, f := range got {
		seen[f] = true
	}
	if !seen["a.go"] || !seen["b.go"] {
		t.Fatalf("ChangedFiles = %v, want both the modified and the untracked file", got)
	}
}

func TestChangedFilesOutsideARepoIsEmpty(t *testing.T) {
	g := NewGitService()
	if got := g.ChangedFiles(t.TempDir()); got != nil {
		t.Fatalf("non-repo dir reported %v, want nil", got)
	}
}
