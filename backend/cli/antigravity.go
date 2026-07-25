package cli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"claude_suite/backend/models"
)

type ModelQuota struct {
	Name      string `json:"name"`       // e.g. "Gemini 3.1 Pro (High)"
	Category  string `json:"category"`   // "gemini" | "gpt" | "claude"
	ResetTime string `json:"reset_time"` // e.g. "0h 0m", "1d 6h"
	UsagePct  int    `json:"usage_pct"`  // e.g. 53, 100, 8
	Status    string `json:"status"`     // "ok", "warning", "exceeded"
}

type AntiAccountKey struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Email        string       `json:"email"`
	Type         string       `json:"type"` // "api_key" or "oauth_token"
	APIKey       string       `json:"api_key"`
	OAuthToken   string       `json:"oauth_token"`
	RefreshToken string       `json:"refresh_token"`
	Status       string       `json:"status"` // "active", "rate_limited_429", "disabled"
	Tier         string       `json:"tier"`   // "PRO", "ULTRA", "FREE"
	IsCurrent    bool         `json:"is_current"`
	LastUsed     string       `json:"last_used"`
	ModelQuotas  []ModelQuota `json:"model_quotas"`
}

type AccountKeyPool struct {
	mu      sync.Mutex
	keys    []AntiAccountKey
	current int
}

func defaultQuotas(tier string) []ModelQuota {
	if tier == "FREE" {
		return []ModelQuota{
			{Name: "Gemini 3.1 Pro (Low)", Category: "gemini", ResetTime: "6d 2h", UsagePct: 8, Status: "ok"},
			{Name: "Gemini 3.5 Flash (Med)", Category: "gemini", ResetTime: "6d 2h", UsagePct: 8, Status: "ok"},
			{Name: "Gemini 3.6 Flash (High)", Category: "gemini", ResetTime: "6d 2h", UsagePct: 8, Status: "ok"},
			{Name: "Gemini 3.1 Flash Lite", Category: "gemini", ResetTime: "6d 2h", UsagePct: 8, Status: "ok"},
			{Name: "Claude Sonnet 4.6 (Th)", Category: "claude", ResetTime: "6d 2h", UsagePct: 8, Status: "ok"},
			{Name: "GPT-OSS 120B (Medium)", Category: "gpt", ResetTime: "6d 2h", UsagePct: 8, Status: "ok"},
		}
	}
	return []ModelQuota{
		{Name: "Gemini 3.1 Pro (High)", Category: "gemini", ResetTime: "0h 0m", UsagePct: 53, Status: "ok"},
		{Name: "Gemini 3.5 Flash (High)", Category: "gemini", ResetTime: "0h 0m", UsagePct: 53, Status: "ok"},
		{Name: "Gemini 3.6 Flash (High)", Category: "gemini", ResetTime: "0h 0m", UsagePct: 53, Status: "ok"},
		{Name: "Gemini 3.1 Flash Lite", Category: "gemini", ResetTime: "0h 0m", UsagePct: 53, Status: "ok"},
		{Name: "Claude Sonnet 4.6 (Th)", Category: "claude", ResetTime: "1d 6h", UsagePct: 8, Status: "ok"},
		{Name: "GPT-OSS 120B (Medium)", Category: "gpt", ResetTime: "1d 6h", UsagePct: 0, Status: "ok"},
	}
}

var GlobalAntiPool = &AccountKeyPool{
	keys: []AntiAccountKey{},
}

func (p *AccountKeyPool) SetKeys(keys []AntiAccountKey) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.keys = keys
	p.current = 0
}

func (p *AccountKeyPool) GetKeys() []AntiAccountKey {
	p.mu.Lock()
	defer p.mu.Unlock()
	keysCopy := make([]AntiAccountKey, len(p.keys))
	for i, k := range p.keys {
		k.IsCurrent = (i == p.current%len(p.keys))
		keysCopy[i] = k
	}
	return keysCopy
}

func (p *AccountKeyPool) AddKey(name, apiKey string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	email := name
	if !strings.Contains(email, "@") {
		email = strings.ToLower(strings.ReplaceAll(name, " ", ".")) + "@gmail.com"
	}
	p.keys = append(p.keys, AntiAccountKey{
		ID:          fmt.Sprintf("key-%d", len(p.keys)+1),
		Name:        name,
		Email:       email,
		Type:        "api_key",
		APIKey:      apiKey,
		Status:      "active",
		Tier:        "PRO",
		LastUsed:    time.Now().Format("1/2/2006 03:04 PM"),
		ModelQuotas: defaultQuotas("PRO"),
	})
}

