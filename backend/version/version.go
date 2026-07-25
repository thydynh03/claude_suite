package version

import (
	"os/exec"
	"strings"
)

// CurrentVersion is the default or build-time injected version string
var CurrentVersion = "v2.8.0"

// GetVersion returns the dynamic version from Git tags if available, or CurrentVersion
func GetVersion() string {
	cmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	if out, err := cmd.Output(); err == nil {
		tag := strings.TrimSpace(string(out))
		if tag != "" {
			return tag
		}
	}
	return CurrentVersion
}
