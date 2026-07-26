package cli

import (
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
	ResetTime string `json:"reset_time"` // e.g. "0h 0m", "1d 6h"; empty when unknown
	UsagePct  int    `json:"usage_pct"`  // -1 when unknown; never guessed
	Status    string `json:"status"`     // "ok", "warning", "exceeded", "unknown"
}

// UsageStats is what this app can honestly say about an account, because it
// watched it happen: how many requests were sent through it and when it was last
// turned away for exceeding a limit.
//
// It replaces the invented percentages that used to fill the quota table. Those
// were compile-time constants — the same 53% and "0h 0m" for every account, on a
// screen headed "Hạn Mức Model" — so the UI looked informative while nothing had
// ever asked Google anything. A wrong number is worse than a blank: a blank
// prompts a question, a number ends it.
type UsageStats struct {
	Requests        int       `json:"requests"`
	RateLimitHits   int       `json:"rate_limit_hits"`
	LastRateLimitAt time.Time `json:"last_rate_limit_at"`
	LastRequestAt   time.Time `json:"last_request_at"`
}

type AntiAccountKey struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Type         string `json:"type"` // "api_key" or "oauth_token"
	APIKey       string `json:"api_key"`
	OAuthToken   string `json:"oauth_token"`
	RefreshToken string `json:"refresh_token"`
	// TokenExpiresAt is when OAuthToken stops working. An access token lasts about
	// an hour, so without this the pool rotates onto accounts that are already dead.
	TokenExpiresAt time.Time    `json:"token_expires_at"`
	Status         string       `json:"status"` // "active", "rate_limited_429", "disabled"
	Tier           string       `json:"tier"`   // "PRO", "ULTRA", "FREE"
	IsCurrent      bool         `json:"is_current"`
	LastUsed       string       `json:"last_used"`
	ModelQuotas    []ModelQuota `json:"model_quotas"`
	// Usage is measured, not guessed. ModelQuotas stays for the day a real quota
	// endpoint is wired up; until then its entries carry status "unknown".
	Usage UsageStats `json:"usage"`
	// TierSource records where Tier came from: "user" when somebody labelled the
	// account in the UI, "unknown" otherwise. Every account used to be born "PRO"
	// with no evidence, which is why the PRO filter never matched anything real.
	TierSource string `json:"tier_source"`
}

// tierUnknown is the honest starting state for an account nobody has labelled.
const tierUnknown = "UNKNOWN"

type AccountKeyPool struct {
	mu      sync.Mutex
	keys    []AntiAccountKey
	current int
}

// unknownQuotas lists the models an account can be asked for, with no claim about
// how much of each is left.
//
// It replaces defaultQuotas, which returned hardcoded percentages and reset times
// — identical for every account, and never once fetched from anywhere. UsagePct
// is -1 rather than 0 so the UI can tell "no data" apart from "nothing used".
func unknownQuotas() []ModelQuota {
	names := []struct{ name, category string }{
		{"Gemini 3.1 Pro", "gemini"},
		{"Gemini 3.5 Flash", "gemini"},
		{"Gemini 3.6 Flash", "gemini"},
		{"Gemini 3.1 Flash Lite", "gemini"},
		{"Claude Sonnet 4.6 (Thinking)", "claude"},
		{"GPT-OSS 120B", "gpt"},
	}

	out := make([]ModelQuota, 0, len(names))
	for _, n := range names {
		out = append(out, ModelQuota{Name: n.name, Category: n.category, UsagePct: -1, Status: "unknown"})
	}
	return out
}

var GlobalAntiPool = &AccountKeyPool{
	keys: []AntiAccountKey{},
}

func (p *AccountKeyPool) SetKeys(keys []AntiAccountKey) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.keys = keys
	ensureKeyIDs(p.keys)
	p.current = 0
}

