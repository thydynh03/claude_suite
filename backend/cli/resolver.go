package cli

import (
	"os"
	"os/exec"
	"path/filepath"
)

// ResolveClaudeCLI dynamic resolution across PATH, AppData, NPM global, and Program Files
func ResolveClaudeCLI() string {
	// 1. Check standard system PATH via LookPath
	if path, err := exec.LookPath("claude"); err == nil {
		return path
	}

	home, _ := os.UserHomeDir()
	appDataLocal := os.Getenv("LOCALAPPDATA")
	appDataRoaming := os.Getenv("APPDATA")
	programFiles := os.Getenv("ProgramFiles")
	programFilesX86 := os.Getenv("ProgramFiles(x86)")

	candidates := []string{
		// Windows AppData Local
		filepath.Join(appDataLocal, "AnthropicClaude", "claude.exe"),
		filepath.Join(appDataLocal, "Programs", "claude", "claude.exe"),
		filepath.Join(appDataLocal, "claude", "claude.exe"),

		// NPM Global
		filepath.Join(appDataRoaming, "npm", "claude.cmd"),
		filepath.Join(appDataRoaming, "npm", "claude.exe"),
		filepath.Join(appDataRoaming, "npm", "claude"),

		// User Home
		filepath.Join(home, ".claude", "bin", "claude.exe"),
		filepath.Join(home, ".claude", "bin", "claude.cmd"),
		filepath.Join(home, "AppData", "Local", "AnthropicClaude", "claude.exe"),

		// Program Files
		filepath.Join(programFiles, "AnthropicClaude", "claude.exe"),
		filepath.Join(programFilesX86, "AnthropicClaude", "claude.exe"),
	}

	for _, cand := range candidates {
		if cand != "" {
			if _, err := os.Stat(cand); err == nil {
				return cand
			}
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
		if cand != "" {
			if _, err := os.Stat(cand); err == nil {
				return cand
			}
		}
	}

	// Dynamic PATH directory scan fallback
	if found := scanPathEnv([]string{"agy.exe", "agy.cmd", "agy.bat", "antigravity.exe", "antigravity.cmd", "agy"}); found != "" {
		return found
	}

	return "agy"
}

func scanPathEnv(targetNames []string) string {
	pathEnv := os.Getenv("PATH")
	dirs := filepath.SplitList(pathEnv)

	for _, dir := range dirs {
		for _, name := range targetNames {
			full := filepath.Join(dir, name)
			if _, err := os.Stat(full); err == nil {
				return full
			}
		}
	}

	// Also check current working directory
	for _, name := range targetNames {
		if _, err := os.Stat(name); err == nil {
			if abs, err := filepath.Abs(name); err == nil {
				return abs
			}
			return name
		}
	}

	return ""
}

// GetDetectedCLIPaths returns resolved paths for diagnostics UI
func GetDetectedCLIPaths() map[string]string {
	return map[string]string{
		"claude":      ResolveClaudeCLI(),
		"antigravity": ResolveAntigravityCLI(),
	}
}
