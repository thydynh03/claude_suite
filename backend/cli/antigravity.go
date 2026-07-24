package cli

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"claude_suite/backend/models"
)

type AntigravityCLI struct {
	executablePath string
}

func NewAntigravityCLI() *AntigravityCLI {
	return &AntigravityCLI{
		executablePath: findAntigravityPath(),
	}
}

func findAntigravityPath() string {
	if path, err := exec.LookPath("agy"); err == nil {
		return path
	}
	if path, err := exec.LookPath("antigravity"); err == nil {
		return path
	}
	if home, err := os.UserHomeDir(); err == nil {
		winPath := filepath.Join(home, "AppData", "Local", "agy", "bin", "agy.exe")
		if _, err := os.Stat(winPath); err == nil {
			return winPath
		}
	}
	return "agy"
}

func (a *AntigravityCLI) RunAgent(agent *models.Agent, prompt string, onLog LogCallback, cwd string) *RunResult {
	return a.execute(agent.Model, prompt, agent.System, onLog, cwd)
}

func (a *AntigravityCLI) RunOnce(prompt string, model string, system string, onLog LogCallback, cwd string) *RunResult {
	return a.execute(model, prompt, system, onLog, cwd)
}

func (a *AntigravityCLI) execute(model, prompt, system string, onLog LogCallback, cwd string) *RunResult {
	startTime := time.Now()

	args := []string{"prompt"}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, "-p", prompt)

	cmd := exec.Command(a.executablePath, args...)
	if cwd != "" && dirExists(cwd) {
		cmd.Dir = cwd
	}

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
