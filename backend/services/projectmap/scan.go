// Package projectmap builds and serves the deterministic project map: a
// per-workspace code graph stored in SQLite, fingerprinted for cheap
// incremental updates, rendered as bounded context for sub-agent prompts.
//
// The schema, node-ID convention and fingerprint algorithm are ported from
// Understand-Anything (MIT); the implementation is pure Go — no Node, no
// Python, no tree-sitter. See docs/MEMORY_CODEGRAPH_PLAN.md.
//
// This package must not import backend/services (services imports it).
package projectmap

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ScannedFile is one file the mapper will fingerprint and (when a structural
// extractor exists) parse. Path is workspace-relative with forward slashes.
type ScannedFile struct {
	Path     string
	Language string // go|typescript|javascript|svelte|config|doc|other
	Size     int64
}

// skippedDirs are never descended into. Generated trees (wailsjs, dist) would
// drown the map in nodes nobody asked about.
var skippedDirs = map[string]bool{
	"node_modules": true, ".git": true, "__pycache__": true, ".venv": true,
	"venv": true, "dist": true, "build": true, ".wails": true, ".idea": true,
	".vscode": true, ".next": true, "coverage": true, "wailsjs": true,
	".claude-suite": true, ".ua": true, "bin": true, "obj": true,
	// .claude holds agent-tool state INCLUDING nested git worktrees
	// (.claude/worktrees/<name> is a full copy of the repo while a background
	// task runs). Scanning it doubles every node under a path nothing else
	// references, and the duplicates crowd the real files out of the pack.
	".claude": true,
}

// maxScannedFileSize skips generated monsters (lockfiles, bundles): they cost
// hashing time and contribute nothing a sub-agent needs.
const maxScannedFileSize = 512 * 1024

var languageByExt = map[string]string{
	".go":     "go",
	".ts":     "typescript",
	".tsx":    "typescript",
	".js":     "javascript",
	".jsx":    "javascript",
	".mjs":    "javascript",
	".svelte": "svelte",
	".json":   "config",
	".yaml":   "config",
	".yml":    "config",
	".toml":   "config",
	".mod":    "config", // go.mod — the module manifest agents look for first
	".md":     "doc",
}

// ScanWorkspace inventories the files worth mapping.
func ScanWorkspace(root string) ([]ScannedFile, error) {
	var files []ScannedFile

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skippedDirs[d.Name()] || strings.HasPrefix(d.Name(), ".trash") {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		lang, known := languageByExt[ext]
		if !known {
			return nil
		}
		// Lockfiles and generated artifacts add noise, not knowledge.
		base := strings.ToLower(d.Name())
		if base == "package-lock.json" || base == "pnpm-lock.yaml" || strings.HasSuffix(base, ".min.js") {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > maxScannedFileSize {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		files = append(files, ScannedFile{
			Path:     filepath.ToSlash(rel),
			Language: lang,
			Size:     info.Size(),
		})
		return nil
	})

	return files, err
}

// ReadFile reads one scanned file's content from the workspace.
func ReadFile(root, relPath string) ([]byte, error) {
	return os.ReadFile(filepath.Join(root, filepath.FromSlash(relPath)))
}
