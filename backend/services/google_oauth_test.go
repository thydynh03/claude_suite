package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func withTokenEndpoint(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	previous := GoogleTokenEndpoint
	GoogleTokenEndpoint = server.URL
	t.Cleanup(func() {
		GoogleTokenEndpoint = previous
		server.Close()
	})
}

func TestRefreshGoogleAccessTokenReturnsTheToken(t *testing.T) {
	withTokenEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if got := r.PostForm.Get("grant_type"); got != "refresh_token" {
			t.Errorf("grant_type = %q, want refresh_token", got)
		}
		if got := r.PostForm.Get("refresh_token"); got != "1//refresh" {
			t.Errorf("refresh_token = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"ya29.access","expires_in":3600}`))
	})

	token, err := RefreshGoogleAccessToken(context.Background(), "id", "secret", "1//refresh")
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if token.AccessToken != "ya29.access" {
		t.Errorf("access token = %q", token.AccessToken)
	}
	// Expiry is pulled a minute in, so a token is never handed over nearly dead.
	if remaining := time.Until(token.ExpiresAt); remaining > 59*time.Minute || remaining < 58*time.Minute {
		t.Errorf("expires in %v, want a little under an hour", remaining)
	}
}

// A revoked token comes back as an error Google explains. Passing the reason on
// matters: invalid_grant means the user revoked access and retrying cannot help.
func TestRefreshGoogleAccessTokenReportsWhyGoogleRefused(t *testing.T) {
	withTokenEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`))
	})

	_, err := RefreshGoogleAccessToken(context.Background(), "id", "secret", "1//dead")
	if err == nil {
		t.Fatal("a revoked token was accepted")
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("error does not say why: %v", err)
	}
}

func TestRefreshGoogleAccessTokenNeedsCredentials(t *testing.T) {
	if _, err := RefreshGoogleAccessToken(context.Background(), "", "", "1//refresh"); err == nil {
		t.Error("missing client credentials were accepted")
	}
	if _, err := RefreshGoogleAccessToken(context.Background(), "id", "secret", ""); err == nil {
		t.Error("a missing refresh token was accepted")
	}
}

func TestRefreshGoogleAccessTokenDefaultsTheLifetime(t *testing.T) {
	withTokenEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"access_token":"ya29.access"}`)) // no expires_in
	})

	token, err := RefreshGoogleAccessToken(context.Background(), "id", "secret", "1//refresh")
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if !token.ExpiresAt.After(time.Now().Add(50 * time.Minute)) {
		t.Errorf("expiry %v is too soon for the assumed hour", token.ExpiresAt)
	}
}

func TestParseGoogleAccountsFile(t *testing.T) {
	accounts, err := ParseGoogleAccountsFile([]byte(`[
		{"email":"one@example.com","refresh_token":"1//aaa"},
		{"email":"two@example.com","refresh_token":"1//bbb"}
	]`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(accounts) != 2 || accounts[0].Email != "one@example.com" {
		t.Errorf("parsed %+v", accounts)
	}
}

// A half-written entry is skipped rather than imported as an account that can
// never authenticate.
func TestParseGoogleAccountsFileSkipsIncompleteEntries(t *testing.T) {
	accounts, err := ParseGoogleAccountsFile([]byte(`[
		{"email":"good@example.com","refresh_token":"1//aaa"},
		{"email":"no-token@example.com","refresh_token":""},
		{"email":"","refresh_token":"1//orphan"}
	]`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(accounts) != 1 || accounts[0].Email != "good@example.com" {
		t.Errorf("parsed %+v, want only the complete entry", accounts)
	}
}

func TestParseGoogleAccountsFileRejectsRubbish(t *testing.T) {
	if _, err := ParseGoogleAccountsFile([]byte(`not json`)); err == nil {
		t.Error("invalid JSON was accepted")
	}
	if _, err := ParseGoogleAccountsFile([]byte(`[]`)); err == nil {
		t.Error("an empty file was accepted")
	}
}
