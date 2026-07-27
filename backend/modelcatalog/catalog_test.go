package modelcatalog

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agent_center/backend/provider"
)

func TestListLeadsWithAliasesAndCoversBothProviders(t *testing.T) {
	c := New(t.TempDir())
	list := c.List()

	if len(list) < 10 {
		t.Fatalf("catalog too small: %d entries", len(list))
	}
	if list[0].ID != "opus" || list[0].Source != "alias" {
		t.Fatalf("aliases must lead the list, got %+v", list[0])
	}

	providers := map[string]bool{}
	ids := map[string]bool{}
	for _, m := range list {
		providers[m.Provider] = true
		if ids[m.ID] {
			t.Fatalf("duplicate id %q in catalog", m.ID)
		}
		ids[m.ID] = true
		if m.TokenLimit <= 0 {
			t.Fatalf("model %q has no token limit", m.ID)
		}
	}
	if !providers[provider.ProviderClaude] || !providers[provider.ProviderAnti] {
		t.Fatalf("both providers must be represented: %+v", providers)
	}
	// The seed must carry everything TokenLimits knows about — the screenshot
	// bug was dropdowns missing models that existed elsewhere in the app.
	for _, want := range []string{"gemini-3.6-flash-medium", "claude-sonnet-4.6-thinking"} {
		if !ids[want] {
			t.Fatalf("builtin list missing %q", want)
		}
	}
}

// A model id reported by a run — e.g. claude-opus-5 the day it ships — must
// appear in the catalog afterwards, and survive a reload.
func TestObservedModelsAreLearnedAndPersisted(t *testing.T) {
	dir := t.TempDir()
	c := New(dir)

	if !c.RecordObserved("claude-opus-5-20260901") {
		t.Fatal("new id must be recorded")
	}
	if c.RecordObserved("claude-opus-5-20260901") {
		t.Fatal("second observation of the same id must be a no-op")
	}
	if c.RecordObserved("claude-opus-4-8") {
		t.Fatal("builtin ids must not duplicate into observed")
	}

	reloaded := New(dir)
	found := false
	for _, m := range reloaded.List() {
		if m.ID == "claude-opus-5-20260901" {
			found = true
			if m.Provider != provider.ProviderClaude || m.Source != "observed" {
				t.Fatalf("observed entry shape wrong: %+v", m)
			}
		}
	}
	if !found {
		t.Fatal("observed model did not survive a reload")
	}
}

func TestRefreshGeminiFetchesAndPersistsTheLiveList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "good-key" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"models": []map[string]interface{}{
				{"name": "models/gemini-9.9-flash", "displayName": "Gemini 9.9 Flash", "supportedGenerationMethods": []string{"generateContent"}},
				{"name": "models/gemini-embed-1", "displayName": "Embedding", "supportedGenerationMethods": []string{"embedContent"}},
				{"name": "models/imagen-x", "displayName": "Imagen", "supportedGenerationMethods": []string{"generateContent"}},
			},
		})
	}))
	defer server.Close()

	dir := t.TempDir()
	c := New(dir)
	c.geminiBase = server.URL

	// Bad key first: refresh must fall through to the working one.
	n, err := c.RefreshGemini([]string{"bad-key", "good-key"})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if n != 1 {
		t.Fatalf("discovered %d models, want 1 (generateContent gemini only)", n)
	}

	reloaded := New(dir)
	found := false
	for _, m := range reloaded.List() {
		if m.ID == "gemini-9.9-flash" {
			found = true
			if m.Provider != provider.ProviderAnti || m.Source != "discovered" {
				t.Fatalf("discovered entry shape wrong: %+v", m)
			}
			if !strings.Contains(m.Label, "API") {
				t.Fatalf("discovered label must say it came from the API: %q", m.Label)
			}
		}
	}
	if !found {
		t.Fatal("discovered model did not survive a reload")
	}
}

func TestRefreshGeminiWithoutKeysErrs(t *testing.T) {
	c := New(t.TempDir())
	if _, err := c.RefreshGemini(nil); err == nil {
		t.Fatal("no keys must be an explicit error, not a silent empty refresh")
	}
}

func TestTokenLimitForUnknownModelHasADefault(t *testing.T) {
	if got := TokenLimitFor("claude-opus-5-brand-new"); got != DefaultTokenLimit {
		t.Fatalf("unknown model limit = %d, want default %d", got, DefaultTokenLimit)
	}
	if got := TokenLimitFor("gemini-3.6-flash-high"); got != 2_000_000 {
		t.Fatalf("known model must keep its curated limit, got %d", got)
	}
}
