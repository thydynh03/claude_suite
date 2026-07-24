package version

import "testing"

func TestVersionString(t *testing.T) {
	if CurrentVersion == "" {
		t.Errorf("expected non-empty CurrentVersion")
	}
	if len(CurrentVersion) < 2 || CurrentVersion[0] != 'v' {
		t.Errorf("expected version to start with 'v', got %s", CurrentVersion)
	}
}
