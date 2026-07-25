package services

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"

	"claude_suite/backend/database"
	"claude_suite/backend/models"
)

type WebhookService struct {
	taskRepo *database.TaskRepository

	// running and server are read by the UI thread and written by the serve
	// goroutine, so every access goes through mu.
	mu      sync.Mutex
	server  *http.Server
	running bool
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
	w.mu.Lock()
	defer w.mu.Unlock()
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

		r.Body = http.MaxBytesReader(rw, r.Body, 1048576) // 1MB limit

		var payload WebhookPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(rw, "Invalid JSON or body size limit exceeded", http.StatusBadRequest)
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

	// Bind before returning. ListenAndServe inside the goroutine reported success
	// even when the port was already taken, so the UI showed the webhook as
	// listening while nothing was bound.
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("webhook cannot listen on port %d: %w", port, err)
	}

	server := &http.Server{Handler: mux}
	w.server = server
	w.running = true
	go func() {
		_ = server.Serve(listener)
		w.mu.Lock()
		w.running = false
		w.mu.Unlock()
	}()

	return nil
}

func (w *WebhookService) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.server != nil && w.running {
		w.running = false
		return w.server.Close()
	}
	return nil
}

func (w *WebhookService) IsRunning() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}
