// Package defaults ships the starter content a fresh install needs.
//
// These files used to live at the repo root and were only ever found because
// one developer's data directory happened to be the checkout itself. Anyone who
// downloaded a built binary got a handful of one-line stubs instead of the real
// role definitions. Embedding them means the binary carries its own defaults.
package defaults

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed roles/*.md
var rolesFS embed.FS

// RoleNames lists the bundled role files.
func RoleNames() ([]string, error) {
	entries, err := fs.ReadDir(rolesFS, "roles")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

// RoleContent returns one bundled role definition.
func RoleContent(name string) ([]byte, error) {
	return rolesFS.ReadFile("roles/" + name)
}

// SeedRoles writes any missing bundled role into dir, creating it if needed.
//
// Existing files are never overwritten: a user who edited a role keeps their
// version, and a new role added in a later release still shows up.
func SeedRoles(dir string) (written []string, err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create roles dir %s: %w", dir, err)
	}

	names, err := RoleNames()
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		target := filepath.Join(dir, name)
		if _, statErr := os.Stat(target); statErr == nil {
			continue
		}
		content, readErr := RoleContent(name)
		if readErr != nil {
			return written, readErr
		}
		if writeErr := os.WriteFile(target, content, 0o644); writeErr != nil {
			return written, fmt.Errorf("write role %s: %w", name, writeErr)
		}
		written = append(written, name)
	}
	return written, nil
}
