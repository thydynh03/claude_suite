package orchestrator

import "testing"

func TestExtractTestURL(t *testing.T) {
	if got := extractTestURL("open http://localhost:3000/login and check"); got != "http://localhost:3000/login" {
		t.Errorf("got %q", got)
	}
	if got := extractTestURL("no url here"); got != "http://localhost:5173" {
		t.Errorf("default failed: %q", got)
	}
	if got := extractTestURL("visit https://example.com."); got != "https://example.com" {
		t.Errorf("trailing dot not trimmed: %q", got)
	}
}

func TestExtractExpectedTexts(t *testing.T) {
	got := extractExpectedTexts("check [EXPECT: Login] and [EXPECT TEXT: Welcome back]")
	if len(got) != 2 || got[0] != "Login" || got[1] != "Welcome back" {
		t.Errorf("got %v", got)
	}
	if len(extractExpectedTexts("nothing")) != 0 {
		t.Error("expected empty")
	}
}
