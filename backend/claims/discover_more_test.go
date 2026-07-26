package claims

import (
	"os"
	"path/filepath"
	"testing"
)

// A workspace in a language the app did not recognise discovered nothing, so
// every claim about it became an unfalsifiable opinion and nothing could ever
// block a merge. That is what a user pointing this at their own project hits.
func TestDiscoverFindsChecksInLanguagesOtherThanGoAndNode(t *testing.T) {
	cases := []struct {
		file     string
		content  string
		wantName string
	}{
		{"Cargo.toml", "[package]\nname=\"x\"\n", "cargo-test"},
		{"pyproject.toml", "[project]\nname=\"x\"\n", "pytest"},
		{"requirements.txt", "pytest\n", "pytest"},
		{"pom.xml", "<project/>", "maven-test"},
		{"build.gradle", "plugins {}", "gradle-test"},
	}

	for _, tc := range cases {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, tc.file), []byte(tc.content), 0644); err != nil {
			t.Fatal(err)
		}

		cat := Discover(dir)
		var found bool
		for _, c := range cat.Checks {
			if c.Name == tc.wantName {
				found = true
				if len(c.Command) == 0 {
					t.Errorf("%s: check %q has no command", tc.file, c.Name)
				}
				if c.TimeoutSec <= 0 {
					t.Errorf("%s: check %q has no timeout, so it could hang the session", tc.file, c.Name)
				}
			}
		}
		if !found {
			t.Errorf("%s: no %q check discovered; got %+v", tc.file, tc.wantName, cat.Checks)
		}
	}
}

// An empty directory must still produce a catalogue rather than nil, or callers
// have to guard every use.
func TestDiscoverOnAnUnknownProjectReturnsAnEmptyCatalogue(t *testing.T) {
	cat := Discover(t.TempDir())
	if cat == nil {
		t.Fatal("Discover returned nil")
	}
	if len(cat.Checks) != 0 {
		t.Errorf("checks = %+v, want none for an empty directory", cat.Checks)
	}
}
