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
	nextName := pool.RotateNextKey()
	if nextName != "Key 2" {
		t.Errorf("expected next name Key 2, got %s", nextName)
	}

	if pool.GetCurrentKey() != "key-2-val" {
		t.Errorf("expected rotated key-2-val, got %s", pool.GetCurrentKey())
	}

	// 2nd rotation
	nextName2 := pool.RotateNextKey()
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
