package services

import (
	"context"
	"fmt"
	"sync/atomic"
)

// BrowserAskOption is one clickable choice rendered in the live log.
type BrowserAskOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// BrowserAskRequest is emitted to the frontend when the agent needs a decision.
// The agent goroutine parks on a channel while this is on screen — it is never
// torn down — so whatever the user picks flows straight back into the ReAct loop.
type BrowserAskRequest struct {
	ID         string             `json:"id"`
	Question   string             `json:"question"`
	Options    []BrowserAskOption `json:"options"`
	AllowOther bool               `json:"allow_other"`
}

// BrowserEventEmitter bridges the service to the Wails event bus.
type BrowserEventEmitter func(event string, payload map[string]interface{})

var askSeq atomic.Uint64

// SetEventEmitter wires the service to the frontend. Without it, AskUser reports
// that no UI is attached and callers fall back to ending the run.
func (s *BrowserAgentService) SetEventEmitter(emit BrowserEventEmitter) {
	s.askMu.Lock()
	defer s.askMu.Unlock()
	s.emit = emit
}

func (s *BrowserAgentService) emitEvent(event string, payload map[string]interface{}) {
	s.askMu.Lock()
	emit := s.emit
	s.askMu.Unlock()
	if emit != nil {
		emit(event, payload)
	}
}

// AskUser renders a choice form in the live log and blocks until the user answers
// or the task is cancelled. The returned string is the chosen option's Value, or
// the raw text the user typed into "Khác".
func (s *BrowserAgentService) AskUser(ctx context.Context, question string, options []BrowserAskOption) (string, error) {
	s.askMu.Lock()
	emit := s.emit
	s.askMu.Unlock()
	if emit == nil {
		return "", fmt.Errorf("chưa gắn giao diện hỏi người dùng")
	}

	req := BrowserAskRequest{
		ID:         fmt.Sprintf("ask-%d", askSeq.Add(1)),
		Question:   question,
		Options:    options,
		AllowOther: true,
	}

	ch := make(chan string, 1)
	s.askMu.Lock()
	if s.askChans == nil {
		s.askChans = make(map[string]chan string)
	}
	s.askChans[req.ID] = ch
	s.askMu.Unlock()

	defer func() {
		s.askMu.Lock()
		delete(s.askChans, req.ID)
		s.askMu.Unlock()
	}()

	opts := make([]map[string]string, 0, len(options))
	for _, o := range options {
		opts = append(opts, map[string]string{"label": o.Label, "value": o.Value})
	}
	emit("browser_ask_user", map[string]interface{}{
		"id":          req.ID,
		"question":    req.Question,
		"options":     opts,
		"allow_other": req.AllowOther,
	})

	select {
	case answer := <-ch:
		s.emitEvent("browser_ask_close", map[string]interface{}{"id": req.ID})
		return answer, nil
	case <-ctx.Done():
		s.emitEvent("browser_ask_close", map[string]interface{}{"id": req.ID})
		return "", ctx.Err()
	}
}

// ResolveAsk delivers the user's answer to the parked agent goroutine.
func (s *BrowserAgentService) ResolveAsk(id, answer string) bool {
	s.askMu.Lock()
	ch, ok := s.askChans[id]
	s.askMu.Unlock()
	if !ok {
		return false
	}
	select {
	case ch <- answer:
		return true
	default:
		return false
	}
}

// cancelPendingAsks closes any open form so the UI does not strand a dead prompt
// when the run is stopped.
func (s *BrowserAgentService) cancelPendingAsks() {
	s.askMu.Lock()
	ids := make([]string, 0, len(s.askChans))
	for id := range s.askChans {
		ids = append(ids, id)
	}
	s.askMu.Unlock()
	for _, id := range ids {
		s.emitEvent("browser_ask_close", map[string]interface{}{"id": id})
	}
}
