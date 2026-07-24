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
