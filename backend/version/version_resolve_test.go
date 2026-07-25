package version

import "testing"

func TestGetVersionPrefersInjectedBuildVersion(t *testing.T) {
	orig := BuildVersion
	t.Cleanup(func() { BuildVersion = orig })

	BuildVersion = "v3.1.4"
	if got := GetVersion(); got != "v3.1.4" {
		t.Fatalf("GetVersion() = %q, want the injected build version", got)
	}
}

func TestGetVersionFallsBackToDevWhenNotInjected(t *testing.T) {
	orig := BuildVersion
	t.Cleanup(func() { BuildVersion = orig })

	BuildVersion = devVersion
	if got := GetVersion(); got == "" {
		t.Fatal("GetVersion() returned empty for an uninjected build")
	}
}
