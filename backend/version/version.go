package version

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// BuildVersion is injected at compile time via -ldflags "-X claude_suite/backend/version.BuildVersion=v..."
var BuildVersion = "v2.8.0"

// CurrentVersion provides backward compatibility
var CurrentVersion = "v2.8.0"

type VersionInfo struct {
	Version string `json:"version"`
}

// SetInstalledVersion persists the newly installed version to version.json
func SetInstalledVersion(ver string) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	vFile := filepath.Join(filepath.Dir(exePath), "version.json")
	data, _ := json.MarshalIndent(VersionInfo{Version: ver}, "", "  ")
	return os.WriteFile(vFile, data, 0644)
}

// GetVersion dynamically resolves current application version:
// 1. Persisted version.json (updated upon auto-update download)
// 2. Local Git tag (`git describe --tags --abbrev=0`)
// 3. BuildVersion (-ldflags or default)
func GetVersion() string {
	// 1. Persisted version.json
	if exePath, err := os.Executable(); err == nil {
		vFile := filepath.Join(filepath.Dir(exePath), "version.json")
		if data, err := os.ReadFile(vFile); err == nil {
			var info VersionInfo
			if err := json.Unmarshal(data, &info); err == nil && strings.TrimSpace(info.Version) != "" {
				return strings.TrimSpace(info.Version)
			}
		}
	}

	// 2. Local Git Tag
	cmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	if out, err := cmd.Output(); err == nil {
		tag := strings.TrimSpace(string(out))
		if tag != "" {
			return tag
		}
	}

	// 3. Build-time injected version
	return BuildVersion
}
