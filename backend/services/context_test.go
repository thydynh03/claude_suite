package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestContextManager_ScanWorkspace(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files
	_ = os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main"), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "README.md"), []byte("# Test"), 0644)

	mgr := NewContextManager()
	files, err := mgr.ScanWorkspace(tempDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(files) < 2 {
		t.Errorf("expected at least 2 files scanned, got %d", len(files))
	}
}
