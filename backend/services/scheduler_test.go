package services

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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
	s.SetTriggerCallback(func(job JobRequest) error { fired <- job.Prompt; return nil })

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

// The fatal flaw this replaced: jobs lived in a map and nothing else, so setting
// up a nightly run and closing the app threw the schedule away without a word.
func TestScheduledJobsSurviveARestart(t *testing.T) {
	store := filepath.Join(t.TempDir(), "scheduled_jobs.json")

	first := NewSchedulerService()
	if err := first.UseStore(store); err != nil {
		t.Fatalf("use store: %v", err)
	}
	id, err := first.ScheduleJob(
		"nightly review", time.Now().Add(time.Hour).Format(time.RFC3339),
		true, "anti", "gemini-3.6-flash-high",
	)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}

	// A new process, reading the same file.
	second := NewSchedulerService()
	if err := second.UseStore(store); err != nil {
		t.Fatalf("reload: %v", err)
	}

	jobs := second.GetJobs()
	if len(jobs) != 1 {
		t.Fatalf("reloaded %d jobs, want 1", len(jobs))
	}
	got := jobs[0]
	if got.ID != id {
		t.Errorf("id = %q, want %q", got.ID, id)
	}
	if got.Prompt != "nightly review" || !got.Repeat {
		t.Errorf("job came back as %+v", got)
	}
	if got.Provider != "anti" || got.Model != "gemini-3.6-flash-high" {
		t.Errorf("provider/model = %q/%q, want the ones it was scheduled with", got.Provider, got.Model)
	}
}

func TestCancelledJobStaysCancelledAfterARestart(t *testing.T) {
	store := filepath.Join(t.TempDir(), "scheduled_jobs.json")

	first := NewSchedulerService()
	first.UseStore(store)
	id, err := first.ScheduleJob("gone", time.Now().Add(time.Hour).Format(time.RFC3339), false, "", "")
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	first.CancelJob(id)

	second := NewSchedulerService()
	second.UseStore(store)
	if jobs := second.GetJobs(); len(jobs) != 0 {
		t.Errorf("a cancelled job came back: %+v", jobs)
	}
}

// A job that fails has to say so on the job itself, or a nightly run can be
// broken for a week and still look untouched.
func TestAFailedRunIsRecordedOnTheJob(t *testing.T) {
	store := filepath.Join(t.TempDir(), "scheduled_jobs.json")
	s := NewSchedulerService()
	s.UseStore(store)

	done := make(chan struct{})
	var once sync.Once
	s.SetTriggerCallback(func(JobRequest) error {
		once.Do(func() { close(done) })
		return errors.New("the CLI was not found")
	})

	if _, err := s.ScheduleJob("doomed", time.Now().Add(-time.Second).Format(time.RFC3339), false, "", ""); err != nil {
		t.Fatalf("schedule: %v", err)
	}
	s.Start()
	defer s.Stop()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the job never ran")
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(store); err == nil && strings.Contains(string(data), "the CLI was not found") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("the failure was never recorded against the job")
}

// A daily job left behind while the app was closed must not fire once per missed
// day when it starts again.
func TestARepeatingJobSkipsMissedOccurrences(t *testing.T) {
	s := NewSchedulerService()

	var runs int32
	s.SetTriggerCallback(func(JobRequest) error {
		atomic.AddInt32(&runs, 1)
		return nil
	})

	// Due five days ago, repeating daily.
	if _, err := s.ScheduleJob("catch up", time.Now().Add(-5*24*time.Hour).Format(time.RFC3339), true, "", ""); err != nil {
		t.Fatalf("schedule: %v", err)
	}
	s.Start()
	defer s.Stop()

	time.Sleep(2500 * time.Millisecond)

	if got := atomic.LoadInt32(&runs); got != 1 {
		t.Errorf("the job ran %d times for five missed days, want once", got)
	}
	jobs := s.GetJobs()
	if len(jobs) != 1 || !jobs[0].TargetTime.After(time.Now()) {
		t.Errorf("next run is not in the future: %+v", jobs)
	}
}

// An unknown kind is rejected when it is scheduled, not at midnight when nobody
// is watching it fail.
func TestScheduleKindRejectsAnUnknownKind(t *testing.T) {
	s := NewSchedulerService()
	when := time.Now().Add(time.Hour).Format(time.RFC3339)

	if _, err := s.ScheduleKind("deploy-to-prod", "go", when, false, "", ""); err == nil {
		t.Error("an unknown kind was accepted")
	}
	if jobs := s.GetJobs(); len(jobs) != 0 {
		t.Errorf("the rejected job was still stored: %+v", jobs)
	}
}

func TestScheduleKindStoresAndReloadsTheKind(t *testing.T) {
	store := filepath.Join(t.TempDir(), "jobs.json")
	first := NewSchedulerService()
	first.UseStore(store)

	when := time.Now().Add(time.Hour).Format(time.RFC3339)
	if _, err := first.ScheduleKind(JobKindE2E, "check the login page", when, true, "", ""); err != nil {
		t.Fatalf("schedule: %v", err)
	}

	second := NewSchedulerService()
	second.UseStore(store)
	jobs := second.GetJobs()
	if len(jobs) != 1 || jobs[0].Kind != JobKindE2E {
		t.Errorf("kind did not survive the restart: %+v", jobs)
	}
}

// A job scheduled before kinds existed has none, and must keep running as the
// prompt it always was rather than being refused.
func TestABlankKindIsAPlainPrompt(t *testing.T) {
	s := NewSchedulerService()
	when := time.Now().Add(time.Hour).Format(time.RFC3339)

	id, err := s.ScheduleKind("", "just ask", when, false, "", "")
	if err != nil {
		t.Fatalf("a blank kind was rejected: %v", err)
	}
	for _, job := range s.GetJobs() {
		if job.ID == id && job.Kind != JobKindPrompt {
			t.Errorf("kind = %q, want %q", job.Kind, JobKindPrompt)
		}
	}
}

// The kind reaches the callback; without it every job would run as a prompt.
func TestTheKindReachesTheTrigger(t *testing.T) {
	s := NewSchedulerService()

	seen := make(chan string, 1)
	s.SetTriggerCallback(func(job JobRequest) error {
		seen <- job.Kind
		return nil
	})

	if _, err := s.ScheduleKind(JobKindDigest, "", time.Now().Add(-time.Second).Format(time.RFC3339), false, "", ""); err != nil {
		t.Fatalf("schedule: %v", err)
	}
	s.Start()
	defer s.Stop()

	select {
	case got := <-seen:
		if got != JobKindDigest {
			t.Errorf("the trigger saw kind %q, want %q", got, JobKindDigest)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the job never ran")
	}
}
