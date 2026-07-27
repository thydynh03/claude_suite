package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// NotifierService sends outbound event notifications (task completion, etc.) to
// a user-configured webhook URL (Slack incoming webhook, Discord, or any custom
// HTTP endpoint that accepts JSON).
type NotifierService struct {
	client *http.Client

	// url is written from the settings UI while worker goroutines are firing
	// notifications, so it is guarded rather than raced on.
	mu      sync.RWMutex
	url     string
	onError func(msg string)
}

func NewNotifierService() *NotifierService {
	return &NotifierService{client: &http.Client{Timeout: 8 * time.Second}}
}

func (n *NotifierService) SetWebhookURL(url string) {
	n.mu.Lock()
	n.url = url
	n.mu.Unlock()
}

func (n *NotifierService) GetWebhookURL() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.url
}

// SetErrorHandler routes delivery failures somewhere the user can see them.
// Without it a rejected webhook was completely silent: the Settings page
// advertises Slack and Discord, both of which reject an unknown body with a
// 400, and nothing anywhere said so.
func (n *NotifierService) SetErrorHandler(fn func(msg string)) {
	n.mu.Lock()
	n.onError = fn
	n.mu.Unlock()
}

func (n *NotifierService) reportError(format string, args ...interface{}) {
	n.mu.RLock()
	fn := n.onError
	n.mu.RUnlock()
	if fn != nil {
		fn(fmt.Sprintf(format, args...))
	}
}

// NotifyTaskEvent posts a JSON event to the configured webhook URL, best-effort
// and non-blocking (call it in a goroutine). No-op if no URL is configured.
//
// The body carries "text" and "content" beside the structured fields: Slack
// requires "text", Discord requires "content", and both ignore extra keys — so
// one payload serves Slack, Discord and custom endpoints, where the structured
// -only version was rejected 400 by the two services the UI recommends.
func (n *NotifierService) NotifyTaskEvent(event, taskTitle, status, detail string) {
	n.mu.RLock()
	url := n.url
	n.mu.RUnlock()
	if url == "" {
		return
	}
	summary := fmt.Sprintf("[Agent Center] %s: %s", event, taskTitle)
	if status != "" {
		summary += " (" + status + ")"
	}
	if detail != "" {
		summary += "\n" + detail
	}
	payload := map[string]string{
		"text":    summary,
		"content": summary,
		"event":   event,
		"title":   taskTitle,
		"status":  status,
		"detail":  detail,
		"source":  "Agent Center",
		"time":    time.Now().Format(time.RFC3339),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		n.reportError("Không tạo được nội dung webhook: %v", err)
		return
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		n.reportError("URL webhook không hợp lệ: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		n.reportError("Không gửi được webhook: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		n.reportError("Webhook trả về HTTP %d — kiểm tra lại URL trong Cài đặt", resp.StatusCode)
	}
}
