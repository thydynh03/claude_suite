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
	// Last resort: the temp dir, never ".". A portable exe run from Program
	// Files or a network share would otherwise try to create its database
	// beside the executable and be denied, with no channel to say why.
	return filepath.Join(os.TempDir(), "ClaudeSuite")
}

// EnsureDataDir returns DataDir, creating it if necessary.
func EnsureDataDir() string {
	dir, _ := EnsureDataDirErr()
	return dir
}

// EnsureDataDirErr is EnsureDataDir for callers that can report the failure.
// Everything downstream — the database, every config file — depends on this
// directory existing, and swallowing the error meant a profile where it could
// not be created produced a string of unexplained failures instead of one
// clear message.
func EnsureDataDirErr() (string, error) {
	dir := DataDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return dir, err
	}
	return dir, nil
}

// LogDir is where rotating application logs are written.
func LogDir() string {
	return filepath.Join(DataDir(), "logs")
}

// LegacyDataDirEnv points at a directory from an older install whose contents
// should be adopted once. It exists so that no build ships someone's machine
// layout baked into it.
const LegacyDataDirEnv = "CLAUDE_SUITE_LEGACY_DATA_DIR"

// LegacyDataDirs lists locations earlier builds could have written to, so their
// contents can be adopted once instead of silently abandoned.
//
// Only ~/.claude_suite is listed by default, because that is the only legacy
// location a downloaded copy of the app could ever have used. Earlier builds
// also hardcoded a specific developer checkout, but that path means nothing on
// anyone else's machine — someone who needs it points LegacyDataDirEnv at it
// rather than having it compiled in.
func LegacyDataDirs() []string {
	var dirs []string
	if v := strings.TrimSpace(os.Getenv(LegacyDataDirEnv)); v != "" {
		dirs = append(dirs, v)
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
