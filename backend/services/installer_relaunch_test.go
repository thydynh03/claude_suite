package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func installerScript(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "build", "windows", "installer", "project.nsi")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read %s: %v", path, err)
	}
	return string(data)
}

// The updater exits the running app and hands over to the installer, because
// NSIS cannot replace an executable the app is holding open. So the installer is
// the only thing left that can bring the app back — without a finish-page run
// the update ends with an empty desktop and no explanation.
//
// This asserts the .nsi file rather than any Go code on purpose. `wails build`
// regenerates the stock template over project.nsi when the file is not tracked,
// and that has already happened once here: the companion CLI tools silently
// vanished from three releases that way. A relaunch that quietly disappears in a
// template refresh looks exactly like an update that crashed the app.
func TestInstallerRelaunchesTheAppWhenItFinishes(t *testing.T) {
	nsi := installerScript(t)

	for _, want := range []string{
		"MUI_FINISHPAGE_RUN",
		"MUI_FINISHPAGE_RUN_FUNCTION LaunchAppAsUser",
		"Function LaunchAppAsUser",
	} {
		if !strings.Contains(nsi, want) {
			t.Errorf("project.nsi no longer contains %q — the app will not reopen after an update", want)
		}
	}
}

// The installer is RequestExecutionLevel admin. MUI's default finish-page run
// uses Exec, which hands the app the installer's elevated token — and this app
// spawns sub-agents with --dangerously-skip-permissions, so an elevated app
// means elevated agents against the user's workspace. Going through
// explorer.exe, which already runs as the logged-in user, drops back to medium
// integrity. An elevated window also refuses drag-and-drop from Explorer with
// no error at all, which reads as the app being broken.
func TestTheRelaunchDoesNotInheritTheInstallersElevation(t *testing.T) {
	nsi := installerScript(t)

	start := strings.Index(nsi, "Function LaunchAppAsUser")
	if start < 0 {
		t.Fatal("LaunchAppAsUser is gone")
	}
	end := strings.Index(nsi[start:], "FunctionEnd")
	if end < 0 {
		t.Fatal("LaunchAppAsUser has no FunctionEnd")
	}
	body := nsi[start : start+end]

	if !strings.Contains(body, `$WINDIR\explorer.exe`) {
		t.Errorf("the relaunch no longer goes through explorer.exe, so the app starts elevated:\n%s", body)
	}
	if !strings.Contains(body, "${PRODUCT_EXECUTABLE}") {
		t.Errorf("the relaunch does not name the product executable:\n%s", body)
	}
}

// Two guards that already cost a release each, kept here because this file is
// the one thing that reads project.nsi in CI.
func TestInstallerStillShipsTheCompanionTools(t *testing.T) {
	nsi := installerScript(t)

	for _, tool := range []string{"claude-suite-claim.exe", "claude-suite-tui.exe"} {
		if !strings.Contains(nsi, tool) {
			t.Errorf("project.nsi no longer packages %s — the join command the app hands out names that path", tool)
		}
	}
	// ${__FILEDIR__} anchors those File lines to the script instead of to
	// makensis's working directory; without it they resolve to nothing and NSIS
	// says so only at build time.
	if !strings.Contains(nsi, "${__FILEDIR__}") {
		t.Error("the File paths are no longer anchored to the script directory")
	}
}

// An update must land on top of the existing installation. Without this the
// in-app updater installs a second copy in Program Files and orphans the one
// the user actually launches.
func TestInstallerReusesTheExistingInstallLocation(t *testing.T) {
	nsi := installerScript(t)

	if !strings.Contains(nsi, `"InstallLocation" "$INSTDIR"`) {
		t.Error("the install location is no longer recorded, so the next update cannot find it")
	}
	if !strings.Contains(nsi, "ReadRegStr $R1") || !strings.Contains(nsi, "StrCpy $INSTDIR $R1") {
		t.Error("the installer no longer reads back where it was installed")
	}
}
