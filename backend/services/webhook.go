package services

import (
	"encoding/json"
	"fmt"
	"net/http"

	"claude_suite/backend/database"
	"claude_suite/backend/models"
)

type WebhookService struct {
	server   *http.Server
	taskRepo *database.TaskRepository
	running  bool
}

type WebhookPayload struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
	Priority    string `json:"priority"`
}

func NewWebhookService(taskRepo *database.TaskRepository) *WebhookService {
	return &WebhookService{taskRepo: taskRepo}
}

func (w *WebhookService) Start(port int) error {
	if w.running {
		return nil
	}
	if port == 0 {
		port = 9090
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(rw, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var payload WebhookPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(rw, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if payload.Title == "" {
			payload.Title = "Webhook Task"
		}

		t := &models.Task{
			Title:       payload.Title,
			Description: payload.Description,
			Prompt:      payload.Prompt,
			Priority:    payload.Priority,
			Status:      "backlog",
		}
		if t.Priority == "" {
			t.Priority = "high"
		}

		if err := w.taskRepo.Create(t); err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(map[string]interface{}{
			"success": true,
			"task_id": t.TaskID,
		})
	})

	w.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	w.running = true
	go func() {
		_ = w.server.ListenAndServe()
		w.running = false
	}()

	return nil
}

func (w *WebhookService) Stop() error {
	if w.server != nil && w.running {
		w.running = false
		return w.server.Close()
	}
	return nil
}

func (w *WebhookService) IsRunning() bool {
	return w.running
}
