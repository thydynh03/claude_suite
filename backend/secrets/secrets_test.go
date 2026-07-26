package secrets

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
)

func TestSealOpenRoundTrip(t *testing.T) {
	plaintext := []byte(`[{"email":"a@gmail.com","refresh_token":"1//very-secret"}]`)

	sealed, err := Seal(plaintext)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if !bytes.HasPrefix(sealed, []byte(header)) {
		t.Fatal("sealed data does not carry the format header")
	}

	got, wasPlain, err := Open(sealed)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if wasPlain {
		t.Error("sealed data reported as plain")
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("round trip changed the data: %q", got)
	}
}

// The reason the file is sealed at all: the token must not sit on disk in
// the clear. Identity-sealing on other platforms is a documented trade, so
// the assertion is Windows-only.
func TestSealedBytesDoNotContainTheSecret(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("sealing is identity off Windows by design")
	}
	secret := "1//this-must-not-appear-on-disk"
	sealed, err := Seal([]byte(`{"refresh_token":"` + secret + `"}`))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if strings.Contains(string(sealed), secret) {
		t.Fatal("the sealed file still contains the secret in the clear")
	}
}

// Existing installs have a plain-JSON file; Open must hand it back verbatim
// and say so, which is what lets the caller re-save it sealed.
func TestOpenPassesLegacyPlainJSONThrough(t *testing.T) {
	legacy := []byte(`[{"email":"a@gmail.com"}]`)
	got, wasPlain, err := Open(legacy)
	if err != nil {
		t.Fatalf("open legacy: %v", err)
	}
	if !wasPlain {
		t.Error("legacy plaintext not reported as plain")
	}
	if !bytes.Equal(got, legacy) {
		t.Errorf("legacy data altered: %q", got)
	}
}

func TestOpenRejectsACorruptSealedBlob(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("sealing is identity off Windows by design")
	}
	if _, _, err := Open([]byte(header + "not a dpapi blob")); err == nil {
		t.Fatal("a corrupt sealed blob opened without error")
	}
}
