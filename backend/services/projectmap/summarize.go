package projectmap

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"agent_center/backend/cli"
	"agent_center/backend/models"
	"agent_center/backend/textutil"
)

const (
	summarizeBatchSize   = 10
	summarizeMaxFileSize = 6000 // chars of one file shown to the model
	summarizeCooldown    = 10 * time.Minute
)

// summarizerState is set via SetSummarizer; without it the map stays fully
// deterministic (signature summaries) and spends zero tokens.
type summarizerState struct {
	mu       sync.Mutex
	runner   cli.CLIRunner
	model    string
	auto     bool
	lastAuto map[string]time.Time
}

// SetSummarizer enables LLM summary enrichment. model should be a cheap one —
// the same "claude-haiku-4-5" the commit-message generator already uses.
func (m *Mapper) SetSummarizer(runner cli.CLIRunner, model string) {
	m.summarizer.mu.Lock()
	defer m.summarizer.mu.Unlock()
	m.summarizer.runner = runner
	m.summarizer.model = model
	if m.summarizer.lastAuto == nil {
		m.summarizer.lastAuto = make(map[string]time.Time)
	}
}

// SetAutoSummarize turns on the post-update drip: after an incremental update
// leaves stale summaries behind, one batch is refreshed in the background,
// rate-limited per workspace. Manual RefreshSummaries ignores this switch.
func (m *Mapper) SetAutoSummarize(enabled bool) {
	m.summarizer.mu.Lock()
	m.summarizer.auto = enabled
	m.summarizer.mu.Unlock()
}

// RefreshSummaries enriches up to maxBatches × 10 stale file summaries with
// one cheap-model call per batch. Returns how many files were refreshed.
func (m *Mapper) RefreshSummaries(workspaceDir string, maxBatches int) (int, error) {
	m.summarizer.mu.Lock()
	runner, model := m.summarizer.runner, m.summarizer.model
	m.summarizer.mu.Unlock()
	if runner == nil {
		return 0, fmt.Errorf("summarizer chưa được cấu hình")
	}
	if maxBatches <= 0 {
		maxBatches = 1
	}

	ws, err := m.ws.EnsureWorkspace(workspaceDir, gitRemoteURL(workspaceDir))
	if err != nil {
		return 0, err
	}

	total := 0
	for batch := 0; batch < maxBatches; batch++ {
		nodes, err := m.graph.StaleFileNodes(ws.WorkspaceID, summarizeBatchSize)
		if err != nil {
			return total, err
		}
		if len(nodes) == 0 {
			break
		}

		prompt := buildSummaryPrompt(workspaceDir, nodes)
		result := runner.RunOnce(prompt, model, "", nil, "")
		if result == nil || !result.Success {
			return total, fmt.Errorf("summary call failed: %s", safeErr(result))
		}

		valid := map[string]bool{}
		fresh := make([]string, 0, len(nodes))
		for _, n := range nodes {
			valid[n.NodeID] = true
			fresh = append(fresh, n.NodeID)
		}
		updates := parseSummaryUpdates(result.Output, valid)
		// Every node in the batch is marked fresh even when the model skipped
		// it — a stubborn file must not wedge the refresh loop forever.
		if err := m.graph.ApplySummaries(ws.WorkspaceID, updates, fresh); err != nil {
			return total, err
		}
		total += len(nodes)
	}
	return total, nil
}

// maybeAutoSummarize is the post-incremental-update drip.
func (m *Mapper) maybeAutoSummarize(workspaceDir, workspaceID string) {
	m.summarizer.mu.Lock()
	enabled := m.summarizer.auto && m.summarizer.runner != nil
	if enabled {
		if last, ok := m.summarizer.lastAuto[workspaceID]; ok && time.Since(last) < summarizeCooldown {
			enabled = false
		} else {
			m.summarizer.lastAuto[workspaceID] = time.Now()
		}
	}
	m.summarizer.mu.Unlock()
	if !enabled {
		return
	}
	go func() { _, _ = m.RefreshSummaries(workspaceDir, 1) }()
}

func buildSummaryPrompt(workspaceDir string, nodes []models.CodeNode) string {
	var b strings.Builder
	b.WriteString(`You are summarizing source files of one project for a code map that other AI agents will read.
For EVERY file below, write ONE summary: what the file is responsible for and what its key exports do — max 2 sentences, max 350 chars. Add 2-5 short lowercase tags.
Respond ONLY with JSON: {"summaries":[{"id":"<the exact id given>","summary":"...","tags":["..."]}]}

FILES:
`)
	for _, n := range nodes {
		content, err := ReadFile(workspaceDir, n.FilePath)
		body := ""
		if err == nil {
			body = textutil.Truncate(string(content), summarizeMaxFileSize, "\n… (truncated)")
		}
		b.WriteString(fmt.Sprintf("\n=== id: %s (path: %s) ===\n%s\n", n.NodeID, n.FilePath, body))
	}
	return b.String()
}

// parseSummaryUpdates repairs-or-drops the model's JSON (the ported
// Understand-Anything stance): unknown ids are dropped (referential
// integrity), lengths clamped, malformed output yields nothing — never a
// crash, never a corrupt node.
func parseSummaryUpdates(raw string, valid map[string]bool) []models.SummaryUpdate {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return nil
	}
	var parsed struct {
		Summaries []models.SummaryUpdate `json:"summaries"`
	}
	if err := json.Unmarshal([]byte(raw[start:end+1]), &parsed); err != nil {
		return nil
	}

	var out []models.SummaryUpdate
	for _, u := range parsed.Summaries {
		if !valid[u.NodeID] || strings.TrimSpace(u.Summary) == "" {
			continue
		}
		u.Summary = textutil.Truncate(strings.TrimSpace(u.Summary), 400, "…")
		if len(u.Tags) > 5 {
			u.Tags = u.Tags[:5]
		}
		for i, tag := range u.Tags {
			u.Tags[i] = textutil.Truncate(strings.ToLower(strings.TrimSpace(tag)), 24, "")
		}
		out = append(out, u)
	}
	return out
}

func safeErr(r *cli.RunResult) string {
	if r == nil {
		return "no result"
	}
	return textutil.Truncate(r.Error, 200, "…")
}
