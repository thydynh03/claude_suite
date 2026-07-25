package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDataDirHonoursOverride(t *testing.T) {
	t.Setenv(DataDirEnv, `X:\somewhere\else`)
	if got := DataDir(); got != `X:\somewhere\else` {
		t.Fatalf("DataDir() = %q, want the override", got)
	}
	if want := filepath.Join(`X:\somewhere\else`, "logs"); LogDir() != want {
		t.Fatalf("LogDir() = %q, want %q", LogDir(), want)
	}
}

func TestDataDirIsUnderTheUserProfile(t *testing.T) {
	t.Setenv(DataDirEnv, "")
	dir := DataDir()
	if dir == "" || dir == "." {
		t.Fatalf("DataDir() = %q, want a real per-user location", dir)
	}

	// Whatever it resolves to must live under a per-user root, never a fixed
	// absolute path that happens to exist on one machine.
	var roots []string
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		roots = append(roots, v)
	}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots, home)
	}
	for _, root := range roots {
		if strings.HasPrefix(strings.ToLower(dir), strings.ToLower(root)) {
			return
		}
	}
	t.Fatalf("DataDir() = %q is not under any of %v", dir, roots)
}

// Earlier builds hardcoded one developer's checkout as the data directory. That
// path is meaningless on a machine that merely downloaded the app, so it must
// not be compiled into the legacy list — anyone who needs it sets the env var.
func TestLegacyDataDirsHaveNoMachineSpecificPath(t *testing.T) {
	t.Setenv(DataDirEnv, filepath.Join(t.TempDir(), "current"))
	t.Setenv(LegacyDataDirEnv, "")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory on this machine")
	}

	for _, dir := range LegacyDataDirs() {
		if !strings.HasPrefix(strings.ToLower(dir), strings.ToLower(home)) {
			t.Fatalf("legacy dir %q is not under the user's home; hardcoded paths do not exist on other machines", dir)
		}
	}
}

func TestLegacyDataDirsAcceptOverrideAndSkipCurrent(t *testing.T) {
	current := filepath.Join(t.TempDir(), "current")
	t.Setenv(DataDirEnv, current)
	t.Setenv(LegacyDataDirEnv, `D:\old-install`)

	dirs := LegacyDataDirs()
	if len(dirs) == 0 || dirs[0] != `D:\old-install` {
		t.Fatalf("LegacyDataDirs() = %v, want the override first", dirs)
	}
	for _, dir := range dirs {
		if strings.EqualFold(filepath.Clean(dir), filepath.Clean(current)) {
			t.Fatal("the current data directory must not be listed as legacy")
		}
	}
}
