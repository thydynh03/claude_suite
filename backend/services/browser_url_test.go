package services

import "testing"

// CDP's Page.navigate takes its URL literally: "google.com" — the way every
// person types it, and the exact input that produced "Cannot navigate to
// invalid URL (-32000)" in the Browser Agent — must arrive with a scheme.
func TestNormalizeNavURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"google.com", "https://google.com"},
		{"  google.com  ", "https://google.com"},
		{"localhost:5173/app", "https://localhost:5173/app"},
		{"https://google.com", "https://google.com"},
		{"http://localhost:5173", "http://localhost:5173"},
		{"HTTPS://Already.Fine", "HTTPS://Already.Fine"},
		{"about:blank", "about:blank"},
		{"file:///C:/tmp/x.html", "file:///C:/tmp/x.html"},
		{"", ""},
	}
	for _, c := range cases {
		if got := normalizeNavURL(c.in); got != c.want {
			t.Errorf("normalizeNavURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
