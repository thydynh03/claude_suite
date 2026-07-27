package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentUserDataDirUsesLocalAppData(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)

	want := filepath.Join(base, "AgentCenter", "ChromeAgent")
	if got := AgentUserDataDir(); got != want {
		t.Errorf("AgentUserDataDir() = %q, want %q", got, want)
	}
}

func TestAgentUserDataDirFallsBackToTempDir(t *testing.T) {
	t.Setenv("LOCALAPPDATA", "")

	want := filepath.Join(os.TempDir(), "AgentCenterChromeAgent")
	if got := AgentUserDataDir(); got != want {
		t.Errorf("AgentUserDataDir() = %q, want %q", got, want)
	}
}

// The profile path can contain spaces, because it is rooted at LOCALAPPDATA and
// a Windows account name may have one. That is fine: the directory is only ever
// passed as its own element of an exec.Command argument slice, which Go quotes
// for the Windows command line, and agentChromePIDs matches it in Go rather than
// interpolating it into a PowerShell string.
//
// This test exists because a comment here used to claim the path "deliberately
// contains no spaces", which is not something this function can promise.
func TestAgentUserDataDirDoesNotPromiseASpaceFreePath(t *testing.T) {
	t.Setenv("LOCALAPPDATA", filepath.Join("C:\\", "Users", "John Smith", "AppData", "Local"))

	if got := AgentUserDataDir(); !strings.Contains(got, " ") {
		t.Errorf("expected the account name's space to survive, got %q", got)
	}
}

func TestDevToolsActivePortReadsFirstLine(t *testing.T) {
	dir := t.TempDir()
	// Chrome writes the port on the first line and the browser target path on the
	// second.
	contents := "63412\n/devtools/browser/6f0a-…\n"
	if err := os.WriteFile(filepath.Join(dir, "DevToolsActivePort"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := devToolsActivePort(dir); got != 63412 {
		t.Errorf("devToolsActivePort() = %d, want 63412", got)
	}
}

func TestDevToolsActivePortReturnsZeroWhenUnusable(t *testing.T) {
	cases := map[string]string{
		"missing file": "",
		"not a number": "not-a-port\n/devtools/browser/x\n",
		"empty file":   "\n",
	}

	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if contents != "" {
				if err := os.WriteFile(filepath.Join(dir, "DevToolsActivePort"), []byte(contents), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if got := devToolsActivePort(dir); got != 0 {
				t.Errorf("devToolsActivePort() = %d, want 0", got)
			}
		})
	}
}

func TestLooksLikeLoginWall(t *testing.T) {
	cases := []struct {
		name  string
		url   string
		title string
		want  bool
	}{
		{"login path", "https://example.com/login?next=/app", "Example", true},
		{"underscore variant", "https://example.com/sign_in", "Example", true},
		{"google accounts", "https://accounts.google.com/o/oauth2/v2/auth", "Google", true},
		{"oauth authorize", "https://id.example.com/oauth/authorize?x=1", "Example", true},
		{"english title", "https://example.com/app", "Sign in · Example", true},
		{"vietnamese title", "https://example.com/app", "Đăng nhập hệ thống", true},
		{"title match is case insensitive", "https://example.com/app", "LOG IN", true},
		{"ordinary page", "https://example.com/dashboard", "Dashboard", false},
		{"title mentioning login later is not a wall", "https://example.com/help", "How to log in", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := LooksLikeLoginWall(tc.url, tc.title); got != tc.want {
				t.Errorf("LooksLikeLoginWall(%q, %q) = %v, want %v", tc.url, tc.title, got, tc.want)
			}
		})
	}
}

// Known limitation, pinned so a change to it is a decision rather than a
// surprise: URL hints are substring matches, so a path that merely starts with
// one of them counts as a sign-in page. The cost is asking the user a question
// that was not needed, which is why the loose match is tolerated.
func TestLooksLikeLoginWallMatchesURLHintsAsSubstrings(t *testing.T) {
	if !LooksLikeLoginWall("https://example.com/signing-documents", "Documents") {
		t.Skip("URL hints are no longer substring matches — update the comment above")
	}
}

func TestJSStringEscapesForInjection(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"plain", "hello", `"hello"`},
		{"double quote", `say "hi"`, `"say \"hi\""`},
		{"backslash", `C:\path`, `"C:\\path"`},
		{"newline", "a\nb", `"a\nb"`},
		{"unicode survives", "đăng nhập", `"đăng nhập"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := jsString(tc.input); got != tc.want {
				t.Errorf("jsString(%q) = %s, want %s", tc.input, got, tc.want)
			}
		})
	}
}

// The result is interpolated into a JavaScript expression that is evaluated in
// the page, so a value must not be able to end the string literal or the script
// block it sits in — and it must still mean the same thing once parsed.
func TestJSStringCannotEscapeItsContext(t *testing.T) {
	hostile := []string{
		`</script><script>alert(1)</script>`,
		`"; alert(1); "`,
		"a b", // U+2028 ends a line in JavaScript, but not in JSON
		"a b",
		`trailing backslash \`,
	}

	for _, input := range hostile {
		encoded := jsString(input)

		// A raw < or > would let the value close the surrounding <script> block.
		for _, forbidden := range []string{"<", ">", " ", " "} {
			if strings.Contains(encoded, forbidden) {
				t.Errorf("jsString(%q) = %s, still contains a raw %q", input, encoded, forbidden)
			}
		}

		var decoded string
		if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
			t.Errorf("jsString(%q) = %s, which is not a valid literal: %v", input, encoded, err)
			continue
		}
		if decoded != input {
			t.Errorf("jsString(%q) decoded back to %q", input, decoded)
		}
	}
}
