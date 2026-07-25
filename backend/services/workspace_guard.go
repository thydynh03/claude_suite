package services

import (
	"fmt"
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
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}

	rel, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path is outside the workspace: %s", path)
	}
	return nil
}
