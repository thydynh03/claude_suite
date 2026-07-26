package cli

import (
	"strings"
	"testing"
	"time"
)

func TestQuotaReadyWithAnActiveAccount(t *testing.T) {
	pool := poolWith(active("a"))
	if ok, _ := pool.QuotaReady(QuotaCooldown); !ok {
		t.Fatal("an active account should make the pool ready")
	}
}

// The reason WaitForQuota exists: every account measured rate-limited
// recently means a run now would just burn a retry into a wall.
func TestQuotaReadyHoldsWhileEveryAccountIsFreshlyLimited(t *testing.T) {
	k := active("a")
	k.Status = "rate_limited_429"
	k.Usage.LastRateLimitAt = time.Now().Add(-1 * time.Hour)
	pool := poolWith(k)

	ok, reason := pool.QuotaReady(QuotaCooldown)
	if ok {
		t.Fatal("a pool rate-limited an hour ago is not ready under a 24h cooldown")
	}
	if !strings.Contains(reason, "hồi lúc") {
		t.Errorf("reason should say when quota is expected back, got %q", reason)
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

// A rate-limit mark with no measured timestamp cannot schedule anything;
// holding a job forever on data nobody recorded would be worse than firing.
func TestQuotaReadyIgnoresUnmeasuredRateLimitMarks(t *testing.T) {
	k := active("a")
	k.Status = "rate_limited_429"
	pool := poolWith(k)

	if ok, _ := pool.QuotaReady(QuotaCooldown); !ok {
		t.Fatal("an unmeasured rate-limit mark must not hold the gate")
	}
}

// An empty pool means environment auth, about which nothing is measured.
func TestQuotaReadyOnAnEmptyPool(t *testing.T) {
	if ok, _ := (&AccountKeyPool{}).QuotaReady(QuotaCooldown); !ok {
		t.Fatal("an empty pool must be ready — the CLI authenticates from the environment")
	}
}
