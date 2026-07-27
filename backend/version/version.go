package version

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"agent_center/backend/paths"
)

// devVersion is what an untagged local build reports. Seeing it in the UI means
// the binary was not built by the release workflow.
//
// It keeps the leading "v" every tag has: TestVersionString asserts that shape,
// and a version that does not look like a version is its own kind of confusing.
const devVersion = "v0.0.0-dev"

// BuildVersion is injected at release time with
// -ldflags "-X agent_center/backend/version.BuildVersion=$TAG".
// Leaving the default as "dev" is deliberate: a hardcoded real-looking version
// is why every release kept reporting v2.8.0 no matter which tag built it.
var BuildVersion = devVersion

// CurrentVersion provides backward compatibility
var CurrentVersion = devVersion

type VersionInfo struct {
	Version string `json:"version"`
}

// versionFile lives in the data directory, not beside the executable: an
// installed build sits in Program Files, which a normal user cannot write to,
// so persisting the post-update version there silently failed.
func versionFile() string {
	return filepath.Join(paths.EnsureDataDir(), "version.json")
}

// SetInstalledVersion persists the newly installed version.
func SetInstalledVersion(ver string) error {
	data, _ := json.MarshalIndent(VersionInfo{Version: strings.TrimSpace(ver)}, "", "  ")
	return os.WriteFile(versionFile(), data, 0o600)
}

// GetVersion resolves the running application's version.
//
// The build-time value comes first. It is injected by the release workflow from
// the git tag, so a tagged build always reports itself correctly and cannot be
// contradicted by leftover state.
//
// version.json is only consulted when nothing was injected — a developer build.
// It is written by the auto-updater after it downloads a newer release.
//
// There used to be a `git describe --tags` step between the two. It had to go:
// in an installed app it reports the tag of whatever repository the user happens
// to have launched from, which is never this application's version, and it spawned
// a console window on every call.
func GetVersion() string {
	if v := strings.TrimSpace(BuildVersion); v != "" && v != devVersion {
		return v
	}

	if data, err := os.ReadFile(versionFile()); err == nil {
		var info VersionInfo
		if err := json.Unmarshal(data, &info); err == nil && strings.TrimSpace(info.Version) != "" {
			return strings.TrimSpace(info.Version)
		}
	}

	return BuildVersion
}
