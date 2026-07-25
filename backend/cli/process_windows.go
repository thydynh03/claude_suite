//go:build windows

package cli

import (
	"context"
	"os/exec"
	"syscall"
)

func newCLICommand(ctx context.Context, executable string, args []string, show bool) *exec.Cmd {
	if show {
		cmdArgs := append([]string{"/k", executable}, args...)
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
