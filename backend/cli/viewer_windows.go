//go:build windows

package cli

import (
	"os/exec"
	"strings"
	"syscall"
)

// spawnViewerConsole opens the detached console that tails a run's log file.
//
// Deliberately NOT Wait()ed on and NOT tied to the run's context: the window
// must outlive the run (that is its purpose), and nothing in the app may ever
// block on it — the `/k` lesson from ARCHITECTURE_DECISIONS §3 applies to any
// process whose lifetime a user's hand controls.
func spawnViewerConsole(logPath, title string) {
	// Single quotes in a PowerShell single-quoted string are escaped by
	// doubling; the path comes from os.CreateTemp but the rule costs nothing.
	psPath := strings.ReplaceAll(logPath, "'", "''")
	psTitle := strings.ReplaceAll(title, "'", "''")

	cmd := exec.Command(
		"powershell", "-NoLogo", "-NoProfile", "-NoExit", "-Command",
		"$Host.UI.RawUI.WindowTitle = 'Claude Suite — "+psTitle+"'; "+
			"Get-Content -LiteralPath '"+psPath+"' -Wait -Tail 1000",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    false,
		CreationFlags: 0x00000010, // CREATE_NEW_CONSOLE
	}
	if err := cmd.Start(); err != nil {
		return
	}
	// Release, never Wait: the viewer belongs to the user now.
	_ = cmd.Process.Release()
}
