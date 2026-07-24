package cli

import "testing"

func TestCLIResolvers(t *testing.T) {
	claudePath := ResolveClaudeCLI()
	if claudePath == "" {
		t.Errorf("expected non-empty Claude CLI path")
	}

	antiPath := ResolveAntigravityCLI()
	if antiPath == "" {
		t.Errorf("expected non-empty Antigravity CLI path")
	}

	paths := GetDetectedCLIPaths()
	if _, ok := paths["claude"]; !ok {
		t.Errorf("expected claude in detected CLI paths map")
	}
}
