//go:build !windows

package sysproc

import (
	"context"
	"os/exec"
)

// Command is exec.Command. Only Windows allocates a console for child
// processes, so there is nothing to suppress here.
func Command(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

// CommandContext is exec.CommandContext.
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}
