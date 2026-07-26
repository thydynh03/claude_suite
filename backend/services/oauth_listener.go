package services

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type OAuthListenerService struct {
	server *http.Server
	mu     sync.Mutex
	// state is the nonce the next callback must present, single-use. The
	// callback endpoint takes unauthenticated input and turns it into a
	// stored credential; without this, any page the user's browser visits
	// (or any process that can reach the port) could complete a login the
	// user never started.
	state string
}

func NewOAuthListenerService() *OAuthListenerService {
	return &OAuthListenerService{}
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

type UserInfoResponse struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// oauthHTTPClient bounds the token exchange; without it the callback handler
// sat blocked on Google for as long as the network cared to hold the socket.
var oauthHTTPClient = &http.Client{Timeout: 30 * time.Second}

// StartOAuthListener serves the Google sign-in callback and returns the state
// nonce the caller must embed in the auth URL as &state=.
//
// The listener binds 127.0.0.1 only and synchronously: the flow hands a
// credential to whoever answers this port, so it must not face the LAN, and a
// taken port must fail here — ListenAndServe in a goroutine reported success
// while Google's redirect landed on connection-refused (the same defect the
// webhook service fixed; this was the remaining copy).
func (s *OAuthListenerService) StartOAuthListener(clientID, clientSecret string, onSuccess func(email, accessToken, refreshToken string)) (string, error) {
	stateBytes := make([]byte, 24)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", err
	}
	state := hex.EncodeToString(stateBytes)

	s.mu.Lock()
	if s.server != nil {
		_ = s.server.Shutdown(context.Background())
		s.server = nil
	}
	s.state = state
	s.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		expected := s.state
		s.mu.Unlock()
		got := r.URL.Query().Get("state")
		if expected == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(got)) != 1 {
			http.Error(w, "Sai hoặc thiếu state — hãy bấm 'Đăng nhập Google' trong app rồi thử lại.", http.StatusForbidden)
			return
		}
		// Single use: a replayed redirect must not mint a second credential.
		s.mu.Lock()
		s.state = ""
		s.mu.Unlock()

		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "Thiếu mã xác thực (OAuth Code)", http.StatusBadRequest)
			return
		}

		// Exchange authorization code for OAuth access token
		resp, err := oauthHTTPClient.PostForm("https://oauth2.googleapis.com/token", url.Values{
			"code":          {code},
			"client_id":     {clientID},
			"client_secret": {clientSecret},
			"redirect_uri":  {"http://localhost:8045/auth/callback"},
			"grant_type":    {"authorization_code"},
		})

		var accessToken string
		var refreshToken string
		var userEmail string = "Account Google"

		if err == nil {
			if resp.StatusCode == http.StatusOK {
				var tokResp TokenResponse
				if err := json.NewDecoder(resp.Body).Decode(&tokResp); err == nil {
					accessToken = tokResp.AccessToken
					refreshToken = tokResp.RefreshToken
				}
			}
			// Closed on every status: the non-200 path used to leak the body
			// and its connection for the life of the process.
			resp.Body.Close()
		}

		// No fallback to the raw code: whatever failed the exchange, the
		// query text is not a credential. Storing it minted a "Google"
		// account out of unauthenticated input.
		if accessToken == "" {
			http.Error(w, "Đổi mã xác thực thất bại — thử đăng nhập lại từ app.", http.StatusBadGateway)
			return
		}

		// Fetch user profile email if access token starts with ya29
		if strings.HasPrefix(accessToken, "ya29") {
			req, _ := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
			req.Header.Set("Authorization", "Bearer "+accessToken)
			if uResp, err := oauthHTTPClient.Do(req); err == nil {
				if uResp.StatusCode == http.StatusOK {
					var uInfo UserInfoResponse
					if err := json.NewDecoder(uResp.Body).Decode(&uInfo); err == nil && uInfo.Email != "" {
						userEmail = uInfo.Email
					}
				}
				uResp.Body.Close()
			}
		}

		// We no longer add a temporary AccessToken to GlobalAntiPool here.
		// We pass both to onSuccess, where the app will save the RefreshToken persistently.

		if onSuccess != nil {
			onSuccess(userEmail, accessToken, refreshToken)
		}

		// Return clean HTML response to browser
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `
			<!DOCTYPE html>
			<html lang="vi">
			<head>
				<meta charset="UTF-8">
				<title>Đăng nhập thành công</title>
				<style>
					body { font-family: system-ui, -apple-system, sans-serif; background: #0f172a; color: #f8fafc; display: flex; align-items: center; justify-content: center; height: 100vh; margin: 0; }
					.card { background: #1e293b; padding: 2.5rem; border-radius: 1.5rem; box-shadow: 0 25px 50px -12px rgba(0,0,0,0.5); text-align: center; max-width: 420px; border: 1px solid #334155; }
					.icon { font-size: 4rem; margin-bottom: 1rem; }
					h1 { font-size: 1.5rem; color: #10b981; margin-bottom: 0.5rem; }
					p { color: #94a3b8; font-size: 0.95rem; line-height: 1.5; }
					.badge { background: #064e3b; color: #34d399; padding: 0.4rem 0.8rem; border-radius: 9999px; font-weight: bold; font-size: 0.85rem; display: inline-block; margin-top: 1rem; }
				</style>
			</head>
			<body>
				<div class="card">
					<div class="icon">🎉</div>
					<h1>Tự Động Lưu Key Thành Công!</h1>
					<p>Tài khoản <strong>%s</strong> đã được xác thực và tự động lưu vào <strong>Claude Suite Control Center</strong>.</p>
					<div class="badge">✓ Bạn có thể đóng cửa sổ trình duyệt này</div>
				</div>
			</body>
			</html>
		`, userEmail)

		// One callback per listener: with the credential delivered, the port
		// does not stay open for the rest of the session.
		s.mu.Lock()
		server := s.server
		s.server = nil
		s.mu.Unlock()
		if server != nil {
			go func() { _ = server.Shutdown(context.Background()) }()
		}
	})

	// Loopback only, bound before returning.
	listener, err := net.Listen("tcp", "127.0.0.1:8045")
	if err != nil {
		return "", fmt.Errorf("cổng 8045 đang bị chiếm (app khác hoặc phiên đăng nhập cũ?): %w", err)
	}

	server := &http.Server{Handler: mux}
	s.mu.Lock()
	s.server = server
	s.mu.Unlock()

	go func() {
		_ = server.Serve(listener)
	}()

	return state, nil
}
