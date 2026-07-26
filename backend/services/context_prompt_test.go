package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A header-only context block framed the whole message as context: the model
// answered "I don't see a question or task in your message" to a clear
// question (observed live from the Cockpit). No readable files → no block.
func TestBuildContextPromptWithoutReadableFilesIsEmpty(t *testing.T) {
	cm := NewContextManager()

	if got := cm.BuildContextPrompt(t.TempDir(), nil); got != "" {
		t.Fatalf("no files: prompt = %q, want empty", got)
	}
	if got := cm.BuildContextPrompt(t.TempDir(), []string{"does-not-exist.go"}); got != "" {
		t.Fatalf("only unreadable files: prompt = %q, want empty", got)
	}
}

func TestBuildContextPromptWithFilesCarriesTheirContent(t *testing.T) {
	cm := NewContextManager()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := cm.BuildContextPrompt(root, []string{"main.go"})
	if !strings.Contains(got, "WORKSPACE CONTEXT") || !strings.Contains(got, "package main") {
		t.Fatalf("context block missing content:\n%s", got)
	}
}