// ensureKeyIDs gives every account an id.
//
// Accounts saved before the field existed load back with an empty one, and the
// UI lists them with a keyed loop: eleven accounts sharing the empty key made
// the whole table render as "no accounts" while the counter above it still said
// eleven. Filling them in on load fixes the stored file on the next save too.
func ensureKeyIDs(keys []AntiAccountKey) {
	seen := map[string]bool{}
	for i := range keys {
		id := strings.TrimSpace(keys[i].ID)
		if id != "" && !seen[id] {
			seen[id] = true
			continue
		}
		// Derive from the email so the id survives reordering; fall back to the
		// position when there is nothing else to go on.
		base := strings.TrimSpace(strings.ToLower(keys[i].Email))
		if base == "" {
			base = strings.TrimSpace(strings.ToLower(keys[i].Name))
		}
		if base == "" {
			base = fmt.Sprintf("key-%d", i+1)
		}
		candidate := base
		for n := 2; seen[candidate]; n++ {
			candidate = fmt.Sprintf("%s-%d", base, n)
		}
		keys[i].ID = candidate
		seen[candidate] = true
	}
}

func (p *AccountKeyPool) GetKeys() []AntiAccountKey {
	p.mu.Lock()
	defer p.mu.Unlock()

	// An empty pool is the state of every fresh install, and %0 panics. A panic
	// inside a bound method never returns to the frontend, so the promise behind
	// it hangs and the page sits there spinning forever.
	if len(p.keys) == 0 {
		return []AntiAccountKey{}
	}

	current := p.current % len(p.keys)
	keysCopy := make([]AntiAccountKey, len(p.keys))
	for i, k := range p.keys {
		k.IsCurrent = i == current
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
		Tier:        tierUnknown,
		TierSource:  "unknown",
		LastUsed:    time.Now().Format("1/2/2006 03:04 PM"),
		ModelQuotas: unknownQuotas(),
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
		Tier:        tierUnknown,
		TierSource:  "unknown",
		LastUsed:    time.Now().Format("1/2/2006 03:04 PM"),
		ModelQuotas: unknownQuotas(),
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
		// Warmup clears rate-limit marks; it does not undo the accounts the user
		// switched off.
		if p.keys[i].Status == statusDisabled {
			continue
		}
		p.keys[i].Status = "active"
		p.keys[i].LastUsed = nowStr
	}
}

// statusDisabled marks an account the user switched off in the pool dashboard.
// It means "do not send requests as this account", so key selection has to
// honour it — otherwise the toggle is decorative.
const statusDisabled = "disabled"

// nextUsableIndex returns the first index at or after start, wrapping once, whose
// account the user has not disabled. It returns -1 when every account is off.
// Callers must hold the lock.
func (p *AccountKeyPool) nextUsableIndex(start int) int {
	if len(p.keys) == 0 {
		return -1
	}
	for offset := 0; offset < len(p.keys); offset++ {
		i := (start + offset) % len(p.keys)
		if p.keys[i].Status != statusDisabled {
			return i
		}
	}
	return -1
}

func (p *AccountKeyPool) GetCurrentAccount() *AntiAccountKey {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.keys) == 0 {
		return nil
	}
	// Skip past accounts the user disabled rather than sending requests as one.
	// With every account off, the caller falls back to the environment.
	i := p.nextUsableIndex(p.current % len(p.keys))
	if i < 0 {
		return nil
	}
	p.current = i
	// Every caller of GetCurrentAccount is about to send a request as this
	// account, so this is the honest place to count one.
	p.keys[i].Usage.Requests++
	p.keys[i].Usage.LastRequestAt = time.Now()
	acc := p.keys[i]
	return &acc
}

