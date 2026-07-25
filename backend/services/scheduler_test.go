package services

import (
	"testing"
	"time"
)

func TestSchedulerService_SchedulePrompt(t *testing.T) {
	svc := NewSchedulerService()
	svc.Start()
	defer svc.Stop()

	target := time.Now().Add(10 * time.Minute).Format(time.RFC3339)
	id, err := svc.SchedulePrompt("Test automated backup prompt", target, false)
	if err != nil {
		t.Fatalf("unexpected error scheduling prompt: %v", err)
	}

	if id == "" {
		t.Errorf("expected non-empty job ID")
	}

	jobs := svc.GetJobs()
	if len(jobs) != 1 {
		t.Errorf("expected 1 scheduled job, got %d", len(jobs))
	}

	svc.CancelJob(id)
	if len(svc.GetJobs()) != 0 {
		t.Errorf("expected 0 jobs after cancel, got %d", len(svc.GetJobs()))
	}
}

// Start and Stop are guarded the same way the orchestrator's are: Stop closes a
// channel, so closing it twice would panic, and a restart has to build a fresh
// one or the new loop returns at once on the closed one.
func TestSchedulerStartStopLifecycle(t *testing.T) {
	s := NewSchedulerService()

	s.Stop() // before any start
	s.Start()
	s.Start() // a second press must not start a second loop
	s.Stop()
	s.Stop() // must not panic on the closed channel

	// Restarting has to work, and the new loop must still be running afterwards.
	s.Start()
	defer s.Stop()

	fired := make(chan string, 1)
	s.SetTriggerCallback(func(prompt string) { fired <- prompt })

	if _, err := s.SchedulePrompt("run me", time.Now().Add(-time.Second).Format(time.RFC3339), false); err != nil {
		t.Fatalf("schedule: %v", err)
	}

	select {
	case got := <-fired:
		if got != "run me" {
			t.Errorf("triggered with %q, want %q", got, "run me")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the restarted loop never fired the due job")
	}
}
