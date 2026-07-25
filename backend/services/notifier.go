package services

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"
)

// NotifierService sends outbound event notifications (task completion, etc.) to
// a user-configured webhook URL (Slack incoming webhook, Discord, or any custom
// HTTP endpoint that accepts JSON).
type NotifierService struct {
	client *http.Client
	url    string
}

func NewNotifierService() *NotifierService {
	return &NotifierService{client: &http.Client{Timeout: 8 * time.Second}}
}

func (n *NotifierService) SetWebhookURL(url string) {
	n.url = url
}

func (n *NotifierService) GetWebhookURL() string {
	return n.url
}

// NotifyTaskEvent posts a JSON event to the configured webhook URL, best-effort
// and non-blocking (call it in a goroutine). No-op if no URL is configured.
func (n *NotifierService) NotifyTaskEvent(event, taskTitle, status, detail string) {
	if n.url == "" {
		return
	}
	payload := map[string]string{
		"event":  event,
		"title":  taskTitle,
		"status": status,
		"detail": detail,
		"source": "Claude Suite",
		"time":   time.Now().Format(time.RFC3339),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, n.url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}
