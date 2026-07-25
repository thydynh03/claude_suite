package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnsureWithinWorkspace reports whether path resolves to somewhere inside root.
//
// filepath.Join already cleans "..", so the interesting case is a symlink that
// lives inside the workspace but points outside it — Join sees a tidy relative
// path and hands back a file from anywhere on disk. Both sides are resolved
// before comparing so the link is followed rather than trusted.
//
// A path that does not exist yet is still checked, by resolving its nearest
// existing ancestor and re-attaching the rest. Writing needs that: the file
// being created has no symlinks of its own, but the directory it lands in
// certainly can.
//
// An empty root means no workspace is selected and there is nothing to contain
// the path to, so the check passes.
//
// This lives in services rather than in either frontend on purpose: the Wails
// adapter and the TUI adapter both expose ReadFileContent, and for a while only
// one of them checked. One shared function is what stops that drifting again.
func EnsureWithinWorkspace(root, path string) error {
	if strings.TrimSpace(root) == "" {
		return nil
	}

	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve workspace: %w", err)
	}
	resolvedPath, err := resolveExisting(path)
	if err != nil {
		return err
	}

	rel, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path is outside the workspace: %s", path)
	}
	return nil
}

// resolveExisting resolves the longest existing prefix of path and re-attaches
// the components that do not exist yet.
//
// EvalSymlinks fails outright on a missing path, which would make every new file
// unwritable. Walking up finds the directory that will actually hold it, which is
// the part that can be a symlink and therefore the part worth resolving.
func resolveExisting(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	remainder := ""
	current := abs
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			if remainder == "" {
				return resolved, nil
			}
			return filepath.Join(resolved, remainder), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}

		parent := filepath.Dir(current)
		if parent == current {
			// Reached the volume root without finding anything that exists.
			return abs, nil
		}
		remainder = filepath.Join(filepath.Base(current), remainder)
		current = parent
	}
}
