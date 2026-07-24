package cli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"claude_suite/backend/models"
)

type AntigravityCLI struct {
	executablePath string
}

func NewAntigravityCLI() *AntigravityCLI {
	return &AntigravityCLI{
		executablePath: ResolveAntigravityCLI(),
	}
}

func (a *AntigravityCLI) RunAgent(agent *models.Agent, prompt string, onLog LogCallback, cwd string) *RunResult {
	return a.execute(agent.Model, prompt, agent.System, agent.SessionID, onLog, cwd)
}

func (a *AntigravityCLI) RunOnce(prompt string, model string, system string, onLog LogCallback, cwd string) *RunResult {
	return a.execute(model, prompt, system, "", onLog, cwd)
}

func (a *AntigravityCLI) RunSession(prompt string, model string, system string, sessionID string, onLog LogCallback, cwd string) *RunResult {
	return a.execute(model, prompt, system, sessionID, onLog, cwd)
}

func (a *AntigravityCLI) execute(model, prompt, system string, sessionID string, onLog LogCallback, cwd string) *RunResult {
	startTime := time.Now()

	args := []string{"--print", prompt, "--dangerously-skip-permissions"}
	if model != "" {
		args = append(args, "--model", model)
	}
	if sessionID != "" {
		if sessionID == "continue" || sessionID == "latest" {
			args = append(args, "--continue")
		} else {
			args = append(args, "--conversation", sessionID)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	var cmd *exec.Cmd
	if ShowCLIConsole {
		cmdArgs := append([]string{"/k", a.executablePath}, args...)
		cmd = exec.CommandContext(ctx, "cmd.exe", cmdArgs...)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    false,
			CreationFlags: 0x00000010, // CREATE_NEW_CONSOLE
		}
	} else {
		cmd = exec.CommandContext(ctx, a.executablePath, args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true,
			CreationFlags: 0x08000000, // CREATE_NO_WINDOW
		}
	}
	if cwd != "" && dirExists(cwd) {
		cmd.Dir = cwd
	}
	cmd.Env = append(os.Environ(),
		"CI=true",
		"NONINTERACTIVE=1",
		"ANTIGRAVITY_SKIP_PERMISSIONS=1",
		"AGY_TRUST_ALL=1",
		"CLAUDE_SKIP_PERMISSIONS=1",
	)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return &RunResult{Success: false, Error: fmt.Sprintf("failed to get stdout pipe: %v", err)}
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return &RunResult{Success: false, Error: fmt.Sprintf("failed to get stderr pipe: %v", err)}
	}

	if err := cmd.Start(); err != nil {
		return &RunResult{Success: false, Error: fmt.Sprintf("failed to start antigravity cli: %v", err)}
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(io.TeeReader(stdoutPipe, &stdoutBuf))
		for scanner.Scan() {
			line := scanner.Text()
			if onLog != nil {
				onLog(line, "INFO")
			}
		}
	}()

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(io.TeeReader(stderrPipe, &stderrBuf))
		for scanner.Scan() {
			line := scanner.Text()
			if onLog != nil {
				onLog(line, "WARN")
			}
		}
	}()

	wg.Wait()
	err = cmd.Wait()

	duration := time.Since(startTime).Seconds()
	outStr := stdoutBuf.String()
	errStr := stderrBuf.String()

	if err != nil && outStr == "" {
		return &RunResult{
			Success:     false,
			Output:      outStr,
			Error:       fmt.Sprintf("%v: %s", err, errStr),
			DurationSec: duration,
		}
	}

	tokens := int64(len(outStr)+len(prompt)) / 4

	return &RunResult{
		Success:     true,
		Output:      outStr,
		Error:       errStr,
		TokensUsed:  tokens,
		DurationSec: duration,
	}
}