// SetTier records a tier the user vouched for, and says so via TierSource.
// Nothing else may write Tier: guessing it is what made the PRO filter meaningless.
func (p *AccountKeyPool) SetTier(id, tier string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch tier {
	case "FREE", "PRO", "ULTRA", tierUnknown:
	default:
		return false
	}

	for i := range p.keys {
		if p.keys[i].ID == id {
			p.keys[i].Tier = tier
			p.keys[i].TierSource = "user"
			if tier == tierUnknown {
				p.keys[i].TierSource = "unknown"
			}
			return true
		}
	}
	return false
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

// RotateNextKey marks the account that hit the limit and moves the pool to
// the next usable one.
//
// fromAccountID is the account the failing run actually used. Tasks run in
// parallel and share this pool, so by the time one failure is handled another
// may have rotated already — punishing p.current positionally then marked a
// healthy account rate-limited and, a rotation later, woke the genuinely
// exhausted one back up. When the pool has already moved off fromAccountID,
// this is a no-op that reports the account currently in use. An empty
// fromAccountID keeps the old positional behaviour.
func (p *AccountKeyPool) RotateNextKey(fromAccountID string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.keys) == 0 {
		return ""
	}
	if len(p.keys) == 1 {
		// Nowhere to rotate, but the 429 was still MEASURED — record it, or
		// a single-account pool never carries a rate-limit timestamp and the
		// quota gate (QuotaReady) can never hold a job for it.
		if fromAccountID == "" || p.keys[0].ID == fromAccountID {
			p.keys[0].Status = "rate_limited_429"
			p.keys[0].Usage.RateLimitHits++
			p.keys[0].Usage.LastRateLimitAt = time.Now()
		}
		return ""
	}

	current := p.current % len(p.keys)
	if fromAccountID != "" && p.keys[current].ID != fromAccountID {
		return p.keys[current].Name
	}
	p.keys[current].Status = "rate_limited_429"
	// Counted, because it is one of the few things about an account's limits this
	// app observes first-hand. The quota table shows it in place of the invented
	// percentages it used to display.
	p.keys[current].Usage.RateLimitHits++
	p.keys[current].Usage.LastRateLimitAt = time.Now()

	// Move to the next account the user has not disabled. Rotating into an
	// account is a decision to try it again, so a stale rate-limit mark is
	// cleared — but a disabled one is never woken up, because switching it off
	// was deliberate.
	next := p.nextUsableIndex((current + 1) % len(p.keys))
	if next < 0 || next == current {
		return ""
	}

	p.current = next
	p.keys[next].Status = "active"
	return p.keys[next].Name
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
		if isRotatableAuthError(errLower) {
			nextKeyName := GlobalAntiPool.RotateNextKey(res.AccountID)
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

	// Same viewer contract as the Claude runner: the agent process is hidden,
	// the visible window is a detached tail that outlives the run (viewer.go).
	viewer := startRunViewer("Antigravity/Gemini · model: " + orDefault(model, "(mặc định CLI)"))
	onLog = viewer.logSink(onLog)
	defer viewer.Finish("RUN KẾT THÚC")

	// The Antigravity CLI has no system-prompt flag, so the system prompt is
	// inlined into the prompt text — the same convention as the Claude runner.
	// It used to be accepted and silently dropped, which meant every role
	// definition the orchestrator injects never reached a Gemini agent.
	fullPrompt := inlineSystemPrompt(system, prompt)

	args := []string{"--print", fullPrompt, "--dangerously-skip-permissions"}
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

	cmd := newCLICommand(ctx, a.executablePath, args)
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
	// Remembered so the failure path can rotate the account this run actually
	// used, not whichever one the pool points at by then.
	usedAccountID := ""
	if acc != nil {
		usedAccountID = acc.ID
	}
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
		if err := forEachLine(io.TeeReader(stdoutPipe, &stdoutBuf), func(line string) {
			if onLog != nil {
				onLog(line, classifyAntiLogLine(line))
			}
		}); err != nil && onLog != nil {
			onLog(fmt.Sprintf("stdout stream error: %v", err), "WARN")
		}
	}()

	go func() {
		defer wg.Done()
		if err := forEachLine(io.TeeReader(stderrPipe, &stderrBuf), func(line string) {
			if onLog != nil {
				onLog(line, "WARN")
			}
		}); err != nil && onLog != nil {
			onLog(fmt.Sprintf("stderr stream error: %v", err), "WARN")
		}
	}()

	wg.Wait()
	err = cmd.Wait()

	duration := time.Since(startTime).Seconds()
	outStr := stdoutBuf.String()
	errStr := stderrBuf.String()

	// Same rule as the Claude runner: a deadline kill is a failure even with
	// partial stdout, or the half-done task gets marked done.
	if ctx.Err() == context.DeadlineExceeded {
		return &RunResult{
			Success:     false,
			Output:      outStr,
			Error:       fmt.Sprintf("run bị dừng vì quá TaskTimeout — công việc dở dang, output một phần được giữ lại. %s", errStr),
			DurationSec: duration,
			AccountID:   usedAccountID,
		}
	}

	if err != nil && outStr == "" {
		return &RunResult{
			Success:     false,
			Output:      outStr,
			Error:       fmt.Sprintf("%v: %s", err, errStr),
			DurationSec: duration,
			AccountID:   usedAccountID,
		}
	}

	// Prefer REAL usage/cost if the CLI reported it; fall back to a length estimate.
	tokens, cost := parseAntiUsage(outStr)
	estimated := false
	if tokens == 0 {
		tokens = int64(len(outStr)+len(prompt)) / 4
		estimated = true
	}

	return &RunResult{
		Success:        true,
		Output:         outStr,
		Error:          errStr,
		TokensUsed:     tokens,
		CostUSD:        cost,
		DurationSec:    duration,
		AccountID:      usedAccountID,
		UsageEstimated: estimated,
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

// NeedsRefresh reports whether an account has a refresh token and an access
// token that is missing or about to expire.
func (k AntiAccountKey) NeedsRefresh() bool {
	if k.RefreshToken == "" || k.Status == statusDisabled {
		return false
	}
	return k.OAuthToken == "" || k.TokenExpiresAt.IsZero() || time.Now().After(k.TokenExpiresAt)
}

// AddRefreshAccount adds an account the user imported from their own Google
// sign-ins. It carries no access token yet: one is minted from the refresh token
// when the account is first used, and again whenever it expires.
func (p *AccountKeyPool) AddRefreshAccount(email, refreshToken string) string {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Re-importing the same file should update the token, not pile up duplicates.
	for i := range p.keys {
		if p.keys[i].Email == email {
			p.keys[i].RefreshToken = refreshToken
			p.keys[i].OAuthToken = ""
			p.keys[i].TokenExpiresAt = time.Time{}
			return p.keys[i].ID
		}
	}

	id := fmt.Sprintf("key-%d", len(p.keys)+1)
	p.keys = append(p.keys, AntiAccountKey{
		ID:           id,
		Name:         email,
		Email:        email,
		Type:         "oauth_token",
		RefreshToken: refreshToken,
		Status:       "active",
		Tier:         tierUnknown,
		TierSource:   "unknown",
		LastUsed:     time.Now().Format("1/2/2006 03:04 PM"),
		ModelQuotas:  unknownQuotas(),
	})
	return id
}

// AccountsNeedingRefresh returns the id and refresh token of every account whose
// access token has to be minted again. Returning copies keeps the network call
// out of the lock.
func (p *AccountKeyPool) AccountsNeedingRefresh() map[string]string {
	p.mu.Lock()
	defer p.mu.Unlock()

	pending := make(map[string]string)
	for _, key := range p.keys {
		if key.NeedsRefresh() {
			pending[key.ID] = key.RefreshToken
		}
	}
	return pending
}

// SetAccessToken stores a freshly minted access token against an account.
func (p *AccountKeyPool) SetAccessToken(id, accessToken string, expiresAt time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i := range p.keys {
		if p.keys[i].ID == id {
			p.keys[i].OAuthToken = accessToken
			p.keys[i].TokenExpiresAt = expiresAt
			p.keys[i].Type = "oauth_token"
			return true
		}
	}
	return false
}

// DisableKey switches an account off and records why. Used when Google refuses
// its refresh token: retrying a revoked one every minute helps nobody.
func (p *AccountKeyPool) DisableKey(id, reason string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.keys {
		if p.keys[i].ID == id {
			p.keys[i].Status = statusDisabled
			p.keys[i].OAuthToken = ""
			return true
		}
	}
	return false
}

// rotatableErrRe matches quota/auth failures on word boundaries. Bare
// substring matching here burned accounts on innocent text: "limit" matched
// "context limit exceeded" (another account fares no better against a long
// prompt) and "429" matched any number containing it, each costing an account
// mark plus three paid retries.
var rotatableErrRe = regexp.MustCompile(`(?i)\b(?:429|401|rate.?limit(?:ed)?|quota|too many requests|exhausted|unauthorized|invalid_grant|token expired|daily limit)\b`)

// isRotatableAuthError reports whether a failure is one another account might
// survive: an exhausted quota, or an access token that expired mid-run.
func isRotatableAuthError(errLower string) bool {
	return rotatableErrRe.MatchString(errLower)
}
