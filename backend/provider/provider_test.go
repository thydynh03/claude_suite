// Package provider decides which CLI runs a request.
//
// It is its own package, and imports nothing but strings, so that every layer
// can reach it. backend/cli cannot import backend/core — core needs services and
// services needs cli — and cli is one of the places that was making this
// decision on its own.
package provider

import "testing"

// One table, so adding a model means editing one place instead of hoping both
// frontends were updated the same way.
func TestResolveProvider(t *testing.T) {
	cases := []struct {
		name     string
		explicit string
		model    string
		want     string
	}{
		// An explicit choice is a decision the user made; it wins.
		{"explicit anti wins over a claude model", ProviderAnti, "claude-sonnet-4-5", ProviderAnti},
		{"explicit claude wins over a gemini model", ProviderClaude, "gemini-3.6-flash-high", ProviderClaude},
		{"runner key is accepted as explicit", RunnerAnti, "claude-opus-4-8", ProviderAnti},

		// With nothing explicit, the model decides. This is the case the desktop
		// app handled and the terminal UI did not.
		{"gemini model infers anti", "", "gemini-3.1-pro-high", ProviderAnti},
		{"thinking model infers anti", "", "some-thinking-model", ProviderAnti},
		{"antigravity model infers anti", "", "antigravity-x", ProviderAnti},
		{"claude model infers claude", "", "claude-sonnet-4-5", ProviderClaude},
		{"unknown model falls back to claude", "", "mystery-model", ProviderClaude},
		{"nothing at all falls back to claude", "", "", ProviderClaude},

		// Neither side should care about case or stray spaces.
		{"case insensitive explicit", "ANTI", "claude-sonnet-4-5", ProviderAnti},
		{"padded explicit", "  anti  ", "claude-sonnet-4-5", ProviderAnti},
		{"case insensitive model", "", "GEMINI-3.6-FLASH", ProviderAnti},

		// An unrecognised explicit value must not silently pin claude when the
		// model plainly says otherwise.
		{"garbage explicit defers to the model", "not-a-provider", "gemini-3.6-flash-high", ProviderAnti},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveProvider(tc.explicit, tc.model); got != tc.want {
				t.Fatalf("ResolveProvider(%q, %q) = %q, want %q", tc.explicit, tc.model, got, tc.want)
			}
		})
	}
}

func TestRunnerKey(t *testing.T) {
	if got := RunnerKey(ProviderAnti); got != RunnerAnti {
		t.Errorf("RunnerKey(anti) = %q", got)
	}
	if got := RunnerKey(ProviderClaude); got != RunnerClaude {
		t.Errorf("RunnerKey(claude) = %q", got)
	}
	// Anything unrecognised must not silently become the paid Antigravity path.
	if got := RunnerKey("nonsense"); got != RunnerClaude {
		t.Errorf("RunnerKey(nonsense) = %q, want the claude runner", got)
	}
}
