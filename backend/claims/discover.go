package claims

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// safeScripts are the package.json scripts worth offering as checks.
//
// Deliberately not "every script". A project's scripts can include deploy,
// publish or release, and a claim naming one of those would have the host run it.
// These five are the ones that answer "is the code correct", which is the only
// question a falsifier asks.
var safeScripts = map[string]string{
	"test":      "Project test suite",
	"lint":      "Linter",
	"check":     "Type check",
	"typecheck": "Type check",
	"build":     "Production build",
}

// Discover builds a catalogue from what the workspace already has.
//
// This is what makes .claude-suite/checks.json optional. The safety property is
// unchanged and it is the only one that matters: an agent still names a check
// and never supplies a command. The commands here come from the project's own
// tooling — the same go.mod and package.json anyone who opens this workspace has
// already agreed to run — not from the machine that submitted the claim.
//
// An explicit catalogue is merged on top, so a project with unusual checks can
// still declare them, and can override a discovered entry by reusing its name.
func Discover(workspace string) *Catalogue {
	var found []Check

	if exists(filepath.Join(workspace, "go.mod")) {
		found = append(found,
			Check{
				Name: "go-build", Description: "Go packages compile",
				Command: []string{"go", "build", "./..."}, TimeoutSec: 300,
			},
			Check{
				Name: "go-vet", Description: "go vet",
				Command: []string{"go", "vet", "./..."}, TimeoutSec: 180,
			},
			Check{
				Name: "go-test", Description: "Go test suite",
				Command: []string{"go", "test", "./...", "-count=1"}, TimeoutSec: 600,
			},
		)
	}

	for dir, prefix := range nodeProjects(workspace) {
		for _, script := range scriptsIn(dir) {
			desc, ok := safeScripts[script]
			if !ok {
				continue
			}
			name := "npm-" + script
			command := []string{"npm", "run", script}
			if prefix != "" {
				name = "npm-" + prefix + "-" + script
				command = []string{"npm", "--prefix", prefix, "run", script}
				desc += " (" + prefix + ")"
			}
			found = append(found, Check{
				Name: name, Description: desc, Command: command, TimeoutSec: 600,
			})
		}
	}

	sort.SliceStable(found, func(i, j int) bool { return found[i].Name < found[j].Name })
	return &Catalogue{Checks: found}
}

// nodeProjects maps a directory to the --prefix needed to run npm there.
// The repo root and a conventional frontend/ subdirectory are enough; walking
// the whole tree would pick up node_modules and every example project in it.
func nodeProjects(workspace string) map[string]string {
	out := map[string]string{}
	if exists(filepath.Join(workspace, "package.json")) {
		out[workspace] = ""
	}
	for _, sub := range []string{"frontend", "web", "ui", "client"} {
		dir := filepath.Join(workspace, sub)
		if exists(filepath.Join(dir, "package.json")) {
			out[dir] = sub
		}
	}
	return out
}

func scriptsIn(dir string) []string {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil
	}
	names := make([]string, 0, len(pkg.Scripts))
	for name := range pkg.Scripts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Merge returns a catalogue where entries from override replace discovered ones
// with the same name, and anything new is appended.
func (c *Catalogue) Merge(override *Catalogue) *Catalogue {
	if override == nil || len(override.Checks) == 0 {
		return c
	}
	byName := map[string]int{}
	merged := append([]Check(nil), c.Checks...)
	for i, check := range merged {
		byName[check.Name] = i
	}
	for _, check := range override.Checks {
		if i, ok := byName[check.Name]; ok {
			merged[i] = check
			continue
		}
		merged = append(merged, check)
	}
	return &Catalogue{Checks: merged}
}

// CatalogueFor is what callers should use: whatever the workspace offers, plus
// anything it declares explicitly.
func CatalogueFor(workspace string) (*Catalogue, error) {
	explicit, err := LoadCatalogue(workspace)
	if err != nil {
		return nil, err
	}
	return Discover(workspace).Merge(explicit), nil
}
