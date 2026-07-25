package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"claude_suite/backend/models"
)

var ShowCLIConsole bool = false

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
	// Route Gemini & Antigravity models to AntigravityCLI
	modelLower := strings.ToLower(agent.Model)
	if strings.Contains(modelLower, "gemini") || strings.Contains(modelLower, "thinking") || agent.Provider == "anti_cli" {
		return c.antigravity.RunAgentCtx(ctx, agent, prompt, onLog, cwd)
	}

	return c.executeCtx(ctx, agent.Model, prompt, agent.System, agent.SessionID, onLog, cwd)
}

func (c *ClaudeCLI) RunOnce(prompt string, model string, system string, onLog LogCallback, cwd string) *RunResult {
	modelLower := strings.ToLower(model)
	if strings.Contains(modelLower, "gemini") || strings.Contains(modelLower, "thinking") {
		return c.antigravity.RunOnce(prompt, model, system, onLog, cwd)
	}
	return c.execute(model, prompt, system, "", onLog, cwd)
}

func (c *ClaudeCLI) RunSession(prompt string, model string, system string, sessionID string, onLog LogCallback, cwd string) *RunResult {
	modelLower := strings.ToLower(model)
	if strings.Contains(modelLower, "gemini") || strings.Contains(modelLower, "thinking") {
		return c.antigravity.RunSession(prompt, model, system, sessionID, onLog, cwd)
	}
	return c.execute(model, prompt, system, sessionID, onLog, cwd)
}

func (c *ClaudeCLI) execute(model, prompt, system, sessionID string, onLog LogCallback, cwd string) *RunResult {
	return c.executeCtx(context.Background(), model, prompt, system, sessionID, onLog, cwd)
}

func (c *ClaudeCLI) executeCtx(parent context.Context, model, prompt, system, sessionID string, onLog LogCallback, cwd string) *RunResult {
	startTime := time.Now()

	fullPrompt := prompt
	if system != "" {
		fullPrompt = fmt.Sprintf("[SYSTEM PROMPT: %s]\n\n%s", system, prompt)
	}

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

	var cmd *exec.Cmd
	if ShowCLIConsole {
		cmdArgs := append([]string{"/k", c.executablePath}, args...)
		cmd = exec.CommandContext(ctx, "cmd.exe", cmdArgs...)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    false,
			CreationFlags: 0x00000010, // CREATE_NEW_CONSOLE
		}
	} else {
		cmd = exec.CommandContext(ctx, c.executablePath, args...)
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
		return &RunResult{Success: false, Error: fmt.Sprintf("failed to start claude cli: %v", err)}
	}

	var stderrBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)

	parsed := &streamParse{}

	// Parse stdout as newline-delimited JSON events (stream-json).
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdoutPipe)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024) // events can be large
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			parseStreamEvent(line, parsed, onLog)
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
	errStr := stderrBuf.String()

	outStr := parsed.result
	if outStr == "" {
		outStr = parsed.assistantText.String()
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
		}
	}

	extractedSession := parsed.sessionID
	if extractedSession == "" {
		extractedSession = sessionID
	}

	tokens := parsed.totalTokens()
	if tokens == 0 {
		tokens = int64(len(outStr)+len(prompt)) / 4 // fallback estimate
	}

	return &RunResult{
		Success:     true,
		Output:      outStr,
		Error:       errStr,
		SessionID:   extractedSession,
		TokensUsed:  tokens,
		CostUSD:     parsed.costUSD,
		DurationSec: duration,
	}
}

// streamParse accumulates state while reading Claude CLI stream-json events.
type streamParse struct {
	assistantText strings.Builder
	result        string
	sessionID     string
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

	switch ev["type"] {
	case "assistant":
		msg, _ := ev["message"].(map[string]interface{})
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
			if len(v) > 120 {
				v = v[:120] + "…"
			}
			return "→ " + v
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
