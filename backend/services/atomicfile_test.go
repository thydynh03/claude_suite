package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileAtomicCreatesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")

	if err := WriteFileAtomic(path, []byte(`{"a":1}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != `{"a":1}` {
		t.Errorf("contents = %q", data)
	}
}

func TestWriteFileAtomicReplacesExistingContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	if err := os.WriteFile(path, []byte("old and longer than the new one"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := WriteFileAtomic(path, []byte("new"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "new" {
		t.Errorf("contents = %q, want the replacement with no remnant of the old", data)
	}
}

// The point of the exercise: nothing is left in the directory to be mistaken for
// the store, or to accumulate on every save.
func TestWriteFileAtomicLeavesNoTemporaryFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "store.json")

	for i := 0; i < 5; i++ {
		if err := WriteFileAtomic(path, []byte("data"), 0o600); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "store.json" {
			t.Errorf("left behind %q", entry.Name())
		}
	}
}

func TestWriteFileAtomicAppliesPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.json")
	if err := WriteFileAtomic(path, []byte("token"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows reports a coarser mode, so this only asserts what holds everywhere:
	// the owner can read and write it.
	if info.Mode().Perm()&0o600 != 0o600 {
		t.Errorf("mode = %v, want at least owner read/write", info.Mode().Perm())
	}
}

func TestWriteFileAtomicReportsAnUnwritableDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-dir", "store.json")

	err := WriteFileAtomic(path, []byte("data"), 0o600)
	if err == nil {
		t.Fatal("writing into a missing directory reported success")
	}
	if !strings.Contains(err.Error(), "temp file") {
		t.Errorf("error does not say what failed: %v", err)
	}
}

// Windows refuses to replace a file another handle has open, and on this project
// something always has one — the UI polls these stores. Without the retry the
// save was silently dropped: the caller saw no error worth acting on and the
// update simply never landed.
func TestWriteFileAtomicSurvivesAConcurrentReader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	if err := WriteFileAtomic(path, []byte(`{"n":0}`), 0o600); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				_, _ = os.ReadFile(path) // the reader that used to break the save
			}
		}
	}()

	var failures int
	for i := 0; i < 40; i++ {
		if err := WriteFileAtomic(path, []byte(`{"n":1}`), 0o600); err != nil {
			failures++
		}
	}
	close(stop)
	<-done

	if failures > 0 {
		t.Errorf("%d of 40 saves were lost while the file was being read", failures)
	}
	data, _ := os.ReadFile(path)
	if string(data) != `{"n":1}` {
		t.Errorf("final contents = %q, want the last write", data)
	}
}
