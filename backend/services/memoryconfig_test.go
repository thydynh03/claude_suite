package services

import (
	"os"
	"path/filepath"
	"testing"

	"agent_center/backend/models"
)

func TestMemoryConfigRoundTripsAndKeepsTheKillSwitch(t *testing.T) {
	dir := t.TempDir()

	// Missing file → defaults.
	cfg := LoadMemoryConfig(dir)
	if cfg.ContextPackMaxChars != DefaultPackBudget || !cfg.AutoSummarize || cfg.SessionResume || cfg.LessonPromotion {
		t.Fatalf("defaults wrong: %+v", cfg)
	}

	// A stored 0 budget is the user's kill switch and must survive reload.
	cfg.ContextPackMaxChars = 0
	cfg.SessionResume = true
	if err := SaveMemoryConfig(dir, cfg); err != nil {
		t.Fatal(err)
	}
	got := LoadMemoryConfig(dir)
	if got.ContextPackMaxChars != 0 {
		t.Fatalf("stored 0 budget came back as %d — kill switch lost", got.ContextPackMaxChars)
	}
	if !got.SessionResume {
		t.Fatal("session resume setting lost")
	}
}

// A config file from an older build without the newer fields keeps their
// defaults instead of zeroing them.
func TestMemoryConfigOldFileKeepsNewDefaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "memory_config.json"), []byte(`{"context_pack_max_chars": 5000}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := LoadMemoryConfig(dir)
	if got.ContextPackMaxChars != 5000 {
		t.Fatalf("stored budget = %d, want 5000", got.ContextPackMaxChars)
	}
	if !got.AutoSummarize {
		t.Fatal("missing auto_summarize field must keep its true default")
	}
	if got != (models.MemoryConfig{ContextPackMaxChars: 5000, AutoSummarize: true}) {
		t.Fatalf("unexpected config: %+v", got)
	}
}
