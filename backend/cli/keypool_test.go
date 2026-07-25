package cli

import "testing"

func poolWith(keys ...AntiAccountKey) *AccountKeyPool {
	pool := &AccountKeyPool{}
	pool.SetKeys(keys)
	return pool
}

func active(id string) AntiAccountKey {
	return AntiAccountKey{ID: id, Name: id, APIKey: id + "-secret", Status: "active"}
}

func disabled(id string) AntiAccountKey {
	return AntiAccountKey{ID: id, Name: id, APIKey: id + "-secret", Status: statusDisabled}
}

// Switching an account off in the pool dashboard has to stop requests going out
// as that account. Until this was fixed, nothing read Status when picking a key,
// so the toggle changed a label and nothing else.
func TestGetCurrentAccountSkipsDisabledAccounts(t *testing.T) {
	pool := poolWith(disabled("first"), active("second"))

	acc := pool.GetCurrentAccount()
	if acc == nil {
		t.Fatal("expected the enabled account, got nil")
	}
	if acc.ID != "second" {
		t.Errorf("selected %q, want the enabled account %q", acc.ID, "second")
	}
}

func TestGetCurrentAccountReturnsNilWhenEveryAccountIsDisabled(t *testing.T) {
	pool := poolWith(disabled("first"), disabled("second"))

	if acc := pool.GetCurrentAccount(); acc != nil {
		t.Errorf("selected %q, want nil so the caller falls back to the environment", acc.ID)
	}
}

func TestGetCurrentAccountReturnsNilWhenPoolIsEmpty(t *testing.T) {
	if acc := poolWith().GetCurrentAccount(); acc != nil {
		t.Errorf("got %v, want nil", acc)
	}
}

// A 429 rotates to the next account, marking the exhausted one rate-limited.
func TestRotateNextKeyMovesToTheNextAccount(t *testing.T) {
	pool := poolWith(active("first"), active("second"))

	if name := pool.RotateNextKey(); name != "second" {
		t.Errorf("rotated to %q, want %q", name, "second")
	}

	keys := pool.GetKeys()
	if keys[0].Status != "rate_limited_429" {
		t.Errorf("exhausted account status = %q, want rate_limited_429", keys[0].Status)
	}
	if acc := pool.GetCurrentAccount(); acc == nil || acc.ID != "second" {
		t.Errorf("current account = %v, want second", acc)
	}
}

// Rotation used to set the next account active unconditionally, which switched a
// disabled account back on behind the user's back.
func TestRotateNextKeyNeverWakesADisabledAccount(t *testing.T) {
	pool := poolWith(active("first"), disabled("second"), active("third"))

	if name := pool.RotateNextKey(); name != "third" {
		t.Errorf("rotated to %q, want %q — the disabled account must be skipped", name, "third")
	}

	for _, k := range pool.GetKeys() {
		if k.ID == "second" && k.Status != statusDisabled {
			t.Errorf("disabled account was switched to %q", k.Status)
		}
	}
}

func TestRotateNextKeyReportsNothingWhenEveryOtherAccountIsDisabled(t *testing.T) {
	pool := poolWith(active("first"), disabled("second"))

	if name := pool.RotateNextKey(); name != "" {
		t.Errorf("rotated to %q, want no rotation", name)
	}
}

func TestRotateNextKeyNeedsMoreThanOneAccount(t *testing.T) {
	if name := poolWith(active("only")).RotateNextKey(); name != "" {
		t.Errorf("rotated to %q with a single account, want no rotation", name)
	}
}

// Rotating into an account is a decision to retry it, so an old rate-limit mark
// is cleared. Only "disabled" is permanent.
func TestRotateNextKeyRetriesARateLimitedAccount(t *testing.T) {
	pool := poolWith(active("first"), AntiAccountKey{ID: "second", Name: "second", Status: "rate_limited_429"})

	if name := pool.RotateNextKey(); name != "second" {
		t.Fatalf("rotated to %q, want second", name)
	}
	for _, k := range pool.GetKeys() {
		if k.ID == "second" && k.Status != "active" {
			t.Errorf("retried account status = %q, want active", k.Status)
		}
	}
}

func TestToggleKeyStatusFlipsBothWays(t *testing.T) {
	pool := poolWith(active("first"))

	if got := pool.ToggleKeyStatus("first"); got != statusDisabled {
		t.Errorf("first toggle = %q, want %q", got, statusDisabled)
	}
	if got := pool.ToggleKeyStatus("first"); got != "active" {
		t.Errorf("second toggle = %q, want active", got)
	}
	if got := pool.ToggleKeyStatus("missing"); got != "" {
		t.Errorf("toggling an unknown id = %q, want an empty string", got)
	}
}

func TestDeleteKey(t *testing.T) {
	pool := poolWith(active("first"), active("second"))

	if !pool.DeleteKey("first") {
		t.Error("DeleteKey reported the account was not found")
	}
	if got := len(pool.GetKeys()); got != 1 {
		t.Errorf("pool holds %d accounts, want 1", got)
	}
	if pool.DeleteKey("first") {
		t.Error("deleting the same account twice reported success")
	}
}

// GetKeys marks exactly one account as current so the dashboard can highlight it.
func TestGetKeysMarksTheCurrentAccount(t *testing.T) {
	pool := poolWith(active("first"), active("second"))
	pool.SetCurrentKey("second")

	var currents []string
	for _, k := range pool.GetKeys() {
		if k.IsCurrent {
			currents = append(currents, k.ID)
		}
	}

	if len(currents) != 1 || currents[0] != "second" {
		t.Errorf("accounts marked current = %v, want [second]", currents)
	}
}

func TestSetCurrentKeyRejectsUnknownID(t *testing.T) {
	pool := poolWith(active("first"))
	if pool.SetCurrentKey("missing") {
		t.Error("SetCurrentKey accepted an id that is not in the pool")
	}
}

// Warmup is the dashboard's "try everything again" button; it must not override
// the accounts the user switched off.
func TestWarmupKeysLeavesDisabledAccountsAlone(t *testing.T) {
	pool := poolWith(
		AntiAccountKey{ID: "first", Status: "rate_limited_429"},
		disabled("second"),
	)

	pool.WarmupKeys()

	for _, k := range pool.GetKeys() {
		switch k.ID {
		case "first":
			if k.Status != "active" {
				t.Errorf("rate-limited account status = %q, want active", k.Status)
			}
		case "second":
			if k.Status != statusDisabled {
				t.Errorf("disabled account status = %q, want it left disabled", k.Status)
			}
		}
	}
}
