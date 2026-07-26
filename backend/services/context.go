package services

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"claude_suite/backend/database"
	"claude_suite/backend/services/projectmap"
)

var TextExtensions = map[string]bool{
	".py": true, ".go": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
	".svelte": true, ".vue": true, ".html": true, ".css": true, ".scss": true, ".less": true,
	".json": true, ".md": true, ".txt": true, ".yaml": true, ".yml": true, ".toml": true,
	".sh": true, ".bat": true, ".ps1": true, ".sql": true, ".c": true, ".cpp": true,
	".h": true, ".hpp": true, ".rs": true, ".java": true, ".kt": true, ".dart": true,
	".mod": true, ".sum": true, ".spec": true, ".xml": true, ".env": true, ".ini": true,
	".conf": true, ".cfg": true, ".properties": true, ".lock": true, ".editorconfig": true,
	".eslintrc": true, ".prettierrc": true, ".babelrc": true, ".dockerignore": true,
	".gitignore": true, ".gitattributes": true, ".npmignore": true, ".cls": true,
}

var BinaryExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".ico": true, ".bmp": true,
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".bin": true, ".o": true,
	".a": true, ".pyc": true, ".pyd": true, ".zip": true, ".tar": true, ".gz": true,
	".7z": true, ".pdf": true, ".mp3": true, ".mp4": true, ".avi": true, ".woff": true,
	".woff2": true, ".ttf": true, ".eot": true, ".sqlite": true, ".db": true,
}

var IgnoredDirs = map[string]bool{
	"node_modules": true, ".git": true, "__pycache__": true, ".venv": true,
	"venv": true, "dist": true, "build": true, ".wails": true, ".idea": true, ".vscode": true,
	".next": true, "coverage": true,
}

type ContextManager struct {
	// Set via AttachRepos / AttachProjectMapper / AttachMemoryRepos
	// (taskpack.go). Nil is valid: BuildTaskPack degrades section by section
	// and the Quick CLI file-context path is unaffected.
	taskRepo       *database.TaskRepository
	attemptRepo    *database.AttemptRepository
	lessonRepo     *database.LessonRepository
	regressionRepo *database.RegressionRepository
	mapper         *projectmap.Mapper
}

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

		base := strings.ToLower(d.Name())
		ext := strings.ToLower(filepath.Ext(path))

		// Check if it's a known text file, or special filename without ext, or not binary
		isText := TextExtensions[ext] ||
			base == "dockerfile" || base == "makefile" || base == "license" || base == "readme" ||
			strings.HasPrefix(base, ".env") || strings.HasPrefix(base, ".git")

		if !isText && !BinaryExtensions[ext] {
			// Include any non-binary unknown extension by default
			isText = true
		}

		if isText {
			rel, errRel := filepath.Rel(root, path)
			if errRel == nil {
				files = append(files, rel)
			}
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

	included := 0
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
		included++
	}

	// No files made it in → return nothing. A header-only block is worse than
	// empty: prepended to a short question, the "WORKSPACE CONTEXT" heading
	// frames the WHOLE message as context, and the model answers "I don't see
	// a question or task in your message" — observed live from the Cockpit.
	if included == 0 {
		return ""
	}
	return sb.String()
}
