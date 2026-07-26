// Package modelcatalog is the single source of truth for which models the app
// offers. Every dropdown used to carry its own hand-copied list — nine copies,
// each missing different entries, all dead the day a provider ships something
// new. The catalog merges four sources instead:
//
//   - aliases: "opus"/"sonnet"/"haiku" — the Claude CLI resolves these to the
//     newest model of each tier, so a brand-new Opus works the day it ships,
//     with no app update;
//   - builtin: the curated seed list;
//   - observed: model ids the CLIs actually reported running (stream-json
//     carries the real model) — the app learns new ids from its own runs;
//   - discovered: the live Gemini list from the Generative Language API,
//     fetched with the key pool's own API keys.
//
// The Claude CLI has no list-models command and subscription auth has no API
// to ask, which is why aliases + observation carry that side.
package modelcatalog

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"claude_suite/backend/models"
	"claude_suite/backend/provider"
)

// Model is one catalog entry.
type Model struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Provider string `json:"provider"` // provider.ProviderClaude | provider.ProviderAnti
	// Source: alias | builtin | observed | discovered
	Source     string `json:"source"`
	TokenLimit int64  `json:"token_limit"`
}

// DefaultTokenLimit applies to models the app has never seen — an unknown id
// must be usable, not rejected.
const DefaultTokenLimit int64 = 1_000_000

const catalogFile = "model_catalog.json"

type persisted struct {
	Observed   map[string]string `json:"observed"` // id → first seen (RFC3339)
	Discovered []Model           `json:"discovered"`
	RefreshedAt string           `json:"refreshed_at"`
}

// Catalog merges the sources and persists the learned ones beside the database.
type Catalog struct {
	mu      sync.Mutex
	dataDir string
	state   persisted
	// httpClient is swappable for tests.
	httpClient *http.Client
	geminiBase string
}

func New(dataDir string) *Catalog {
	c := &Catalog{
		dataDir:    dataDir,
		state:      persisted{Observed: map[string]string{}},
		httpClient: &http.Client{Timeout: 10 * time.Second},
		geminiBase: "https://generativelanguage.googleapis.com/v1beta/models",
	}
	if data, err := os.ReadFile(filepath.Join(dataDir, catalogFile)); err == nil {
		_ = json.Unmarshal(data, &c.state)
		if c.state.Observed == nil {
			c.state.Observed = map[string]string{}
		}
	}
	return c
}

// aliases always lead the list: they are the future-proof choice.
func aliases() []Model {
	return []Model{
		{ID: "opus", Label: "Opus — luôn bản mới nhất", Provider: provider.ProviderClaude, Source: "alias", TokenLimit: DefaultTokenLimit},
		{ID: "sonnet", Label: "Sonnet — luôn bản mới nhất", Provider: provider.ProviderClaude, Source: "alias", TokenLimit: DefaultTokenLimit},
		{ID: "haiku", Label: "Haiku — luôn bản mới nhất", Provider: provider.ProviderClaude, Source: "alias", TokenLimit: DefaultTokenLimit},
	}
}

// builtins seed the curated list. Token limits come from models.TokenLimits
// so the two stay in one place.
func builtins() []Model {
	seed := []struct {
		id, label string
	}{
		{"claude-opus-4-8", "Claude 4.8 Opus"},
		{"claude-sonnet-4-5", "Claude 4.5 Sonnet"},
		{"claude-haiku-4-5", "Claude 4.5 Haiku"},
		{"gemini-3.6-flash-high", "Gemini 3.6 Flash (High)"},
		{"gemini-3.6-flash-medium", "Gemini 3.6 Flash (Medium)"},
		{"gemini-3.6-flash-low", "Gemini 3.6 Flash (Low)"},
		{"gemini-3.5-flash-high", "Gemini 3.5 Flash (High)"},
		{"gemini-3.1-pro-high", "Gemini 3.1 Pro (High)"},
		{"claude-sonnet-4.6-thinking", "Claude 4.6 Sonnet Thinking (Antigravity)"},
		{"claude-opus-4.6-thinking", "Claude 4.6 Opus Thinking (Antigravity)"},
	}
	out := make([]Model, 0, len(seed))
	for _, s := range seed {
		out = append(out, Model{
			ID:         s.id,
			Label:      s.label,
			Provider:   provider.ResolveProvider("", s.id),
			Source:     "builtin",
			TokenLimit: TokenLimitFor(s.id),
		})
	}
	return out
}

