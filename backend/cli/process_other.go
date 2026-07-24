//go:build !windows

package cli

import (
	"context"
	"os/exec"
)

func newCLICommand(ctx context.Context, executable string, args []string, _ bool) *exec.Cmd {
	return exec.CommandContext(ctx, executable, args...)
}
