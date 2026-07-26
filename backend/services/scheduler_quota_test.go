package services

import (
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// A due job that asked to wait for quota must stay due — not fire, not be
// consumed — until the gate opens, and then fire on the next tick.
func TestWaitForQuotaHoldsADueJobUntilTheGateOpens(t *testing.T) {
	s := NewSchedulerService()
	if err := s.UseStore(filepath.Join(t.TempDir(), "jobs.json")); err != nil {
		t.Fatal(err)
	}

	var runs atomic.Int32
	s.SetTriggerCallback(func(JobRequest) error {
		runs.Add(1)
		return nil
	})

	var gateOpen atomic.Bool
	s.SetQuotaGate(func(provider string) (bool, string) {
		if provider != "anti" {
			t.Errorf("gate asked about provider %q, want the job's own", provider)
		}
		return gateOpen.Load(), "đang cạn quota (test)"
	})

	id, err := s.ScheduleKind(JobKindPrompt, "overnight run", time.Now().Add(-time.Second).Format(time.RFC3339), false, "anti", "gemini-3.6-flash-high")
	if err != nil {
		t.Fatal(err)
	}
	if !s.SetJobWaitForQuota(id, true) {
		t.Fatal("SetJobWaitForQuota did not find the job")
	}

	// Gate closed: however many ticks pass, nothing fires and the one-shot
	// job is not consumed.
	for i := 0; i < 5; i++ {
		s.checkJobs()
	}
	if got := runs.Load(); got != 0 {
		t.Fatalf("job fired %d times through a closed gate", got)
	}
	for _, j := range s.GetJobs() {
		if j.ID == id && !j.Enabled {
			t.Fatal("the held one-shot job was consumed while waiting")
		}
	}

	// Gate opens: the very next tick fires it.
	gateOpen.Store(true)
	s.checkJobs()

	deadline := time.Now().Add(2 * time.Second)
	for runs.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("job never fired after the gate opened")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Jobs without the flag are untouched by the gate, whatever it reports.
func TestJobsWithoutWaitForQuotaIgnoreTheGate(t *testing.T) {
	s := NewSchedulerService()
	if err := s.UseStore(filepath.Join(t.TempDir(), "jobs.json")); err != nil {
		t.Fatal(err)
	}

	var runs atomic.Int32
	s.SetTriggerCallback(func(JobRequest) error {
		runs.Add(1)
		return nil
	})
	s.SetQuotaGate(func(string) (bool, string) { return false, "closed" })

	if _, err := s.ScheduleKind(JobKindPrompt, "now", time.Now().Add(-time.Second).Format(time.RFC3339), false, "claude", "claude-sonnet-4-5"); err != nil {
		t.Fatal(err)
	}
	s.checkJobs()

	deadline := time.Now().Add(2 * time.Second)
	for runs.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("an unflagged job was held by the quota gate")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// The flag survives a restart like every other part of a schedule.
func TestWaitForQuotaPersists(t *testing.T) {
	store := filepath.Join(t.TempDir(), "jobs.json")
	s := NewSchedulerService()
	if err := s.UseStore(store); err != nil {
		t.Fatal(err)
	}
	id, err := s.ScheduleKind(JobKindPrompt, "x", time.Now().Add(time.Hour).Format(time.RFC3339), false, "anti", "m")
	if err != nil {
		t.Fatal(err)
	}
	s.SetJobWaitForQuota(id, true)

	reloaded := NewSchedulerService()
	if err := reloaded.UseStore(store); err != nil {
		t.Fatal(err)
	}
	for _, j := range reloaded.GetJobs() {
		if j.ID == id {
			if !j.WaitForQuota {
				t.Fatal("wait_for_quota did not survive the reload")
			}
			return
		}
	}
	t.Fatal("job not found after reload")
}
