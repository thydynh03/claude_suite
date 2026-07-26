package cli

import "testing"

// The quota table used to be filled from a compile-time constant: the same 53%
// and "0h 0m" for every account, under a heading that says "hạn mức". Nothing had
// ever asked Google anything. This test exists to stop that coming back.
func TestNewAccountsClaimNoQuotaTheyHaveNotMeasured(t *testing.T) {
	pool := &AccountKeyPool{}
	pool.AddKey("Tài khoản A", "api-key-a")
	pool.AddOAuthKey("Google (b@gmail.com)", "oauth-b")
	pool.AddRefreshAccount("c@gmail.com", "refresh-c")

	for _, k := range pool.GetKeys() {
		if k.Tier != tierUnknown {
			t.Errorf("%s: tier = %q, want %q — the tier was never fetched", k.Email, k.Tier, tierUnknown)
		}
		if k.TierSource != "unknown" {
			t.Errorf("%s: tier_source = %q, want unknown", k.Email, k.TierSource)
		}
		for _, q := range k.ModelQuotas {
			if q.UsagePct != -1 {
				t.Errorf("%s / %s: usage_pct = %d, want -1 (no data)", k.Email, q.Name, q.UsagePct)
			}
			if q.Status != "unknown" {
				t.Errorf("%s / %s: status = %q, want unknown", k.Email, q.Name, q.Status)
			}
			if q.ResetTime != "" {
				t.Errorf("%s / %s: reset_time = %q, want empty — nothing knows when it resets", k.Email, q.Name, q.ResetTime)
			}
		}
	}
}

// Requests and 429s are the two things the app genuinely observes, so they are
// what the dashboard shows instead.
func TestUsageCountsWhatTheAppActuallyObserves(t *testing.T) {
	pool := &AccountKeyPool{}
	pool.AddKey("A", "key-a")
	pool.AddKey("B", "key-b")

	pool.GetCurrentAccount()
	pool.GetCurrentAccount()

	if got := pool.GetKeys()[0].Usage.Requests; got != 2 {
		t.Errorf("requests = %d, want 2", got)
	}
	if pool.GetKeys()[0].Usage.LastRequestAt.IsZero() {
		t.Error("last_request_at not recorded")
	}

	pool.RotateNextKey()
	first := pool.GetKeys()[0]
	if first.Usage.RateLimitHits != 1 {
		t.Errorf("rate_limit_hits = %d, want 1", first.Usage.RateLimitHits)
	}
	if first.Usage.LastRateLimitAt.IsZero() {
		t.Error("last_rate_limit_at not recorded")
	}
}

// A tier the user vouched for is worth keeping; one this code invented is not.
func TestSetTierRecordsThatAHumanChoseIt(t *testing.T) {
	pool := &AccountKeyPool{}
	pool.AddKey("A", "key-a")
	id := pool.GetKeys()[0].ID

	if !pool.SetTier(id, "PRO") {
		t.Fatal("SetTier returned false for a valid tier")
	}
	got := pool.GetKeys()[0]
	if got.Tier != "PRO" || got.TierSource != "user" {
		t.Errorf("tier = %q from %q, want PRO from user", got.Tier, got.TierSource)
	}

	if pool.SetTier(id, "PLATINUM") {
		t.Error("SetTier accepted a tier that does not exist")
	}
	if pool.GetKeys()[0].Tier != "PRO" {
		t.Error("a rejected tier still overwrote the stored one")
	}
}
