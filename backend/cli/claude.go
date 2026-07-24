package cli

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"claude_suite/backend/models"
)

type ClaudeCLI struct {
	executablePath string
	antigravity    *AntigravityCLI
}

func NewClaudeCLI() *ClaudeCLI {
	return &ClaudeCLI{
		executablePath: findClaudePath(),
		antigravity:    NewAntigravityCLI(),
	}
}

func findClaudePath() string {
	// Check standard PATH first
	if path, err := exec.LookPath("claude"); err == nil {
		return path
	}
	// Check Windows user AppData path
	if home, err := os.UserHomeDir(); err == nil {
		winPath := filepath.Join(home, "AppData", "Local", "AnthropicClaude", "claude.exe")
		if _, err := os.Stat(winPath); err == nil {
			return winPath
		}
	}
	return "claude"
}

func (c *ClaudeCLI) RunAgent(agent *models.Agent, prompt string, onLog LogCallback, cwd string) *RunResult {
	// Route Gemini & Antigravity models to AntigravityCLI
	modelLower := strings.ToLower(agent.Model)
	if strings.Contains(modelLower, "gemini") || strings.Contains(modelLower, "thinking") || agent.Provider == "anti_cli" {
		return c.antigravity.RunAgent(agent, prompt, onLog, cwd)
	}

	return c.execute(agent.Model, prompt, agent.System, agent.SessionID, onLog, cwd)
}

func (c *ClaudeCLI) RunOnce(prompt string, model string, system string, onLog LogCallback, cwd string) *RunResult {
	modelLower := strings.ToLower(model)
	if strings.Contains(modelLower, "gemini") || strings.Contains(modelLower, "thinking") {
		return c.antigravity.RunOnce(prompt, model, system, onLog, cwd)
	}
	return c.execute(model, prompt, system, "", onLog, cwd)
}

func (c *ClaudeCLI) execute(model, prompt, system, sessionID string, onLog LogCallback, cwd string) *RunResult {
	startTime := time.Now()

	args := []string{"-p", prompt}
	if model != "" {
		args = append(args, "--model", model)
	}
	if system != "" {
		args = append(args, "--system", system)
	}
	if sessionID != "" {
		args = append(args, "--resume", sessionID)
	}

	cmd := exec.Command(c.executablePath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
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
		return &RunResult{Success: false, Error: fmt.Sprintf("failed to start claude cli: %v", err)}
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)

	// Stream stdout line by line
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

	// Stream stderr line by line
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

	if err != nil {
		return &RunResult{
			Success:     false,
			Output:      outStr,
			Error:       fmt.Sprintf("%v: %s", err, errStr),
			DurationSec: duration,
		}
	}

	// Extract session ID if printed
	extractedSession := extractSessionID(outStr)
	if extractedSession == "" {
		extractedSession = sessionID
	}

	// Estimate tokens
	tokens := int64(len(outStr)+len(prompt)) / 4

	return &RunResult{
		Success:     true,
		Output:      outStr,
		Error:       errStr,
		SessionID:   extractedSession,
		TokensUsed:  tokens,
		DurationSec: duration,
	}
}

func extractSessionID(text string) string {
	re := regexp.MustCompile(`Session ID:\s*([a-f0-9\-]+)`)
	m := re.FindStringSubmatch(text)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
