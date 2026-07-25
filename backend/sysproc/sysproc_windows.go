//go:build windows

// Package sysproc spawns helper processes without flashing a console window.
//
// A GUI build has no console of its own, so every exec.Command that runs a
// console program — git, reg, taskkill, powershell — makes Windows allocate one
// and tear it down again. The user sees a black window blink for a fraction of a
// second, repeatedly, because these run on a timer (git status polling, the
// snapshot before each task).
package sysproc

import (
	"context"
	"os/exec"
	"syscall"
)

// createNoWindow is CREATE_NO_WINDOW: run the child without allocating a console.
const createNoWindow = 0x08000000

func hide(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
	return cmd
}

// Command is exec.Command with the console window suppressed.
func Command(name string, args ...string) *exec.Cmd {
	return hide(exec.Command(name, args...))
}

// CommandContext is exec.CommandContext with the console window suppressed.
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return hide(exec.CommandContext(ctx, name, args...))
}
