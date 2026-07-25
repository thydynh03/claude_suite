package cli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"claude_suite/backend/models"
)

type AntiAccountKey struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"` // "api_key" or "oauth_token"
	APIKey     string `json:"api_key"`
	OAuthToken string `json:"oauth_token"`
	Status     string `json:"status"` // "active", "rate_limited_429"
}

type AccountKeyPool struct {
	mu      sync.Mutex
	keys    []AntiAccountKey
	current int
}

var GlobalAntiPool = &AccountKeyPool{
	keys: []AntiAccountKey{
		{ID: "key-1", Name: "Default Key (Environment)", Type: "api_key", APIKey: "", Status: "active"},
	},
}

func (p *AccountKeyPool) GetKeys() []AntiAccountKey {
	p.mu.Lock()
	defer p.mu.Unlock()
	keysCopy := make([]AntiAccountKey, len(p.keys))
	copy(keysCopy, p.keys)
	return keysCopy
}

func (p *AccountKeyPool) AddKey(name, apiKey string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.keys = append(p.keys, AntiAccountKey{
		ID:     fmt.Sprintf("key-%d", len(p.keys)+1),
		Name:   name,
		Type:   "api_key",
		APIKey: apiKey,
		Status: "active",
	})
}

func (p *AccountKeyPool) AddOAuthKey(name, oauthToken string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.keys = append(p.keys, AntiAccountKey{
		ID:         fmt.Sprintf("key-%d", len(p.keys)+1),
		Name:       name,
		Type:       "oauth_token",
		OAuthToken: oauthToken,
		Status:     "active",
	})
}

func (p *AccountKeyPool) GetCurrentAccount() *AntiAccountKey {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.keys) == 0 {
		return nil
	}
	acc := p.keys[p.current%len(p.keys)]
	return &acc
}

func (p *AccountKeyPool) GetCurrentKey() string {
	acc := p.GetCurrentAccount()
	if acc == nil {
		return ""
	}
	if acc.Type == "oauth_token" {
		return acc.OAuthToken
	}
	return acc.APIKey
}

func (p *AccountKeyPool) RotateNextKey() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.keys) <= 1 {
		return ""
	}
	p.keys[p.current%len(p.keys)].Status = "rate_limited_429"
	p.current = (p.current + 1) % len(p.keys)
	nextKey := &p.keys[p.current]
	nextKey.Status = "active"
	return nextKey.Name
}

type AntigravityCLI struct {
	executablePath string
}

func NewAntigravityCLI() *AntigravityCLI {
	return &AntigravityCLI{
		executablePath: ResolveAntigravityCLI(),
	}
}

func (a *AntigravityCLI) RunAgent(agent *models.Agent, prompt string, onLog LogCallback, cwd string) *RunResult {
	return a.executeWithRotation(context.Background(), agent.Model, prompt, agent.System, agent.SessionID, onLog, cwd, 0)
}

func (a *AntigravityCLI) RunAgentCtx(ctx context.Context, agent *models.Agent, prompt string, onLog LogCallback, cwd string) *RunResult {
	return a.executeWithRotation(ctx, agent.Model, prompt, agent.System, agent.SessionID, onLog, cwd, 0)
}

func (a *AntigravityCLI) RunOnce(prompt string, model string, system string, onLog LogCallback, cwd string) *RunResult {
	return a.executeWithRotation(context.Background(), model, prompt, system, "", onLog, cwd, 0)
}

func (a *AntigravityCLI) RunSession(prompt string, model string, system string, sessionID string, onLog LogCallback, cwd string) *RunResult {
	return a.executeWithRotation(context.Background(), model, prompt, system, sessionID, onLog, cwd, 0)
}

func (a *AntigravityCLI) executeWithRotation(ctx context.Context, model, prompt, system, sessionID string, onLog LogCallback, cwd string, attempt int) *RunResult {
	res := a.execute(ctx, model, prompt, system, sessionID, onLog, cwd)

	// Detect 429 Rate Limit / Quota Exhaustion for Auto-Rotation
	if !res.Success && attempt < 3 {
		errLower := strings.ToLower(res.Error)
		if strings.Contains(errLower, "429") || strings.Contains(errLower, "quota") || strings.Contains(errLower, "limit") || strings.Contains(errLower, "exhausted") {
			nextKeyName := GlobalAntiPool.RotateNextKey()
			if nextKeyName != "" {
				if onLog != nil {
					onLog(fmt.Sprintf("⚠️ Anti CLI gặp lỗi 429 Rate Limit / Quota. Tự động xoay vòng sang %s và thử lại ngay lập tức...", nextKeyName), "WARN")
				}
				time.Sleep(1 * time.Second)
				return a.executeWithRotation(ctx, model, prompt, system, sessionID, onLog, cwd, attempt+1)
			}
		}
	}

	return res
}

func (a *AntigravityCLI) execute(parent context.Context, model, prompt, system string, sessionID string, onLog LogCallback, cwd string) *RunResult {
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

	ctx, cancel := context.WithTimeout(parent, TaskTimeout())
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

	env := append(os.Environ(),
		"CI=true",
		"NONINTERACTIVE=1",
		"ANTIGRAVITY_SKIP_PERMISSIONS=1",
		"AGY_TRUST_ALL=1",
		"CLAUDE_SKIP_PERMISSIONS=1",
	)

	acc := GlobalAntiPool.GetCurrentAccount()
	if acc != nil {
		if acc.Type == "oauth_token" && acc.OAuthToken != "" {
			env = append(env,
				"ANTIGRAVITY_OAUTH_TOKEN="+acc.OAuthToken,
				"ANTIGRAVITY_AUTH_TYPE=oauth",
				"GEMINI_OAUTH_TOKEN="+acc.OAuthToken,
			)
		} else if acc.APIKey != "" {
			env = append(env,
				"GEMINI_API_KEY="+acc.APIKey,
				"ANTIGRAVITY_API_KEY="+acc.APIKey,
				"ANTIGRAVITY_AUTH_TYPE=api_key",
			)
		}
	}
	cmd.Env = env

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
