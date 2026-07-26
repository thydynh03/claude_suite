//go:build windows

package cli

import (
	"context"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"claude_suite/backend/sysproc"
)

// newCLICommand builds the agent process so that cancelling ctx kills the
// whole tree, not just the direct child.
//
// There is often an intermediary between us and the real agent: an npm shim
// (claude.cmd) is hosted by an implicit cmd.exe, so the agent is a grandchild.
// exec's default cancel is TerminateProcess on the direct child only, which
// orphaned the actual agent — still editing the workspace with
// --dangerously-skip-permissions after Stop or timeout — while the inherited
// pipe handles kept cmd.Wait() blocked far past the timeout, holding the
// worker slot with it.
func newCLICommand(ctx context.Context, executable string, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, executable, args...)
	// Always hidden. A CREATE_NEW_CONSOLE on the agent used to serve the
	// "show console" toggle, but stdout/stderr are piped to the app, so that
	// window was EMPTY and closed with the process before anyone could read
	// it. The visible window is now the detached tail viewer (viewer.go) —
	// it shows the streamed output and outlives the run.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}

	// taskkill /T walks the descendant tree, so a shim's grandchild dies with
	// its host instead of surviving the kill.
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		kill := sysproc.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid))
		if err := kill.Run(); err != nil {
			// taskkill fails on an already-gone PID; the plain kill both
			// covers that and keeps exec's own bookkeeping intact.
			return cmd.Process.Kill()
		}
		return nil
	}
	// Even a descendant taskkill missed must not pin our pipes open forever:
	// after cancel or child exit, Wait force-closes them within this bound so
	// the scanner goroutines and the worker slot are released.
	cmd.WaitDelay = 10 * time.Second
	return cmd
}
