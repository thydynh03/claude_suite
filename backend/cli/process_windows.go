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
func newCLICommand(ctx context.Context, executable string, args []string, show bool) *exec.Cmd {
	cmd := exec.CommandContext(ctx, executable, args...)
	if show {
		// The executable runs directly in its own console — no `cmd.exe /c`
		// wrapper. The wrapper displayed nothing (stdout/stderr go to our
		// pipes either way) and it re-parsed Go's MSVCRT-quoted argv with
		// cmd's rules, so one double quote in a task prompt flipped the quote
		// state and made `&`, `|`, `>` live metacharacters: prompt text could
		// truncate the command or execute as commands.
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    false,
			CreationFlags: 0x00000010, // CREATE_NEW_CONSOLE
		}
	} else {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true,
			CreationFlags: 0x08000000, // CREATE_NO_WINDOW
		}
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
