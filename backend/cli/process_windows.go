//go:build windows

package cli

import (
	"context"
	"os/exec"
	"syscall"
)

func newCLICommand(ctx context.Context, executable string, args []string, show bool) *exec.Cmd {
	if show {
		// /c, not /k: /k leaves the shell alive after the tool exits, so cmd.Wait()
		// would block until the user closes the console by hand and the task would
		// never complete. The new console still shows the output either way.
		cmdArgs := append([]string{"/c", executable}, args...)
		cmd := exec.CommandContext(ctx, "cmd.exe", cmdArgs...)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    false,
			CreationFlags: 0x00000010, // CREATE_NEW_CONSOLE
		}
		return cmd
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	return cmd
}
