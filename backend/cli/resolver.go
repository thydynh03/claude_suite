package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ResolveClaudeCLI finds the Claude Code CLI across PATH, the native
// installer's bin dir, and npm's global dir.
//
// %LOCALAPPDATA%\AnthropicClaude is deliberately NOT searched: that is where
// the Claude Desktop GUI installs, and its claude.exe is not the CLI. Picking
// it up launched the desktop app hidden with CLI arguments it ignores — the
// window never appeared, no stream-json ever arrived, and every task sat there
// until the 10-minute timeout killed it, forever, with nothing ever saying the
// CLI was not installed.
func ResolveClaudeCLI() string {
	// 1. Check standard system PATH via LookPath
	if path, err := exec.LookPath("claude"); err == nil {
		return path
	}

	home, _ := os.UserHomeDir()
	appDataLocal := os.Getenv("LOCALAPPDATA")
	appDataRoaming := os.Getenv("APPDATA")

	candidates := []string{
		// Native installer (the path `claude install` uses on Windows)
		filepath.Join(home, ".local", "bin", "claude.exe"),
		filepath.Join(home, ".local", "bin", "claude"),

		// NPM Global
		filepath.Join(appDataRoaming, "npm", "claude.cmd"),
		filepath.Join(appDataRoaming, "npm", "claude.exe"),
		filepath.Join(appDataRoaming, "npm", "claude"),

		// User Home / local program dirs
		filepath.Join(home, ".claude", "bin", "claude.exe"),
		filepath.Join(home, ".claude", "bin", "claude.cmd"),
		filepath.Join(appDataLocal, "Programs", "claude", "claude.exe"),
		filepath.Join(appDataLocal, "claude", "claude.exe"),
	}

	for _, cand := range candidates {
		if isExecutableFile(cand) {
			return cand
		}
	}

	// Dynamic PATH directory scan fallback
	if found := scanPathEnv([]string{"claude.exe", "claude.cmd", "claude.bat", "claude"}); found != "" {
		return found
	}

	return "claude"
}

// ResolveAntigravityCLI dynamic resolution across PATH, AppData, NPM, AGY bin
func ResolveAntigravityCLI() string {
	// 1. Check standard system PATH via LookPath
	if path, err := exec.LookPath("agy"); err == nil {
		return path
	}
	if path, err := exec.LookPath("antigravity"); err == nil {
		return path
	}

	home, _ := os.UserHomeDir()
	appDataLocal := os.Getenv("LOCALAPPDATA")
	appDataRoaming := os.Getenv("APPDATA")

	candidates := []string{
		// AGY / Antigravity standard install paths
		filepath.Join(appDataLocal, "agy", "bin", "agy.exe"),
		filepath.Join(appDataLocal, "antigravity", "bin", "agy.exe"),
		filepath.Join(appDataLocal, "Programs", "agy", "agy.exe"),

		// Gemini / Antigravity user home paths
		filepath.Join(home, ".gemini", "antigravity", "bin", "agy.exe"),
		filepath.Join(home, ".gemini", "antigravity", "bin", "antigravity.exe"),
		filepath.Join(home, ".agy", "bin", "agy.exe"),
		filepath.Join(home, "AppData", "Local", "agy", "bin", "agy.exe"),

		// NPM Global
		filepath.Join(appDataRoaming, "npm", "agy.cmd"),
		filepath.Join(appDataRoaming, "npm", "antigravity.cmd"),
	}

	for _, cand := range candidates {
		if isExecutableFile(cand) {
			return cand
		}
	}

	// Dynamic PATH directory scan fallback
	if found := scanPathEnv([]string{"agy.exe", "agy.cmd", "agy.bat", "antigravity.exe", "antigravity.cmd", "agy"}); found != "" {
		return found
	}

	return "agy"
}

// isExecutableFile rejects directories, which a bare os.Stat accepts: an
// extracted folder named "claude" beside the exe was returned as the CLI and
// produced "%1 is not a valid Win32 application" for a user whose setup was
// otherwise fine.
func isExecutableFile(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	// On Windows only a handful of extensions are runnable; a data file that
	// happens to be named "claude" is not a CLI.
	switch strings.ToLower(filepath.Ext(path)) {
	case ".exe", ".cmd", ".bat", ".com", ".ps1", "":
		return true
	default:
		return false
	}
}

// scanPathEnv walks PATH only. The process's current working directory used to
// be searched too — for a portable exe that is the user's Downloads folder,
// so any previously downloaded file named claude.exe was executed as the CLI.
func scanPathEnv(targetNames []string) string {
	pathEnv := os.Getenv("PATH")
	dirs := filepath.SplitList(pathEnv)

	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		for _, name := range targetNames {
			full := filepath.Join(dir, name)
			if isExecutableFile(full) {
				return full
			}
		}
	}

	return ""
}

// CLIInstalled reports whether a real binary was found, as opposed to the bare
// fallback name that exec will fail to resolve. Callers use it to say "the CLI
// is not installed" instead of surfacing a raw exec error.
func CLIInstalled(resolved string) bool {
	return resolved != "" && resolved != "claude" && resolved != "agy"
}

// GetDetectedCLIPaths returns resolved paths for diagnostics UI
func GetDetectedCLIPaths() map[string]string {
	return map[string]string{
		"claude":      ResolveClaudeCLI(),
		"antigravity": ResolveAntigravityCLI(),
	}
}
