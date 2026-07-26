package cli

import (
	"testing"
)

func TestAccountKeyPoolRotation(t *testing.T) {
	pool := &AccountKeyPool{
		keys: []AntiAccountKey{
			{ID: "key-1", Name: "Key 1", APIKey: "key-1-val", Status: "active"},
			{ID: "key-2", Name: "Key 2", APIKey: "key-2-val", Status: "active"},
			{ID: "key-3", Name: "Key 3", APIKey: "key-3-val", Status: "active"},
		},
	}

	if pool.GetCurrentKey() != "key-1-val" {
		t.Errorf("expected initial key-1-val, got %s", pool.GetCurrentKey())
	}

	// 1st rotation on 429 Rate Limit
	nextName := pool.RotateNextKey("key-1")
	if nextName != "Key 2" {
		t.Errorf("expected next name Key 2, got %s", nextName)
	}

	if pool.GetCurrentKey() != "key-2-val" {
		t.Errorf("expected rotated key-2-val, got %s", pool.GetCurrentKey())
	}

	// 2nd rotation
	nextName2 := pool.RotateNextKey("key-2")
	if nextName2 != "Key 3" {
		t.Errorf("expected next name Key 3, got %s", nextName2)
	}

	if pool.GetCurrentKey() != "key-3-val" {
		t.Errorf("expected rotated key-3-val, got %s", pool.GetCurrentKey())
	}

	// Verify key status tracking
	keys := pool.GetKeys()
	if keys[0].Status != "rate_limited_429" || keys[1].Status != "rate_limited_429" || keys[2].Status != "active" {
		t.Errorf("unexpected key statuses: %+v", keys)
	}
}

// Three concurrent tasks all fail under account A. Only the first rotation may
// punish A; the other two must see that the pool already moved and leave the
// healthy accounts alone. Positional rotation used to mark B and C
// rate-limited (fabricated stats) and rotate back to A — every retry then ran
// as the one genuinely exhausted account.
func TestRotateNextKeyPunishesOnlyTheAccountThatFailed(t *testing.T) {
	pool := &AccountKeyPool{
		keys: []AntiAccountKey{
			{ID: "a", Name: "A", APIKey: "a-val", Status: "active"},
			{ID: "b", Name: "B", APIKey: "b-val", Status: "active"},
			{ID: "c", Name: "C", APIKey: "c-val", Status: "active"},
		},
	}

	// All three failing runs used account "a".
	if got := pool.RotateNextKey("a"); got != "B" {
		t.Fatalf("first rotation moved to %q, want B", got)
	}
	if got := pool.RotateNextKey("a"); got != "B" {
		t.Fatalf("second rotation for the same failure punished someone: got %q, want no-op reporting B", got)
	}
	if got := pool.RotateNextKey("a"); got != "B" {
		t.Fatalf("third rotation for the same failure punished someone: got %q, want no-op reporting B", got)
	}

	keys := pool.GetKeys()
	if keys[0].Status != "rate_limited_429" {
		t.Errorf("the failing account was not marked: %+v", keys[0])
	}
	if keys[1].Status != "active" || keys[2].Status != "active" {
		t.Errorf("healthy accounts were poisoned: B=%s C=%s", keys[1].Status, keys[2].Status)
	}
	if keys[0].Usage.RateLimitHits != 1 {
		t.Errorf("rate-limit hits on A = %d, want exactly 1 (measured, not fabricated)", keys[0].Usage.RateLimitHits)
	}
}