func (p *AccountKeyPool) AddOAuthKey(name, oauthToken string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	email := name
	if strings.HasPrefix(email, "Google (") && strings.HasSuffix(email, ")") {
		email = email[8 : len(email)-1]
	}
	if !strings.Contains(email, "@") {
		email = strings.ToLower(strings.ReplaceAll(name, " ", ".")) + "@gmail.com"
	}
	p.keys = append(p.keys, AntiAccountKey{
		ID:          fmt.Sprintf("key-%d", len(p.keys)+1),
		Name:        name,
		Email:       email,
		Type:        "oauth_token",
		OAuthToken:  oauthToken,
		Status:      "active",
		Tier:        "PRO",
		LastUsed:    time.Now().Format("1/2/2006 03:04 PM"),
		ModelQuotas: defaultQuotas("PRO"),
	})
}

func (p *AccountKeyPool) DeleteKey(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	var newKeys []AntiAccountKey
	found := false
	for _, k := range p.keys {
		if k.ID == id {
			found = true
			continue
		}
		newKeys = append(newKeys, k)
	}
	if found {
		p.keys = newKeys
		if p.current >= len(p.keys) && len(p.keys) > 0 {
			p.current = 0
		}
	}
	return found
}

func (p *AccountKeyPool) SetCurrentKey(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, k := range p.keys {
		if k.ID == id {
			p.current = i
			return true
		}
	}
	return false
}

func (p *AccountKeyPool) ToggleKeyStatus(id string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, k := range p.keys {
		if k.ID == id {
			if k.Status == "active" {
				p.keys[i].Status = "disabled"
			} else {
				p.keys[i].Status = "active"
			}
			return p.keys[i].Status
		}
	}
	return ""
}

func (p *AccountKeyPool) WarmupKeys() {
	p.mu.Lock()
	defer p.mu.Unlock()
	nowStr := time.Now().Format("1/2/2006 03:04 PM")
	for i := range p.keys {
		p.keys[i].Status = "active"
		p.keys[i].LastUsed = nowStr
	}
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

	cmd := newCLICommand(ctx, a.executablePath, args, ShowCLIConsole)
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
				onLog(line, classifyAntiLogLine(line))
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

	// Prefer REAL usage/cost if the CLI reported it; fall back to a length estimate.
	tokens, cost := parseAntiUsage(outStr)
	if tokens == 0 {
		tokens = int64(len(outStr)+len(prompt)) / 4
	}

	return &RunResult{
		Success:     true,
		Output:      outStr,
		Error:       errStr,
		TokensUsed:  tokens,
		CostUSD:     cost,
		DurationSec: duration,
	}
}

// antiToolLineRe matches common tool-call announcement patterns emitted by CLI
// coding agents (Antigravity/Gemini CLI has no documented structured stream
// format, unlike Claude CLI's --output-format stream-json, so this is a
// best-effort text heuristic rather than a real event parser).
var antiToolLineRe = regexp.MustCompile(`(?i)^\s*(?:🔧|⚙️|\$)?\s*(running|executing|reading file|writing file|editing file|creating file|deleting file|calling tool|tool call|invoking|shell command|exec)\b`)

// classifyAntiLogLine tags a raw Antigravity/Gemini CLI stdout line as "TOOL"
// when it looks like a tool invocation, else "INFO".
func classifyAntiLogLine(line string) string {
	if antiToolLineRe.MatchString(line) {
		return "TOOL"
	}
	return "INFO"
}

var (
	antiInTokRe  = regexp.MustCompile(`"input_tokens"\s*:\s*(\d+)`)
	antiOutTokRe = regexp.MustCompile(`"output_tokens"\s*:\s*(\d+)`)
	antiTotTokRe = regexp.MustCompile(`(?i)total[_ ]tokens?\s*[:=]\s*(\d+)`)
	antiCostRe   = regexp.MustCompile(`"total_cost_usd"\s*:\s*([0-9.]+)`)
)

// parseAntiUsage best-effort extracts token usage and cost from Antigravity/Gemini
// CLI output (JSON usage block or a "total_tokens: N" line). Returns (0,0) if none
// found so the caller can fall back to an estimate.
func parseAntiUsage(out string) (int64, float64) {
	var tokens int64
	if m := antiInTokRe.FindStringSubmatch(out); m != nil {
		tokens += atoi64(m[1])
	}
	if m := antiOutTokRe.FindStringSubmatch(out); m != nil {
		tokens += atoi64(m[1])
	}
	if tokens == 0 {
		if m := antiTotTokRe.FindStringSubmatch(out); m != nil {
			tokens = atoi64(m[1])
		}
	}
	var cost float64
	if m := antiCostRe.FindStringSubmatch(out); m != nil {
		fmt.Sscanf(m[1], "%f", &cost)
	}
	return tokens, cost
}

func atoi64(s string) int64 {
	var n int64
	fmt.Sscanf(s, "%d", &n)
	return n
}
