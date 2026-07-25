package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerify_NoProjectFiles(t *testing.T) {
	dir := t.TempDir()
	v := NewVerifyService()
	res := v.Verify(dir)
	if res.Ran {
		t.Errorf("expected Ran=false for an empty dir, got true")
	}
	if !res.Passed {
		t.Errorf("expected Passed=true when nothing ran")
	}
}

func TestVerify_EmptyCwd(t *testing.T) {
	v := NewVerifyService()
	res := v.Verify("")
	if res.Ran || !res.Passed {
		t.Errorf("empty cwd should be a no-op pass, got %+v", res)
	}
}

func TestVerify_GoBuildFailure(t *testing.T) {
	dir := t.TempDir()
	// A go.mod with a syntactically broken .go file must fail the build.
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module brokenmod\n\ngo 1.22\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main( {\n"), 0644); err != nil {
		t.Fatal(err)
	}
	v := NewVerifyService()
	res := v.Verify(dir)
	if !res.Ran {
		t.Fatal("expected the Go check to run")
	}
	if res.Passed {
		t.Error("expected Passed=false for a broken Go file")
	}
	if res.Report == "" {
		t.Error("expected a non-empty report with the build error")
	}
}

func TestHasNpmBuildScript(t *testing.T) {
	dir := t.TempDir()
	pkg := filepath.Join(dir, "package.json")
	os.WriteFile(pkg, []byte(`{"scripts":{"build":"vite build"}}`), 0644)
	if !hasNpmBuildScript(pkg) {
		t.Error("expected true when a build script is present")
	}
	os.WriteFile(pkg, []byte(`{"scripts":{"test":"vitest"}}`), 0644)
	if hasNpmBuildScript(pkg) {
		t.Error("expected false when no build script is present")
	}
	if hasNpmBuildScript(filepath.Join(dir, "missing.json")) {
		t.Error("expected false for a missing file")
	}
}
