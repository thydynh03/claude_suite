package services

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEnsureWithinWorkspaceAllowsFilesInside(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(inside, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "sub", "deep.txt")
	if err := os.MkdirAll(filepath.Dir(nested), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nested, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{root, inside, nested} {
		if err := EnsureWithinWorkspace(root, path); err != nil {
			t.Errorf("EnsureWithinWorkspace(%q) = %v, want nil", path, err)
		}
	}
}

func TestEnsureWithinWorkspaceRejectsPathOutside(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := EnsureWithinWorkspace(root, outside); err == nil {
		t.Fatal("a file outside the workspace was accepted")
	}
}

// The case filepath.Join cannot catch: the path looks tidy and relative, but the
// link resolves somewhere else entirely.
func TestEnsureWithinWorkspaceRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	secret := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(root, "innocent.txt")
	if err := os.Symlink(secret, link); err != nil {
		// Windows needs Developer Mode or elevation to create symlinks.
		if runtime.GOOS == "windows" {
			t.Skipf("cannot create symlinks on this machine: %v", err)
		}
		t.Fatal(err)
	}

	if err := EnsureWithinWorkspace(root, link); err == nil {
		t.Fatal("a symlink pointing outside the workspace was accepted")
	}
}

// No workspace selected means there is nothing to contain paths to.
func TestEnsureWithinWorkspaceSkipsWhenNoWorkspace(t *testing.T) {
	file := filepath.Join(t.TempDir(), "anywhere.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{"", "   "} {
		if err := EnsureWithinWorkspace(root, file); err != nil {
			t.Errorf("EnsureWithinWorkspace(%q, ...) = %v, want nil", root, err)
		}
	}
}
