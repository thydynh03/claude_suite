package cli

import (
	"sync"
	"testing"
	"time"
)

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

	if name := pool.RotateNextKey(""); name != "second" {
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

	if name := pool.RotateNextKey(""); name != "third" {
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

	if name := pool.RotateNextKey(""); name != "" {
		t.Errorf("rotated to %q, want no rotation", name)
	}
}

func TestRotateNextKeyNeedsMoreThanOneAccount(t *testing.T) {
	if name := poolWith(active("only")).RotateNextKey(""); name != "" {
		t.Errorf("rotated to %q with a single account, want no rotation", name)
	}
}

// Rotating into an account is a decision to retry it, so an old rate-limit mark
// is cleared. Only "disabled" is permanent.
func TestRotateNextKeyRetriesARateLimitedAccount(t *testing.T) {
	pool := poolWith(active("first"), AntiAccountKey{ID: "second", Name: "second", Status: "rate_limited_429"})

	if name := pool.RotateNextKey(""); name != "second" {
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

func TestIsRotatableAuthError(t *testing.T) {
	cases := map[string]bool{
		"http 429 too many requests":            true,
		"quota exceeded for this project":       true,
		"401 unauthorized":                      true, // an expired access token
		"invalid_grant: token has been revoked": true,
		"connection refused":                    false,
		"file not found":                        false,
	}
	for message, want := range cases {
		if got := isRotatableAuthError(message); got != want {
			t.Errorf("isRotatableAuthError(%q) = %v, want %v", message, got, want)
		}
	}
}

// An imported account has only a refresh token; its access token is minted when
// it is first needed.
func TestAddRefreshAccountStartsWithoutAnAccessToken(t *testing.T) {
	pool := &AccountKeyPool{}
	id := pool.AddRefreshAccount("one@example.com", "1//refresh")

	keys := pool.GetKeys()
	if len(keys) != 1 || keys[0].RefreshToken != "1//refresh" {
		t.Fatalf("account was not stored: %+v", keys)
	}
	if keys[0].OAuthToken != "" {
		t.Error("an access token was invented at import time")
	}
	if !keys[0].NeedsRefresh() {
		t.Error("a freshly imported account does not report needing a token")
	}
	if id == "" {
		t.Error("no id was returned")
	}
}

// Re-importing the same file must update the token rather than pile up copies.
func TestAddRefreshAccountReplacesTheSameEmail(t *testing.T) {
	pool := &AccountKeyPool{}
	pool.AddRefreshAccount("one@example.com", "1//old")
	pool.AddRefreshAccount("one@example.com", "1//new")

	keys := pool.GetKeys()
	if len(keys) != 1 {
		t.Fatalf("re-import created %d accounts, want 1", len(keys))
	}
	if keys[0].RefreshToken != "1//new" {
		t.Errorf("refresh token = %q, want the re-imported one", keys[0].RefreshToken)
	}
}

func TestSetAccessTokenSatisfiesTheRefreshCheck(t *testing.T) {
	pool := &AccountKeyPool{}
	id := pool.AddRefreshAccount("one@example.com", "1//refresh")

	if pending := pool.AccountsNeedingRefresh(); len(pending) != 1 {
		t.Fatalf("expected one account awaiting a token, got %d", len(pending))
	}

	pool.SetAccessToken(id, "ya29.fresh", time.Now().Add(time.Hour))

	if pending := pool.AccountsNeedingRefresh(); len(pending) != 0 {
		t.Errorf("the account still wants a token after being given one")
	}
	if got := pool.GetCurrentKey(); got != "ya29.fresh" {
		t.Errorf("current key = %q, want the fresh access token", got)
	}
}

// An hour later the same token is no longer usable and has to be minted again.
func TestAnExpiredAccessTokenIsRefreshedAgain(t *testing.T) {
	pool := &AccountKeyPool{}
	id := pool.AddRefreshAccount("one@example.com", "1//refresh")
	pool.SetAccessToken(id, "ya29.stale", time.Now().Add(-time.Minute))

	if pending := pool.AccountsNeedingRefresh(); len(pending) != 1 {
		t.Error("an expired access token was treated as still good")
	}
}

// Ids were minted as "key-<count+1>", so anything deleted freed a number that
// the next account then reused. DeleteKey removes EVERY row with the id, so
// deleting the newcomer silently deleted the older account with the same id.
func TestAddingAfterADeleteDoesNotReuseAnId(t *testing.T) {
	pool := &AccountKeyPool{}
	first := pool.AddRefreshAccount("one@example.com", "1//a")
	second := pool.AddRefreshAccount("two@example.com", "1//b")
	pool.DeleteKey(first)

	third := pool.AddRefreshAccount("three@example.com", "1//c")
	if third == second {
		t.Fatalf("new account reused the id %q of a live account", third)
	}

	// The real damage: deleting the newcomer took the survivor with it.
	pool.DeleteKey(third)
	keys := pool.GetKeys()
	if len(keys) != 1 || keys[0].Email != "two@example.com" {
		t.Fatalf("deleting one account left %+v, want only two@example.com", keys)
	}
}

// Naming a key "Work Key" used to display "work.key@gmail.com" as its account
// email — invented identity in the table that is meant to be measured, and it
// broke the email dedupe when the real account was imported later.
func TestAddKeyDoesNotInventAnEmail(t *testing.T) {
	pool := &AccountKeyPool{}
	pool.AddKey("Work Key", "AIza-secret")

	keys := pool.GetKeys()
	if len(keys) != 1 {
		t.Fatalf("expected one account, got %d", len(keys))
	}
	if keys[0].Email != "" {
		t.Errorf("email = %q, want empty rather than an invented address", keys[0].Email)
	}
}

// Drawing the pool on screen must not count requests against an account: the
// TUI redraws on every keypress, and GetCurrentAccount's contract is "a
// request is about to be sent".
func TestPeekCurrentAccountDoesNotCountARequest(t *testing.T) {
	pool := poolWith(AntiAccountKey{ID: "key-1", Email: "one@example.com", Status: "active"})

	before := pool.GetKeys()[0].Usage.Requests
	for i := 0; i < 5; i++ {
		if acc := pool.PeekCurrentAccount(); acc == nil {
			t.Fatal("peek returned no account")
		}
	}
	if after := pool.GetKeys()[0].Usage.Requests; after != before {
		t.Errorf("request count went %d → %d just from looking at the pool", before, after)
	}

	pool.GetCurrentAccount()
	if after := pool.GetKeys()[0].Usage.Requests; after != before+1 {
		t.Errorf("an actual run did not count: %d → %d", before, after)
	}
}

// The runners share one instance across every task goroutine, so re-resolving
// the executable must not be a plain field write. This drives the concurrent
// path the race detector job would otherwise catch in production code.
func TestResolvedPathSurvivesConcurrentEnsure(t *testing.T) {
	rp := newResolvedPath("claude") // the bare fallback: not installed
	resolved := "C:\\tools\\claude.exe"

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got, ok := rp.ensure(func() string { return resolved }); !ok || got != resolved {
				t.Errorf("ensure returned (%q, %v), want the resolved path", got, ok)
			}
			_ = rp.get()
		}()
	}
	wg.Wait()

	if rp.get() != resolved {
		t.Errorf("cached path = %q, want %q", rp.get(), resolved)
	}
}

// When nothing resolves, the caller must be told so it can name the missing
// CLI rather than surfacing a raw exec error about %PATH%.
func TestResolvedPathReportsWhenNothingIsInstalled(t *testing.T) {
	rp := newResolvedPath("agy")
	if _, ok := rp.ensure(func() string { return "agy" }); ok {
		t.Error("ensure reported success for the bare fallback name")
	}
}

// A revoked refresh token is switched off rather than retried every minute.
func TestDisableKeyStopsAnAccountBeingRetried(t *testing.T) {
	pool := &AccountKeyPool{}
	id := pool.AddRefreshAccount("gone@example.com", "1//revoked")

	if !pool.DisableKey(id, "invalid_grant") {
		t.Fatal("DisableKey did not find the account")
	}
	if pending := pool.AccountsNeedingRefresh(); len(pending) != 0 {
		t.Error("a disabled account is still queued for refresh")
	}
}
