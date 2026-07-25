package cli

import "testing"

// An empty pool is what every fresh install has. GetKeys used to compute
// p.current % len(p.keys) unconditionally, and a panic inside a bound method
// never returns to the frontend: the promise hangs and the page spins forever.
func TestGetKeysOnAnEmptyPoolDoesNotPanic(t *testing.T) {
	pool := &AccountKeyPool{}
	got := pool.GetKeys()
	if len(got) != 0 {
		t.Fatalf("empty pool returned %d keys", len(got))
	}
}

// Accounts saved before the id field existed load back with an empty one. The
// UI lists them with a keyed loop, so eleven accounts sharing the empty key
// rendered as "no accounts" while the counter above still said eleven.
func TestSetKeysBackfillsMissingIDs(t *testing.T) {
	pool := &AccountKeyPool{}
	pool.SetKeys([]AntiAccountKey{
		{Email: "one@example.com"},
		{Email: "two@example.com"},
		{Name: "No Email"},
		{},
	})

	seen := map[string]bool{}
	for i, k := range pool.GetKeys() {
		if k.ID == "" {
			t.Fatalf("key %d still has no id", i)
		}
		if seen[k.ID] {
			t.Fatalf("id %q is used twice; a keyed list cannot render duplicates", k.ID)
		}
		seen[k.ID] = true
	}
}

// An id that was already stored must survive, or every reload renames accounts.
func TestSetKeysKeepsExistingIDs(t *testing.T) {
	pool := &AccountKeyPool{}
	pool.SetKeys([]AntiAccountKey{
		{ID: "stable-id", Email: "one@example.com"},
		{Email: "two@example.com"},
	})

	got := pool.GetKeys()
	if got[0].ID != "stable-id" {
		t.Fatalf("existing id was rewritten to %q", got[0].ID)
	}
}

// Two accounts that arrive with the same id are still a duplicate key.
func TestSetKeysDeduplicatesCollidingIDs(t *testing.T) {
	pool := &AccountKeyPool{}
	pool.SetKeys([]AntiAccountKey{
		{ID: "same", Email: "one@example.com"},
		{ID: "same", Email: "two@example.com"},
	})

	got := pool.GetKeys()
	if got[0].ID == got[1].ID {
		t.Fatalf("both keys kept the id %q", got[0].ID)
	}
}

// Exactly one account is marked current, whatever the rotation counter is.
func TestGetKeysMarksOneCurrent(t *testing.T) {
	pool := &AccountKeyPool{}
	pool.SetKeys([]AntiAccountKey{
		{Email: "one@example.com"}, {Email: "two@example.com"}, {Email: "three@example.com"},
	})
	pool.current = 7 // past the end, as rotation leaves it

	current := 0
	for _, k := range pool.GetKeys() {
		if k.IsCurrent {
			current++
		}
	}
	if current != 1 {
		t.Fatalf("%d keys marked current, want exactly 1", current)
	}
}
