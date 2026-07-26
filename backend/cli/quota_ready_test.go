package cli

import (
	"strings"
	"testing"
	"time"
)

func TestQuotaReadyWithAnUntouchedAccount(t *testing.T) {
	pool := poolWith(active("a"))
	if ok, _ := pool.QuotaReady(QuotaCooldown); !ok {
		t.Fatal("an account with no measured rate-limit should make the pool ready")
	}
}

// The gate judges by the measured timestamp, never Status: rotation leaves
// whichever account it lands on marked "active" while its minutes-old 429
// persists, so a status-based gate read a fully exhausted pool as open.
// This drives the REAL rotation sequence and expects the gate closed.
func TestQuotaReadyClosesAfterRealRotationExhaustsThePool(t *testing.T) {
	pool := poolWith(active("a"), active("b"))

	// Both accounts 429 in sequence, exactly as executeWithRotation does it.
	pool.RotateNextKey("a") // marks a, lands on b (Status becomes "active")
	pool.RotateNextKey("b") // marks b, lands on a (Status becomes "active")

	ok, reason := pool.QuotaReady(QuotaCooldown)
	if ok {
		t.Fatalf("pool exhausted by real rotation still reads ready (%s) — the gate is judging Status, not measurements", reason)
	}
	if !strings.Contains(reason, "hồi lúc") {
		t.Errorf("reason should say when quota is expected back, got %q", reason)
	}
}

// A single-account pool has nowhere to rotate, but its 429 is still measured
// — without the recording, the gate can never hold a job for the most common
// setup of all.
func TestQuotaReadyClosesForASingleAccountAfterA429(t *testing.T) {
	pool := poolWith(active("only"))

	pool.RotateNextKey("only")

	if ok, _ := pool.QuotaReady(QuotaCooldown); ok {
		t.Fatal("single-account pool reads ready right after its measured 429")
	}
	if hits := pool.GetKeys()[0].Usage.RateLimitHits; hits != 1 {
		t.Fatalf("rate_limit_hits = %d, want the 429 recorded even with nowhere to rotate", hits)
	}
}

// A measured 429 older than the cooldown is a quota cycle ago — the account
// is presumed reset, which is the whole scheduling trick.
func TestQuotaReadyReopensAfterTheCooldown(t *testing.T) {
	k := active("a")
	k.Status = "rate_limited_429"
	k.Usage.LastRateLimitAt = time.Now().Add(-25 * time.Hour)
	pool := poolWith(k)

	if ok, _ := pool.QuotaReady(QuotaCooldown); !ok {
		t.Fatal("a 429 older than the cooldown should count as reset")
	}
}

// Disabling an account was deliberate; the gate must not resurrect it.
func TestQuotaReadyNeverCountsDisabledAccounts(t *testing.T) {
	pool := poolWith(disabled("a"))
	ok, reason := pool.QuotaReady(QuotaCooldown)
	if ok {
		t.Fatal("a fully disabled pool must not be ready")
	}
	if !strings.Contains(reason, "tắt") {
		t.Errorf("reason should say the pool is disabled, got %q", reason)
	}
}

// An empty pool means environment auth, about which nothing is measured.
func TestQuotaReadyOnAnEmptyPool(t *testing.T) {
	if ok, _ := (&AccountKeyPool{}).QuotaReady(QuotaCooldown); !ok {
		t.Fatal("an empty pool must be ready — the CLI authenticates from the environment")
	}
}
