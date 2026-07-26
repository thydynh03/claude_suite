package services

import (
	"encoding/json"
	"os"
	"path/filepath"

	"claude_suite/backend/models"
)

// memoryConfigFile sits beside the database like the other config files.
const memoryConfigFile = "memory_config.json"

// DefaultMemoryConfig is what a fresh install runs with: pack on at 12k chars,
// summary drip on, session resume and cross-workspace promotion off (both are
// the riskier halves — resume grows context unpredictably, promotion is where
// a bad lesson does the most damage).
func DefaultMemoryConfig() models.MemoryConfig {
	return models.MemoryConfig{
		ContextPackMaxChars: DefaultPackBudget,
		AutoSummarize:       true,
		SessionResume:       false,
		LessonPromotion:     false,
	}
}

// LoadMemoryConfig reads dataDir/memory_config.json over the defaults. A
// missing or unreadable file yields the defaults; a present field always wins
// (so a stored 0 budget stays the kill switch the user chose).
func LoadMemoryConfig(dataDir string) models.MemoryConfig {
	cfg := DefaultMemoryConfig()
	data, err := os.ReadFile(filepath.Join(dataDir, memoryConfigFile))
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

// SaveMemoryConfig persists the config beside the database.
func SaveMemoryConfig(dataDir string, cfg models.MemoryConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, memoryConfigFile), append(data, '\n'), 0o644)
}
