package textutil

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// Every case asserts the secret VALUE is gone — not merely that something was
// replaced. The original ECC regex passed a weaker version of this test while
// leaking bearer tokens.
func TestScrubRemovesTheActualSecretValues(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		secret string
	}{
		{"api key equals", "calling with api_key=sk-abc123def", "sk-abc123def"},
		{"json token", `{"token": "ghp_ExampleSecret999"}`, "ghp_ExampleSecret999"},
		{"authorization bearer header", "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.payload.sig", "eyJhbGciOiJIUzI1NiJ9"},
		{"password spaced", "password = hunter2", "hunter2"},
		{"basic auth header", "AUTH=Basic dXNlcjpwYXNz", "dXNlcjpwYXNz"},
		{"bare bearer in prose", "gọi API với Bearer ya29.a0AfH6SMBx rồi retry", "ya29.a0AfH6SMBx"},
		{"x-api-key header", "x-api-key: 1234-5678-secret", "1234-5678-secret"},
		{"refresh token", `"refresh_token": "1//0gExampleRefresh"`, "0gExampleRefresh"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Scrub(tc.input)
			if strings.Contains(got, tc.secret) {
				t.Fatalf("secret survived scrubbing:\n in: %s\nout: %s", tc.input, got)
			}
			if !strings.Contains(got, "[REDACTED]") {
				t.Fatalf("nothing was redacted:\n in: %s\nout: %s", tc.input, got)
			}
		})
	}
}

func TestScrubLeavesOrdinaryTextAlone(t *testing.T) {
	in := "Sửa hàm BuildTaskPack trong backend/services/taskpack.go, thêm test tiếng Việt."
	if got := Scrub(in); got != in {
		t.Fatalf("ordinary text was mangled:\n in: %s\nout: %s", in, got)
	}
}

// Scrub runs before textutil.Truncate on Vietnamese payloads — the pipeline
// must keep the text valid UTF-8 end to end.
func TestScrubThenTruncateKeepsValidUTF8(t *testing.T) {
	in := strings.Repeat("lỗi xác thực với token=abc123 khi gọi dịch vụ; ", 100)
	out := Truncate(Scrub(in), 500, "…")
	if !utf8.ValidString(out) {
		t.Fatal("scrub+truncate produced invalid UTF-8")
	}
	if strings.Contains(out, "abc123") {
		t.Fatal("secret survived the pipeline")
	}
}
