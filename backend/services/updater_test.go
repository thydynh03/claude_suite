package services

import (
	"strings"
	"testing"
)

// The v2.15.0 release ships exactly these four .exe assets — names verified
// against the live release (the installer is agent_center-…, from wails.json's
// `name`, not AgentCenter-…). Upload order is not guaranteed, so the chooser
// must work by name, not position.
func releaseAssets() []ReleaseAsset {
	return []ReleaseAsset{
		{Name: "agent-center-claim.exe", BrowserDownloadURL: "https://example.com/agent-center-claim.exe"},
		{Name: "agent-center-tui.exe", BrowserDownloadURL: "https://example.com/agent-center-tui.exe"},
		{Name: "agent_center-amd64-installer.exe", BrowserDownloadURL: "https://example.com/agent_center-amd64-installer.exe"},
		{Name: "AgentCenter.exe", BrowserDownloadURL: "https://example.com/AgentCenter.exe"},
	}
}

func TestChooseAssetURLInstalledPrefersInstaller(t *testing.T) {
	got := chooseAssetURL(releaseAssets(), true)
	if want := "https://example.com/agent_center-amd64-installer.exe"; got != want {
		t.Fatalf("installed copy chose %q, want %q", got, want)
	}
}

func TestChooseAssetURLPortablePrefersBareExe(t *testing.T) {
	got := chooseAssetURL(releaseAssets(), false)
	if want := "https://example.com/AgentCenter.exe"; got != want {
		t.Fatalf("portable copy chose %q, want %q", got, want)
	}
}

// The bug that shipped: the old picker took the first asset ending in .exe,
// which depending on order was a companion CLI tool. Neither mode may ever
// select claim or tui.
func TestChooseAssetURLNeverPicksCompanionTools(t *testing.T) {
	assets := []ReleaseAsset{
		{Name: "agent-center-claim.exe", BrowserDownloadURL: "https://example.com/claim.exe"},
		{Name: "agent-center-tui.exe", BrowserDownloadURL: "https://example.com/tui.exe"},
	}
	for _, installed := range []bool{true, false} {
		if got := chooseAssetURL(assets, installed); got != "" {
			t.Fatalf("installed=%v chose companion tool %q, want empty", installed, got)
		}
	}
}

func TestChooseAssetURLFallsBackAcrossModes(t *testing.T) {
	onlyInstaller := []ReleaseAsset{
		{Name: "AgentCenter-amd64-installer.exe", BrowserDownloadURL: "https://example.com/installer.exe"},
	}
	if got := chooseAssetURL(onlyInstaller, false); got != "https://example.com/installer.exe" {
		t.Fatalf("portable copy with installer-only release chose %q", got)
	}
	onlyPortable := []ReleaseAsset{
		{Name: "AgentCenter.exe", BrowserDownloadURL: "https://example.com/portable.exe"},
	}
	if got := chooseAssetURL(onlyPortable, true); got != "https://example.com/portable.exe" {
		t.Fatalf("installed copy with portable-only release chose %q", got)
	}
}

func TestChooseAssetURLIgnoresNonExeAssets(t *testing.T) {
	assets := []ReleaseAsset{
		{Name: "checksums.txt", BrowserDownloadURL: "https://example.com/checksums.txt"},
		{Name: "source.zip", BrowserDownloadURL: "https://example.com/source.zip"},
	}
	if got := chooseAssetURL(assets, true); got != "" {
		t.Fatalf("chose non-exe asset %q", got)
	}
}

func TestBuildUpdaterBatCarriesBothPathsAndBoundsTheRetry(t *testing.T) {
	bat := buildUpdaterBat(`C:\Temp\AgentCenter-update-1.exe`, `C:\Apps\AgentCenter.exe`)
	for _, want := range []string{
		`set "NEW_EXE=C:\Temp\AgentCenter-update-1.exe"`,
		`set "OLD_EXE=C:\Apps\AgentCenter.exe"`,
		// copy, not move: a same-volume move keeps the %TEMP% owner-only ACL
		// on the app exe, locking other accounts out of it.
		`copy /b /y "%NEW_EXE%" "%OLD_EXE%"`,
		// Giving up must still fall through to the relaunch, or a transient
		// lock leaves the user with an app that closed and never came back —
		// but it goes via :GAVEUP first, which drops the marker that lets the
		// next check say the swap failed instead of silently re-offering it.
		`if %COUNT% GEQ 60 goto GAVEUP`,
		`:GAVEUP`,
		// cmd.exe reads a .bat in the console's OEM codepage; without this a
		// Vietnamese path arrived as mojibake and the copy could never match.
		`chcp 65001`,
		`del "%NEW_EXE%"`,
		`start "" "%OLD_EXE%"`,
		`del "%~f0"`,
	} {
		if !strings.Contains(bat, want) {
			t.Fatalf("bat missing %q:\n%s", want, bat)
		}
	}
	if strings.Contains(bat, "move ") {
		t.Fatalf("bat swaps with move, which keeps the temp dir's ACL:\n%s", bat)
	}
	// The relaunch may not be conditional on the copy having succeeded: the
	// only line allowed between the DONE label and `start` is the download
	// cleanup.
	if !strings.Contains(bat, ":DONE\ndel \"%NEW_EXE%\" > NUL 2>&1\nstart \"\" \"%OLD_EXE%\"") {
		t.Fatalf("relaunch is not unconditional after :DONE:\n%s", bat)
	}
	// A stray unescaped %s or %d in the template would leave fmt noise behind.
	if strings.Contains(bat, "%!") {
		t.Fatalf("bat contains a fmt formatting error:\n%s", bat)
	}
}

