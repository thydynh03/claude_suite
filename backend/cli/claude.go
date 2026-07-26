package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"claude_suite/backend/models"
	"claude_suite/backend/textutil"

	"claude_suite/backend/provider"
)

var ShowCLIConsole bool = false

// claudeMissingMessage says what is actually wrong. The raw exec error
// ("executable file not found in %PATH%") sent users hunting for a PATH
// problem when the CLI simply was not installed.
const claudeMissingMessage = "chưa tìm thấy Claude Code CLI trên máy này. " +
	"Cài bằng `npm install -g @anthropic-ai/claude-code` (hoặc bản cài đặt gốc), " +
	"rồi chạy lại task — không cần khởi động lại app."

type ClaudeCLI struct {
	executablePath string
	antigravity    *AntigravityCLI
}

func NewClaudeCLI() *ClaudeCLI {
	return &ClaudeCLI{
		executablePath: ResolveClaudeCLI(),
		antigravity:    NewAntigravityCLI(),
	}
}

func (c *ClaudeCLI) RunAgent(agent *models.Agent, prompt string, onLog LogCallback, cwd string) *RunResult {
	return c.RunAgentCtx(context.Background(), agent, prompt, onLog, cwd)
}

func (c *ClaudeCLI) RunAgentCtx(ctx context.Context, agent *models.Agent, prompt string, onLog LogCallback, cwd string) *RunResult {
	// Route Gemini & Antigravity models to AntigravityCLI. The agent's declared
	// provider wins when it has one; otherwise the model name decides.
	if provider.IsAnti(provider.ResolveProvider(agent.Provider, agent.Model)) {
		return c.antigravity.RunAgentCtx(ctx, agent, prompt, onLog, cwd)
	}

	return c.executeCtx(ctx, agent.Model, prompt, agent.System, agent.SessionID, onLog, cwd)
}

func (c *ClaudeCLI) RunOnce(prompt string, model string, system string, onLog LogCallback, cwd string) *RunResult {
	if provider.IsAnti(provider.ResolveProvider("", model)) {
		return c.antigravity.RunOnce(prompt, model, system, onLog, cwd)
	}
	return c.execute(model, prompt, system, "", onLog, cwd)
}

func (c *ClaudeCLI) RunSession(prompt string, model string, system string, sessionID string, onLog LogCallback, cwd string) *RunResult {
	if provider.IsAnti(provider.ResolveProvider("", model)) {
		return c.antigravity.RunSession(prompt, model, system, sessionID, onLog, cwd)
	}
	return c.execute(model, prompt, system, sessionID, onLog, cwd)
}

func (c *ClaudeCLI) execute(model, prompt, system, sessionID string, onLog LogCallback, cwd string) *RunResult {
	return c.executeCtx(context.Background(), model, prompt, system, sessionID, onLog, cwd)
}

func (c *ClaudeCLI) executeCtx(parent context.Context, model, prompt, system, sessionID string, onLog LogCallback, cwd string) *RunResult {
	startTime := time.Now()

	// The "show console" toggle opens a detached viewer window tailing this
	// run's log — the agent process itself always runs hidden (viewer.go).
	viewer := startRunViewer("Claude Agent · model: " + orDefault(model, "(mặc định CLI)"))
	onLog = viewer.logSink(onLog)
	defer viewer.Finish("RUN KẾT THÚC")

	fullPrompt := inlineSystemPrompt(system, prompt)

	// stream-json emits newline-delimited JSON events so we can surface tool calls
	// and read REAL token usage / cost from the final result event.
	args := []string{"-p", fullPrompt, "--output-format", "stream-json", "--verbose", "--dangerously-skip-permissions"}
	if model != "" {
		args = append(args, "--model", model)
	}
	if sessionID != "" {
		args = append(args, "--resume", sessionID)
	}

	ctx, cancel := context.WithTimeout(parent, TaskTimeout())
	defer cancel()

	// Re-resolve when the cached answer is the bare fallback name. The path is
	// resolved once at app start, so the natural repair loop — task fails, user
	// installs the CLI, user retries — kept failing: a running process never
	// sees the PATH an installer just extended.
	exePath := c.executablePath
	if !CLIInstalled(exePath) {
		if fresh := ResolveClaudeCLI(); CLIInstalled(fresh) {
			exePath = fresh
			c.executablePath = fresh
		} else {
			return &RunResult{Success: false, Error: claudeMissingMessage}
		}
	}

	cmd := newCLICommand(ctx, exePath, args)
	if cwd != "" {
		if dirExists(cwd) {
			cmd.Dir = cwd
		} else {
			// Running in the app's own working directory instead is not a
			// harmless fallback: the sub-agent runs with
			// --dangerously-skip-permissions, so for a portable exe it would
			// start writing files into the user's Downloads folder, outside
			// any git snapshot.
			return &RunResult{
				Success: false,
				Error:   fmt.Sprintf("workspace không tồn tại: %s — hãy chọn lại thư mục dự án trước khi chạy agent", cwd),
			}
		}
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
		return &RunResult{Success: false, Error: fmt.Sprintf("failed to start claude cli: %v", err)}
	}

	var stderrBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)

	parsed := &streamParse{}

	// Parse stdout as newline-delimited JSON events (stream-json).
	go func() {
		defer wg.Done()
		if err := forEachLine(stdoutPipe, func(line string) {
			line = strings.TrimSpace(line)
			if line == "" {
				return
			}
			parseStreamEvent(line, parsed, onLog)
		}); err != nil && onLog != nil {
			onLog(fmt.Sprintf("stdout stream error: %v", err), "WARN")
		}
	}()

	// Stream stderr line by line
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
	errStr := stderrBuf.String()

	outStr := parsed.result
	if outStr == "" {
		outStr = parsed.assistantText.String()
	}

	// A timeout kill is a failure no matter what was streamed first: the
	// process died mid-edit. The partial-success fallthrough below used to
	// catch it — any run that produced one text block before the deadline
	// came back Success:true, and the orchestrator marked the half-finished
	// task done. The parent cannot tell either: the deadline fires on OUR
	// context, so the orchestrator's own ctx.Err() stays nil.
	if ctx.Err() == context.DeadlineExceeded {
		return &RunResult{
			Success:     false,
			Output:      outStr,
			Error:       fmt.Sprintf("run bị dừng vì quá TaskTimeout — công việc dở dang, output một phần được giữ lại. %s", errStr),
			SessionID:   parsed.sessionID,
			TokensUsed:  parsed.totalTokens(),
			CostUSD:     parsed.costUSD,
			DurationSec: duration,
			ModelUsed:   parsed.modelUsed,
		}
	}

	if err != nil {
		if outStr == "" {
			return &RunResult{
				Success:     false,
				Output:      outStr,
				Error:       fmt.Sprintf("%v: %s", err, errStr),
				DurationSec: duration,
			}
		}
		// Non-zero exit but we still captured a result — surface as partial success.
	}
	if parsed.isError {
		return &RunResult{
			Success:     false,
			Output:      outStr,
			Error:       strings.TrimSpace(outStr + " " + errStr),
			SessionID:   parsed.sessionID,
			TokensUsed:  parsed.totalTokens(),
			CostUSD:     parsed.costUSD,
			DurationSec: duration,
			ModelUsed:   parsed.modelUsed,
		}
	}

	extractedSession := parsed.sessionID
	if extractedSession == "" {
		extractedSession = sessionID
	}

	tokens := parsed.totalTokens()
	estimated := false
	if tokens == 0 {
		tokens = int64(len(outStr)+len(prompt)) / 4 // fallback estimate
		estimated = true
	}

	return &RunResult{
		Success:        true,
		Output:         outStr,
		Error:          errStr,
		SessionID:      extractedSession,
		TokensUsed:     tokens,
		CostUSD:        parsed.costUSD,
		DurationSec:    duration,
		UsageEstimated: estimated,
		ModelUsed:      parsed.modelUsed,
	}
}

