package textutil

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// The bug this package exists for: the old code sliced by byte index, so a cut
// that landed inside a multi-byte character produced invalid UTF-8. "ế" is three
// bytes, so every offset in this loop lands somewhere different inside it.
func TestTruncateNeverSplitsACharacter(t *testing.T) {
	for pad := 0; pad < 6; pad++ {
		text := strings.Repeat("a", pad) + "ế" + strings.Repeat("b", 20)
		got := Truncate(text, pad+2, "…")
		if !utf8.ValidString(got) {
			t.Errorf("pad=%d produced invalid UTF-8: %q", pad, got)
		}
	}
}

func TestTruncateTailNeverSplitsACharacter(t *testing.T) {
	for pad := 0; pad < 6; pad++ {
		text := strings.Repeat("a", 20) + "ế" + strings.Repeat("b", pad)
		got := TruncateTail(text, pad+2)
		if !utf8.ValidString(got) {
			t.Errorf("pad=%d produced invalid UTF-8: %q", pad, got)
		}
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		max    int
		suffix string
		want   string
	}{
		{"within budget is untouched", "hello", 10, "…", "hello"},
		{"exactly at budget is untouched", "hello", 5, "…", "hello"},
		{"ascii is cut at the budget", "hello world", 5, "…", "hello…"},
		{"cut backs off to a character boundary", "aaaế", 4, "…", "aaa…"},
		{"whole characters are kept", "aaaế", 6, "…", "aaaế"},
		{"zero max returns the input", "hello", 0, "…", "hello"},
		{"negative max returns the input", "hello", -1, "…", "hello"},
		{"empty suffix is allowed", "hello world", 5, "", "hello"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Truncate(tc.input, tc.max, tc.suffix); got != tc.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tc.input, tc.max, got, tc.want)
			}
		})
	}
}

func TestTruncateTail(t *testing.T) {
	cases := []struct {
		name  string
		input string
		max   int
		want  string
	}{
		{"within budget is untouched", "hello", 10, "hello"},
		{"keeps the end", "hello world", 5, "world"},
		{"start moves forward to a boundary", "ếbbb", 4, "bbb"},
		{"whole characters are kept", "ếbbb", 6, "ếbbb"},
		{"zero max returns the input", "hello", 0, "hello"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TruncateTail(tc.input, tc.max); got != tc.want {
				t.Errorf("TruncateTail(%q, %d) = %q, want %q", tc.input, tc.max, got, tc.want)
			}
		})
	}
}

// The budget bounds the payload, so the result must never exceed it apart from
// the suffix the caller asked for.
func TestTruncateRespectsTheBudget(t *testing.T) {
	text := strings.Repeat("ế", 100)
	got := Truncate(text, 50, "")
	if len(got) > 50 {
		t.Errorf("result is %d bytes, budget was 50", len(got))
	}
	if !utf8.ValidString(got) {
		t.Errorf("result is not valid UTF-8: %q", got)
	}
}
