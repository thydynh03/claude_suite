package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

type OAuthListenerService struct {
	server *http.Server
	mu     sync.Mutex
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

func (s *OAuthListenerService) StartOAuthListener(clientID, clientSecret string, onSuccess func(email, accessToken, refreshToken string)) error {
	s.mu.Lock()
	if s.server != nil {
		_ = s.server.Shutdown(context.Background())
	}
	s.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "Thiếu mã xác thực (OAuth Code)", http.StatusBadRequest)
			return
		}

		// Exchange authorization code for OAuth access token
		resp, err := http.PostForm("https://oauth2.googleapis.com/token", url.Values{
			"code":          {code},
			"client_id":     {clientID},
			"client_secret": {clientSecret},
			"redirect_uri":  {"http://localhost:8045/auth/callback"},
			"grant_type":    {"authorization_code"},
		})

		var accessToken string
		var refreshToken string
		var userEmail string = "Account Google"

		if err == nil && resp.StatusCode == http.StatusOK {
			var tokResp TokenResponse
			if err := json.NewDecoder(resp.Body).Decode(&tokResp); err == nil {
				accessToken = tokResp.AccessToken
				refreshToken = tokResp.RefreshToken
			}
			resp.Body.Close()
		}

		// If exchange via secret was not available, code itself or direct bearer token can be passed
		if accessToken == "" {
			accessToken = code
		}

		// Fetch user profile email if access token starts with ya29
		if strings.HasPrefix(accessToken, "ya29") {
			req, _ := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
			req.Header.Set("Authorization", "Bearer "+accessToken)
			if uResp, err := http.DefaultClient.Do(req); err == nil && uResp.StatusCode == http.StatusOK {
				var uInfo UserInfoResponse
				if err := json.NewDecoder(uResp.Body).Decode(&uInfo); err == nil && uInfo.Email != "" {
					userEmail = uInfo.Email
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
	})

	s.server = &http.Server{
		Addr:    ":8045",
		Handler: mux,
	}

	go func() {
		_ = s.server.ListenAndServe()
	}()

	return nil
}
