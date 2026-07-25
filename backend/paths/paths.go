// Package paths resolves where Claude Suite keeps per-user state: the SQLite
// database, the JSON configs, the agent role markdown and the logs.
//
// It exists as its own package so that both database and logger can agree on
// one location without one importing the other.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DataDirEnv overrides the data directory. Useful for tests, for portable
// installs, and for staying on a legacy location without editing code.
const DataDirEnv = "CLAUDE_SUITE_DATA_DIR"

// DataDir is the per-user directory holding all application state.
//
// On Windows this is %LOCALAPPDATA%\ClaudeSuite, matching where the browser
// agent already keeps its Chrome profile.
func DataDir() string {
	if v := strings.TrimSpace(os.Getenv(DataDirEnv)); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		if base := os.Getenv("LOCALAPPDATA"); base != "" {
			return filepath.Join(base, "ClaudeSuite")
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".claude_suite")
	}
	return "."
}

// EnsureDataDir returns DataDir, creating it if necessary.
func EnsureDataDir() string {
	dir := DataDir()
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

// LogDir is where rotating application logs are written.
func LogDir() string {
	return filepath.Join(DataDir(), "logs")
}

// LegacyDataDirs lists locations used by earlier builds, so their contents can
// be adopted once instead of silently abandoned.
//
// `e:\exe` was hardcoded in GetDBPath: it is the developer's checkout, and any
// machine that merely happened to have that path would have had its state
// written there too.
func LegacyDataDirs() []string {
	var dirs []string
	if runtime.GOOS == "windows" {
		dirs = append(dirs, `e:\exe`)
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".claude_suite"))
	}

	current := filepath.Clean(DataDir())
	out := dirs[:0]
	for _, dir := range dirs {
		if !strings.EqualFold(filepath.Clean(dir), current) {
			out = append(out, dir)
		}
	}
	return out
}

// StateFiles are the JSON configs that live alongside the database.
var StateFiles = []string{
	"ui_config.json",
	"workspace_config.json",
	"integrations_config.json",
	"gcp_oauth.json",
	"anti_accounts.json",
}

// StateDirs are the directories that live alongside the database.
var StateDirs = []string{"roles"}

// DatabaseFile is the SQLite filename inside the data directory.
const DatabaseFile = "agent_manager.db"