// TokenLimitFor returns the known limit or the safe default — an unknown model
// must never be rejected for lacking a hardcoded row.
func TokenLimitFor(id string) int64 {
	if limit, ok := models.TokenLimits[id]; ok {
		return limit
	}
	return DefaultTokenLimit
}

// List returns the merged catalog: aliases, builtin Claude, observed, builtin
// Antigravity, then discovered Gemini — deduped by id, first source wins.
func (c *Catalog) List() []Model {
	c.mu.Lock()
	defer c.mu.Unlock()

	var merged []Model
	seen := map[string]bool{}
	add := func(m Model) {
		if m.ID == "" || seen[m.ID] {
			return
		}
		seen[m.ID] = true
		merged = append(merged, m)
	}

	for _, m := range aliases() {
		add(m)
	}
	for _, m := range builtins() {
		add(m)
	}

	observedIDs := make([]string, 0, len(c.state.Observed))
	for id := range c.state.Observed {
		observedIDs = append(observedIDs, id)
	}
	sort.Strings(observedIDs)
	for _, id := range observedIDs {
		add(Model{
			ID:         id,
			Label:      id + " (đã thấy trong run)",
			Provider:   provider.ResolveProvider("", id),
			Source:     "observed",
			TokenLimit: TokenLimitFor(id),
		})
	}

	for _, m := range c.state.Discovered {
		add(m)
	}
	return merged
}

// RecordObserved learns a model id a CLI reported actually running. Returns
// true when the id is new to the catalog.
func (c *Catalog) RecordObserved(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, m := range aliases() {
		if m.ID == id {
			return false
		}
	}
	for _, m := range builtins() {
		if m.ID == id {
			return false
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, known := c.state.Observed[id]; known {
		return false
	}
	c.state.Observed[id] = time.Now().Format(time.RFC3339)
	c.saveLocked()
	return true
}

// RefreshGemini fetches the live model list with the pool's API keys. The
// first key that answers wins. Returns how many models the API reported.
func (c *Catalog) RefreshGemini(apiKeys []string) (int, error) {
	var lastErr error
	for _, key := range apiKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		discovered, err := c.fetchGeminiModels(key)
		if err != nil {
			lastErr = err
			continue
		}
		c.mu.Lock()
		c.state.Discovered = discovered
		c.state.RefreshedAt = time.Now().Format(time.RFC3339)
		c.saveLocked()
		c.mu.Unlock()
		return len(discovered), nil
	}
	if lastErr != nil {
		return 0, lastErr
	}
	return 0, fmt.Errorf("pool không có API key nào (chỉ OAuth) — không gọi được API liệt kê model")
}

func (c *Catalog) fetchGeminiModels(apiKey string) ([]Model, error) {
	type apiModel struct {
		Name             string   `json:"name"`
		DisplayName      string   `json:"displayName"`
		SupportedMethods []string `json:"supportedGenerationMethods"`
	}
	type apiResp struct {
		Models        []apiModel `json:"models"`
		NextPageToken string     `json:"nextPageToken"`
	}

	var out []Model
	pageToken := ""
	for page := 0; page < 3; page++ {
		u := c.geminiBase + "?pageSize=200&key=" + url.QueryEscape(apiKey)
		if pageToken != "" {
			u += "&pageToken=" + url.QueryEscape(pageToken)
		}
		resp, err := c.httpClient.Get(u)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("models API trả %d", resp.StatusCode)
		}
		var parsed apiResp
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, err
		}
		for _, m := range parsed.Models {
			id := strings.TrimPrefix(m.Name, "models/")
			if !strings.Contains(strings.ToLower(id), "gemini") {
				continue
			}
			if !supportsGenerate(m.SupportedMethods) {
				continue
			}
			label := m.DisplayName
			if label == "" {
				label = id
			}
			out = append(out, Model{
				ID:         id,
				Label:      label + " (API)",
				Provider:   provider.ProviderAnti,
				Source:     "discovered",
				TokenLimit: TokenLimitFor(id),
			})
		}
		if parsed.NextPageToken == "" {
			break
		}
		pageToken = parsed.NextPageToken
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func supportsGenerate(methods []string) bool {
	if len(methods) == 0 {
		return true // older API omits the field; do not hide everything
	}
	for _, m := range methods {
		if strings.EqualFold(m, "generateContent") {
			return true
		}
	}
	return false
}

// saveLocked persists observed/discovered. Callers hold c.mu.
func (c *Catalog) saveLocked() {
	data, err := json.MarshalIndent(c.state, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(c.dataDir, catalogFile), append(data, '\n'), 0o644)
}
