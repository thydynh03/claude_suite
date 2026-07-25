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

// Writing has to work for a file that does not exist yet, which is every new
// file. EvalSymlinks fails outright on a missing path, so the guard resolves the
// nearest existing ancestor instead.
func TestEnsureWithinWorkspaceAllowsNewFiles(t *testing.T) {
	root := t.TempDir()

	newFile := filepath.Join(root, "does-not-exist-yet.txt")
	if err := EnsureWithinWorkspace(root, newFile); err != nil {
		t.Errorf("a new file directly in the workspace was rejected: %v", err)
	}

	nested := filepath.Join(root, "sub", "deeper", "new.txt")
	if err := EnsureWithinWorkspace(root, nested); err != nil {
		t.Errorf("a new file in a new subdirectory was rejected: %v", err)
	}
}

func TestEnsureWithinWorkspaceRejectsNewFileOutside(t *testing.T) {
	root := t.TempDir()
	elsewhere := filepath.Join(t.TempDir(), "new.txt")

	if err := EnsureWithinWorkspace(root, elsewhere); err == nil {
		t.Fatal("a new file outside the workspace was accepted")
	}
}

// The dangerous case for writing: the file does not exist, but the directory it
// would land in is a symlink pointing somewhere else entirely.
func TestEnsureWithinWorkspaceRejectsNewFileUnderSymlinkedDir(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	link := filepath.Join(root, "innocent-dir")
	if err := os.Symlink(outside, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("cannot create symlinks on this machine: %v", err)
		}
		t.Fatal(err)
	}

	target := filepath.Join(link, "payload.txt")
	if err := EnsureWithinWorkspace(root, target); err == nil {
		t.Fatal("a new file under a symlinked directory escaped the workspace")
	}
}
