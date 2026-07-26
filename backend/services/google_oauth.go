package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GoogleTokenEndpoint is where a refresh token is exchanged for an access token.
// It is a variable so tests can point it at a local server.
var GoogleTokenEndpoint = "https://oauth2.googleapis.com/token"

// GoogleToken is the useful part of an exchange response.
type GoogleToken struct {
	AccessToken string
	ExpiresAt   time.Time
}

// ErrGoogleRefused marks an answer FROM Google — a revoked token, the wrong
// client — as opposed to never having reached it. Only a refusal justifies
// disabling an account for good: treating "no network" the same way disabled
// every account of a user who imported their file while offline, and nothing
// re-enables a disabled account.
var ErrGoogleRefused = errors.New("Google từ chối refresh token")

// RefreshGoogleAccessToken trades a long-lived refresh token for a short-lived
// access token.
//
// This is what makes a pool of Google accounts work at all. An access token is
// good for about an hour, so an account added by signing in once stops working
// shortly afterwards — rotating onto it after a 429 then achieves nothing. The
// refresh token is the part worth storing; the access token is derived from it
// whenever one is needed.
func RefreshGoogleAccessToken(ctx context.Context, clientID, clientSecret, refreshToken string) (GoogleToken, error) {
	if clientID == "" || clientSecret == "" {
		return GoogleToken{}, fmt.Errorf("thiếu GCP client id/secret để đổi refresh token")
	}
	if refreshToken == "" {
		return GoogleToken{}, fmt.Errorf("tài khoản không có refresh token")
	}

	form := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, GoogleTokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return GoogleToken{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		return GoogleToken{}, fmt.Errorf("gọi Google token endpoint: %w", err)
	}
	defer response.Body.Close()

	var body struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return GoogleToken{}, fmt.Errorf("đọc phản hồi token: %w", err)
	}

	// A revoked or wrong-client refresh token comes back as 400 with a reason.
	// Passing that reason on matters: "invalid_grant" means the user revoked
	// access, which no amount of retrying will fix.
	if body.Error != "" {
		detail := body.Error
		if body.ErrorDesc != "" {
			detail += ": " + body.ErrorDesc
		}
		return GoogleToken{}, fmt.Errorf("%w (%s)", ErrGoogleRefused, detail)
	}
	if response.StatusCode != http.StatusOK || body.AccessToken == "" {
		return GoogleToken{}, fmt.Errorf("%w: trả về %s mà không có access token", ErrGoogleRefused, response.Status)
	}

	lifetime := time.Duration(body.ExpiresIn) * time.Second
	if lifetime <= 0 {
		lifetime = time.Hour // Google's usual, when it says nothing
	}
	return GoogleToken{
		AccessToken: body.AccessToken,
		// Expire a minute early so a token is never handed to the CLI with
		// seconds left on it.
		ExpiresAt: time.Now().Add(lifetime - time.Minute),
	}, nil
}

// GoogleAccountImport is one entry of the refresh-token file a user exports from
// their own Google sign-ins.
type GoogleAccountImport struct {
	Email        string `json:"email"`
	RefreshToken string `json:"refresh_token"`
}

// ParseGoogleAccountsFile reads that file. Entries missing an email or a token
// are skipped rather than imported half-formed.
func ParseGoogleAccountsFile(data []byte) ([]GoogleAccountImport, error) {
	var entries []GoogleAccountImport
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("tệp tài khoản không phải JSON hợp lệ: %w", err)
	}

	valid := make([]GoogleAccountImport, 0, len(entries))
	for _, entry := range entries {
		entry.Email = strings.TrimSpace(entry.Email)
		entry.RefreshToken = strings.TrimSpace(entry.RefreshToken)
		if entry.Email == "" || entry.RefreshToken == "" {
			continue
		}
		valid = append(valid, entry)
	}
	if len(valid) == 0 {
		return nil, fmt.Errorf("không tìm thấy tài khoản hợp lệ nào trong tệp")
	}
	return valid, nil
}