// The audience for this app is Vietnamese, so %TEMP% routinely carries an
// accented profile name and portable copies sit in folders like "Phần mềm".
// cmd.exe decodes a .bat in the console codepage, so the switch to UTF-8 has
// to come before any line that carries a path — otherwise the copy target is
// mojibake, all 60 attempts fail, and the app never relaunches.
func TestBuildUpdaterBatSwitchesToUTF8BeforeAnyPath(t *testing.T) {
	bat := buildUpdaterBat(`C:\Users\Trần\AppData\Local\Temp\up.exe`, `D:\Phần mềm\AgentCenter.exe`)

	chcp := strings.Index(bat, "chcp 65001")
	newExe := strings.Index(bat, "set \"NEW_EXE=")
	if chcp < 0 {
		t.Fatalf("no codepage switch at all:\n%s", bat)
	}
	if chcp > newExe {
		t.Fatalf("codepage switch comes after the paths, too late to help:\n%s", bat)
	}
	if !strings.Contains(bat, `set "OLD_EXE=D:\Phần mềm\AgentCenter.exe"`) {
		t.Fatalf("accented path did not survive into the script:\n%s", bat)
	}
}

// Batch expands %VAR% while reading a line, so a single % inside a path eats
// the text after it and the copy silently targets a truncated name.
func TestBuildUpdaterBatEscapesPercentInPaths(t *testing.T) {
	bat := buildUpdaterBat(`C:\Temp\up.exe`, `D:\100%% Work\AgentCenter.exe`)
	if !strings.Contains(bat, `set "OLD_EXE=D:\100%%%% Work\AgentCenter.exe"`) {
		t.Fatalf("literal percent was not doubled for batch:\n%s", bat)
	}
}

func TestIsInstallerAssetLooksOnlyAtTheFilename(t *testing.T) {
	if !isInstallerAsset("https://github.com/thydynh03/Agent_Center/releases/download/v2.15.0/agent_center-amd64-installer.exe") {
		t.Error("the real installer asset was not recognized")
	}
	// The release tag is part of the URL; a tag mentioning "installer" must
	// not reroute a portable exe through the installer branch.
	if isInstallerAsset("https://github.com/thydynh03/Agent_Center/releases/download/v2.16.0-installer-fix/AgentCenter.exe") {
		t.Error("a tag containing 'installer' misclassified the portable exe")
	}
}

// `!=` was the old update test: a yanked newest release read as an "update"
// and silently downgraded every install already past it.
func TestIsNewerVersionIsAnOrderNotADifference(t *testing.T) {
	cases := []struct {
		candidate, current string
		want               bool
	}{
		{"v2.16.0", "v2.15.0", true},
		{"v2.15.0", "v2.16.0", false}, // the downgrade that shipped as an update
		{"v2.15.0", "v2.15.0", false},
		{"v2.15.1", "v2.15.0", true},
		{"v3.0.0", "v2.99.99", true},
		{"v2.16.0-rc1", "v2.16.0", false}, // prerelease compares as its base
		{"v2.16.0", "v0.0.0-dev", true},   // dev builds accept any release
		{"garbage", "v2.15.0", false},     // an unparsable tag is not an update
		{"v2.16.0", "garbage", true},
	}
	for _, c := range cases {
		if got := isNewerVersion(c.candidate, c.current); got != c.want {
			t.Errorf("isNewerVersion(%q, %q) = %v, want %v", c.candidate, c.current, got, c.want)
		}
	}
}

func TestDirWritable(t *testing.T) {
	if !dirWritable(t.TempDir()) {
		t.Fatal("temp dir reported unwritable")
	}
	if dirWritable(`C:\this-directory-does-not-exist-agent-center`) {
		t.Fatal("nonexistent dir reported writable")
	}
}