// streamParse accumulates state while reading Claude CLI stream-json events.
type streamParse struct {
	assistantText strings.Builder
	result        string
	sessionID     string
	modelUsed     string
	inputTokens   int64
	outputTokens  int64
	costUSD       float64
	isError       bool
}

func (s *streamParse) totalTokens() int64 { return s.inputTokens + s.outputTokens }

// parseStreamEvent decodes one NDJSON line and streams meaningful pieces (text,
// tool calls) to onLog while recording usage/session for the final result.
func parseStreamEvent(line string, s *streamParse, onLog LogCallback) {
	var ev map[string]interface{}
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		// Not JSON — emit raw so nothing is silently swallowed.
		if onLog != nil {
			onLog(line, "INFO")
		}
		return
	}

	if sid, ok := ev["session_id"].(string); ok && sid != "" {
		s.sessionID = sid
	}
	// The CLI reports the model it ACTUALLY runs — in the system:init event
	// and on every assistant message. This is what lets the app answer "which
	// model is this agent using" with a measured fact instead of the
	// configured wish (the CLI can resolve aliases or fall back on its own).
	if mdl, ok := ev["model"].(string); ok && mdl != "" {
		s.modelUsed = mdl
	}

	switch ev["type"] {
	case "assistant":
		msg, _ := ev["message"].(map[string]interface{})
		if mdl, ok := msg["model"].(string); ok && mdl != "" {
			s.modelUsed = mdl
		}
		content, _ := msg["content"].([]interface{})
		for _, c := range content {
			block, _ := c.(map[string]interface{})
			switch block["type"] {
			case "text":
				if txt, ok := block["text"].(string); ok && strings.TrimSpace(txt) != "" {
					s.assistantText.WriteString(txt)
					s.assistantText.WriteString("\n")
					if onLog != nil {
						onLog(txt, "INFO")
					}
				}
			case "tool_use":
				name, _ := block["name"].(string)
				if onLog != nil {
					onLog(fmt.Sprintf("🔧 Tool: %s %s", name, toolInputSummary(block["input"])), "TOOL")
				}
			}
		}
	case "result":
		if r, ok := ev["result"].(string); ok {
			s.result = r
		}
		if st, ok := ev["subtype"].(string); ok && st != "success" {
			s.isError = true
		}
		if isErr, ok := ev["is_error"].(bool); ok && isErr {
			s.isError = true
		}
		if cost, ok := ev["total_cost_usd"].(float64); ok {
			s.costUSD = cost
		}
		if usage, ok := ev["usage"].(map[string]interface{}); ok {
			if v, ok := usage["input_tokens"].(float64); ok {
				s.inputTokens += int64(v)
			}
			if v, ok := usage["output_tokens"].(float64); ok {
				s.outputTokens += int64(v)
			}
		}
	}
}

// toolInputSummary produces a short, safe one-line summary of a tool_use input.
func toolInputSummary(input interface{}) string {
	m, ok := input.(map[string]interface{})
	if !ok {
		return ""
	}
	for _, key := range []string{"file_path", "path", "command", "pattern", "url", "query"} {
		if v, ok := m[key].(string); ok && v != "" {
			return "→ " + textutil.Truncate(v, 120, "…")
		}
	}
	return ""
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
