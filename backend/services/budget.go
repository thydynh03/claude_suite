package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// BudgetGuard caps what the agents may spend in a day.
//
// This exists because the app can spend money without anyone watching: the
// orchestrator retries failed tasks on its own, and a scheduled job can start it
// at 2am. A bug in that retry path once ran a task ten times against a limit of
// three, each run a paid CLI invocation. A ceiling turns that class of fault
// into a stopped queue and a log line rather than a bill.
//
// The running total is written to disk. Keeping it in memory alone would mean
// restarting the app cleared the day's spend, which is not a cap so much as a
// suggestion.
type BudgetGuard struct {
	mu        sync.Mutex
	limitUSD  float64
	day       string // YYYY-MM-DD, so a new day starts fresh
	spentUSD  float64
	storePath string
	now       func() time.Time // swapped in tests to cross a day boundary
}

type budgetState struct {
	Day      string  `json:"day"`
	SpentUSD float64 `json:"spent_usd"`
	LimitUSD float64 `json:"limit_usd"`
}

func NewBudgetGuard() *BudgetGuard {
	return &BudgetGuard{now: time.Now}
}

// UseStore points the guard at a file and loads the total already spent today.
func (b *BudgetGuard) UseStore(path string) error {
	b.mu.Lock()
	b.storePath = path
	b.mu.Unlock()

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read budget: %w", err)
	}

	var state budgetState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("parse budget: %w", err)
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.limitUSD = state.LimitUSD
	// Yesterday's total does not carry over.
	if state.Day == b.today() {
		b.day, b.spentUSD = state.Day, state.SpentUSD
	}
	return nil
}

func (b *BudgetGuard) today() string {
	now := b.now
	if now == nil {
		now = time.Now
	}
	return now().Format("2006-01-02")
}

// rolloverLocked starts a new day when the date has changed.
func (b *BudgetGuard) rolloverLocked() {
	if today := b.today(); b.day != today {
		b.day, b.spentUSD = today, 0
	}
}

// SetLimit sets the daily ceiling in US dollars. Zero or less means no cap,
// which is the default: a limit nobody chose should not stop anyone's work.
func (b *BudgetGuard) SetLimit(usd float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.limitUSD = usd
	b.rolloverLocked()
	b.saveLocked()
}

// Record adds the cost of one completed run.
func (b *BudgetGuard) Record(costUSD float64) {
	if costUSD <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rolloverLocked()
	b.spentUSD += costUSD
	b.saveLocked()
}

// Allow reports whether more work may start, with the figures for the message
// shown when it may not.
func (b *BudgetGuard) Allow() (allowed bool, spent, limit float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rolloverLocked()
	if b.limitUSD <= 0 {
		return true, b.spentUSD, 0
	}
	return b.spentUSD < b.limitUSD, b.spentUSD, b.limitUSD
}

// Snapshot is what the UI shows: today, spent so far, and the ceiling.
func (b *BudgetGuard) Snapshot() (day string, spent, limit float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rolloverLocked()
	return b.day, b.spentUSD, b.limitUSD
}

func (b *BudgetGuard) saveLocked() {
	if b.storePath == "" {
		return
	}
	if b.day == "" {
		b.day = b.today()
	}
	data, err := json.MarshalIndent(budgetState{
		Day: b.day, SpentUSD: b.spentUSD, LimitUSD: b.limitUSD,
	}, "", "  ")
	if err != nil {
		return
	}
	_ = WriteFileAtomic(b.storePath, data, 0o600)
}
