package cli

import (
	"testing"
	"time"
)

// The reading Google sends is "how much is left"; the table renders "how much
// is used". Getting that inversion wrong would show a fresh account as spent.
func TestQuotaFromFractionInvertsRemainingIntoUsed(t *testing.T) {
	cases := []struct {
		frac   float64
		want   int
		status string
	}{
		{1, 0, "ok"},
		{0.5, 50, "ok"},
		{0.19, 81, "warning"},
		{0, 100, "exceeded"},
	}
	for _, c := range cases {
		got := QuotaFromFraction("M", "gemini", c.frac, false, time.Hour)
		if got.UsagePct != c.want || got.Status != c.status {
			t.Errorf("frac %v -> usage %d/%s, want %d/%s", c.frac, got.UsagePct, got.Status, c.want, c.status)
		}
	}
}

// -1 is "Google said nothing", and it must not be arithmetic'd into 200%.
func TestQuotaFromFractionKeepsNoDataUnknown(t *testing.T) {
	got := QuotaFromFraction("M", "gemini", -1, false, time.Hour)
	if got.UsagePct != -1 || got.Status != "unknown" {
		t.Errorf("got usage %d status %q, want -1/unknown", got.UsagePct, got.Status)
	}
	if got.ResetTime != "" {
		t.Errorf("reset time %q on a model with no data", got.ResetTime)
	}
}

// An account flagged exhausted is exceeded even if the fraction rounds above 0.
func TestExhaustedBeatsTheFraction(t *testing.T) {
	got := QuotaFromFraction("M", "gemini", 0.4, true, 0)
	if got.Status != "exceeded" {
		t.Errorf("status = %q, want exceeded", got.Status)
	}
}

func TestResetCountdownReadsAsATime(t *testing.T) {
	cases := map[time.Duration]string{
		30 * time.Hour:               "1d 6h",
		2*time.Hour + 15*time.Minute: "2h 15m",
		8 * time.Minute:              "8m",
		-time.Hour:                   "",
	}
	for d, want := range cases {
		if got := humanDuration(d); got != want {
			t.Errorf("humanDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

// A tier the user vouched for is evidence. An empty answer from Google is not,
// and must not be allowed to wipe it.
func TestAFailedFetchDoesNotEraseATierTheUserSet(t *testing.T) {
	pool := &AccountKeyPool{}
	pool.AddKey("A", "key-a")
	id := pool.GetKeys()[0].ID
	pool.SetTier(id, "ULTRA")

	if !pool.ApplyQuota(id, "", "", "", nil, "Google từ chối") {
		t.Fatal("ApplyQuota returned false for a valid id")
	}

	got := pool.GetKeys()[0]
	if got.Tier != "ULTRA" || got.TierSource != "user" {
		t.Errorf("tier = %q from %q, want ULTRA from user", got.Tier, got.TierSource)
	}
	if got.QuotaError == "" {
		t.Error("the reason the fetch failed was not kept")
	}
}

// A tier Google gave outranks one that was typed in, and says where it came from.
func TestAFetchedTierOverridesAndSaysSo(t *testing.T) {
	pool := &AccountKeyPool{}
	pool.AddKey("A", "key-a")
	id := pool.GetKeys()[0].ID
	pool.SetTier(id, "FREE")

	quotas := []ModelQuota{QuotaFromFraction("Gemini 3 Pro", "gemini", 0.25, false, 3*time.Hour)}
	pool.ApplyQuota(id, "PRO", "standard-tier", "proj-7", quotas, "")

	got := pool.GetKeys()[0]
	if got.Tier != "PRO" || got.TierSource != "google" {
		t.Errorf("tier = %q from %q, want PRO from google", got.Tier, got.TierSource)
	}
	if got.RawTier != "standard-tier" || got.ProjectID != "proj-7" {
		t.Errorf("raw tier %q project %q not recorded", got.RawTier, got.ProjectID)
	}
	if got.QuotaFetchedAt.IsZero() {
		t.Error("the reading was not dated")
	}
	if len(got.ModelQuotas) != 1 || got.ModelQuotas[0].UsagePct != 75 {
		t.Errorf("quotas = %+v", got.ModelQuotas)
	}
}

// Keeping the previous numbers after a failed refresh would date stale readings
// to now — the same "looks informative, is not" failure as the constant.
func TestAFailedRefreshClearsTheOldNumbers(t *testing.T) {
	pool := &AccountKeyPool{}
	pool.AddKey("A", "key-a")
	id := pool.GetKeys()[0].ID
	pool.ApplyQuota(id, "PRO", "standard-tier", "proj-7",
		[]ModelQuota{QuotaFromFraction("Gemini 3 Pro", "gemini", 0.9, false, time.Hour)}, "")

	pool.ApplyQuota(id, "", "", "", nil, "mạng lỗi")

	for _, q := range pool.GetKeys()[0].ModelQuotas {
		if q.UsagePct != -1 || q.Status != "unknown" {
			t.Errorf("%s kept a stale reading: %d/%s", q.Name, q.UsagePct, q.Status)
		}
	}
}
