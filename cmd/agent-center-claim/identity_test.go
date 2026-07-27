package main

import "strings"

import "testing"

func TestDefaultAuthorNamesAMachineAndAnAgent(t *testing.T) {
	got := defaultAuthor("claude")

	if strings.Contains(got, "YOU") || strings.Contains(got, "your-agent") {
		t.Fatalf("author = %q, still the placeholder the app used to hand out", got)
	}
	if !strings.HasSuffix(got, "/claude") {
		t.Errorf("author = %q, want it to end in the provider name", got)
	}
	if strings.HasPrefix(got, "/") {
		t.Errorf("author = %q, want a user or host before the slash", got)
	}
}

func TestDefaultAuthorStillNamesSomeoneWhenTheProviderIsUnset(t *testing.T) {
	got := defaultAuthor("")

	if !strings.HasSuffix(got, "/agent") {
		t.Errorf("author = %q, want the generic agent suffix", got)
	}
	if strings.Count(got, "/") != 1 {
		t.Errorf("author = %q, want exactly one separator", got)
	}
}

// A space or a slash in a Windows account name would otherwise produce an author
// string that reads as a different user, or as an extra path segment.
func TestSanitiseRemovesTheSeparatorsTheFormatReliesOn(t *testing.T) {
	for _, in := range []string{"An Nguyen", "DOMAIN/an", "an@corp"} {
		got := sanitise(in)
		if strings.ContainsAny(got, " /@") {
			t.Errorf("sanitise(%q) = %q, still contains a separator", in, got)
		}
	}
}
