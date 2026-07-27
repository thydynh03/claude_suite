package services

import (
	"strings"
	"testing"
)

// Two runs at once share one Chrome through one profile, so when the first
// finishes its cleanup tears down the CDP connection under the second — which
// then fails every remaining step with "context canceled". The second run is
// refused instead.
func TestASecondRunIsRefusedWhileOneIsInFlight(t *testing.T) {
	s := NewBrowserAgentService()

	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	result, err := s.RunAutonomousBrowserTask(
		"https://example.com", "làm gì đó", "", "gemini", false, true, false, 3, nil, nil, nil, false)

	if err == nil {
		t.Fatal("a second concurrent run was accepted")
	}
	if result == nil || result.Success {
		t.Fatalf("result = %+v, want an unsuccessful one", result)
	}
	if !strings.Contains(result.Error, "đang chạy") {
		t.Errorf("error = %q, want it to say a run is already in flight", result.Error)
	}
	if result.Status != "failed" {
		t.Errorf("status = %q, want failed", result.Status)
	}
}

// The refusal path must not touch the in-flight run's state. Clearing it here
// would disarm Stop for the run that is actually going, which is the same class
// of bug as the unconditional cleanup this replaced.
func TestRefusingASecondRunLeavesTheFirstRunAlone(t *testing.T) {
	s := NewBrowserAgentService()

	firstCancelled := false
	s.mu.Lock()
	s.running = true
	s.cancelFunc = func() { firstCancelled = true }
	s.mu.Unlock()

	_, _ = s.RunAutonomousBrowserTask(
		"https://example.com", "x", "", "gemini", false, true, false, 1, nil, nil, nil, false)

	s.mu.Lock()
	stillRunning := s.running
	stillArmed := s.cancelFunc != nil
	s.mu.Unlock()

	if !stillRunning {
		t.Error("the refused run cleared the in-flight run's guard")
	}
	if !stillArmed {
		t.Error("the refused run disarmed Stop for the in-flight run")
	}
	if firstCancelled {
		t.Error("the refused run cancelled the in-flight run")
	}
}
