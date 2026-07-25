package services

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNoLimitMeansNoCeiling(t *testing.T) {
	guard := NewBudgetGuard()
	guard.Record(1000)

	if allowed, _, _ := guard.Allow(); !allowed {
		t.Error("work was blocked with no limit set")
	}
}

func TestSpendingUpToTheLimitStopsFurtherWork(t *testing.T) {
	guard := NewBudgetGuard()
	guard.SetLimit(1.00)

	guard.Record(0.60)
	if allowed, _, _ := guard.Allow(); !allowed {
		t.Error("blocked while still under the limit")
	}

	guard.Record(0.45) // now 1.05, over
	allowed, spent, limit := guard.Allow()
	if allowed {
		t.Error("work was allowed after the limit was passed")
	}
	if spent < 1.04 || limit != 1.00 {
		t.Errorf("reported spent=%v limit=%v, want about 1.05 and 1.00", spent, limit)
	}
}

func TestFreeRunsDoNotCountAgainstTheBudget(t *testing.T) {
	guard := NewBudgetGuard()
	guard.SetLimit(1.00)

	// Antigravity often reports no cost at all; recording zero must not move the
	// total, or a hundred free runs would exhaust a budget nothing was spent on.
	for i := 0; i < 100; i++ {
		guard.Record(0)
	}

	if _, spent, _ := guard.Allow(); spent != 0 {
		t.Errorf("spent = %v after only free runs, want 0", spent)
	}
}

// A cap that a restart clears is not a cap. The total has to be on disk.
func TestTodaysSpendSurvivesARestart(t *testing.T) {
	store := filepath.Join(t.TempDir(), "budget.json")

	first := NewBudgetGuard()
	if err := first.UseStore(store); err != nil {
		t.Fatalf("use store: %v", err)
	}
	first.SetLimit(2.00)
	first.Record(1.50)

	second := NewBudgetGuard()
	if err := second.UseStore(store); err != nil {
		t.Fatalf("reload: %v", err)
	}

	_, spent, limit := second.Snapshot()
	if spent < 1.49 || spent > 1.51 {
		t.Errorf("spent = %v after restart, want 1.50", spent)
	}
	if limit != 2.00 {
		t.Errorf("limit = %v after restart, want 2.00", limit)
	}
}

// Yesterday's spending must not hold today's work hostage.
func TestANewDayStartsFromZero(t *testing.T) {
	store := filepath.Join(t.TempDir(), "budget.json")
	day := time.Date(2026, 7, 25, 23, 0, 0, 0, time.UTC)

	guard := NewBudgetGuard()
	guard.now = func() time.Time { return day }
	if err := guard.UseStore(store); err != nil {
		t.Fatalf("use store: %v", err)
	}
	guard.SetLimit(1.00)
	guard.Record(1.20) // past the ceiling

	if allowed, _, _ := guard.Allow(); allowed {
		t.Fatal("expected to be at the ceiling before midnight")
	}

	guard.now = func() time.Time { return day.Add(2 * time.Hour) } // just past midnight

	allowed, spent, _ := guard.Allow()
	if !allowed {
		t.Error("still blocked on the new day")
	}
	if spent != 0 {
		t.Errorf("spent = %v on the new day, want 0", spent)
	}
}

// Reopening on a later day must not read yesterday's total back in.
func TestAStoredTotalFromYesterdayIsIgnored(t *testing.T) {
	store := filepath.Join(t.TempDir(), "budget.json")
	yesterday := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)

	first := NewBudgetGuard()
	first.now = func() time.Time { return yesterday }
	first.UseStore(store)
	first.SetLimit(1.00)
	first.Record(1.20)

	second := NewBudgetGuard()
	second.now = func() time.Time { return yesterday.Add(24 * time.Hour) }
	if err := second.UseStore(store); err != nil {
		t.Fatalf("reload: %v", err)
	}

	allowed, spent, limit := second.Allow()
	if !allowed {
		t.Error("today was blocked by yesterday's spending")
	}
	if spent != 0 {
		t.Errorf("spent = %v, want 0 on a new day", spent)
	}
	if limit != 1.00 {
		t.Errorf("limit = %v, want the configured 1.00 to persist", limit)
	}
}
