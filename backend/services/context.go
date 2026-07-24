package services

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var TextExtensions = map[string]bool{
	".py": true, ".go": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
	".svelte": true, ".vue": true, ".html": true, ".css": true, ".json": true,
	".md": true, ".txt": true, ".yaml": true, ".yml": true, ".toml": true,
	".sh": true, ".bat": true, ".ps1": true, ".sql": true, ".c": true, ".cpp": true,
	".h": true, ".hpp": true, ".rs": true, ".java": true, ".kt": true, ".dart": true,
	".mod": true, ".sum": true, ".spec": true, ".xml": true, ".env": true,
}

var IgnoredDirs = map[string]bool{
	"node_modules": true, ".git": true, "__pycache__": true, ".venv": true,
	"venv": true, "dist": true, "build": true, ".wails": true, ".idea": true, ".vscode": true,
}

type ContextManager struct{}

func NewContextManager() *ContextManager {
	return &ContextManager{}
}

func (c *ContextManager) ScanWorkspace(root string) ([]string, error) {
	var files []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if IgnoredDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if TextExtensions[ext] {
			rel, _ := filepath.Rel(root, path)
			files = append(files, rel)
		}
		return nil
	})

	return files, err
}

func (c *ContextManager) BuildContextPrompt(root string, paths []string) string {
	var sb strings.Builder
	sb.WriteString("### WORKSPACE CONTEXT & SOURCE FILES ###\n\n")

	if root != "" {
		sb.WriteString(fmt.Sprintf("Project Root Directory: %s\n\n", root))
	}

	for _, p := range paths {
		fullPath := p
		if !filepath.IsAbs(p) && root != "" {
			fullPath = filepath.Join(root, p)
		}

		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		relPath := p
		if root != "" {
			if r, err := filepath.Rel(root, fullPath); err == nil {
				relPath = r
			}
		}

		sb.WriteString(fmt.Sprintf("File: %s\n```\n%s\n```\n\n", relPath, string(content)))
	}

	return sb.String()
}
