// Package provider decides which CLI runs a request.
//
// It is its own package, and imports nothing but strings, so that every layer
// can reach it. backend/cli cannot import backend/core — core needs services and
// services needs cli — and cli is one of the places that was making this
// decision on its own.
package provider

import "strings"

// Provider names used throughout the app.
const (
	ProviderClaude = "claude"
	ProviderAnti   = "anti"
)

// Runner keys as the CLI layer stores them.
const (
	RunnerClaude = "claude_cli"
	RunnerAnti   = "anti_cli"
)

// antiModelMarkers are the substrings that identify a model served by the
// Antigravity CLI rather than the Claude one.
//
// "thinking" is here because the Antigravity model list has carried names ending
// in it, and routing one of those to the Claude runner fails at the far end with
// an error that says nothing about why.
var antiModelMarkers = []string{"gemini", "thinking", "antigravity"}

// ResolveProvider decides which CLI should run a request.
//
// The two frontends used to answer this differently. The desktop app inferred
// the provider from the model string; the terminal UI took an explicit argument
// and compared it to one constant. They did not disagree in practice, but only
// because the terminal UI's provider toggle also swapped its model list — the
// agreement was held by a UI detail, not by the code, and adding a model in one
// place would have broken it silently.
//
// An explicit choice wins when it is given, because a user who picked a provider
// meant it. Otherwise the model name decides, which is what makes passing a
// Gemini model without naming a provider still reach the right runner.
func ResolveProvider(explicit, model string) string {
	switch strings.ToLower(strings.TrimSpace(explicit)) {
	case ProviderAnti, RunnerAnti:
		return ProviderAnti
	case ProviderClaude, RunnerClaude:
		return ProviderClaude
	}

	lower := strings.ToLower(model)
	for _, marker := range antiModelMarkers {
		if strings.Contains(lower, marker) {
			return ProviderAnti
		}
	}
	return ProviderClaude
}

// RunnerKey maps a provider to the key the CLI session store uses.
func RunnerKey(provider string) string {
	if provider == ProviderAnti {
		return RunnerAnti
	}
	return RunnerClaude
}

// IsAnti reports whether a resolved provider is the Antigravity one.
func IsAnti(provider string) bool { return provider == ProviderAnti }
