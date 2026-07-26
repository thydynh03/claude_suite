package claims

import (
	"testing"
)

// AppendCheck must round-trip through LoadCatalogue and refuse duplicates and
// commandless entries — the argv-only stance survives the new write path.
func TestAppendCheckRoundTripsThroughLoadCatalogue(t *testing.T) {
	ws := t.TempDir()

	check := Check{
		Name:        "updater-retry-regression",
		Description: "Guard cho regression: updater retry loop",
		Command:     []string{"go", "test", "./backend/services/", "-run", "TestUpdaterRetry"},
	}
	if err := AppendCheck(ws, check); err != nil {
		t.Fatalf("append: %v", err)
	}

	catalogue, err := LoadCatalogue(ws)
	if err != nil {
		t.Fatalf("catalogue written by AppendCheck fails its own loader: %v", err)
	}
	got, ok := catalogue.Lookup("updater-retry-regression")
	if !ok {
		t.Fatal("appended check not found")
	}
	if len(got.Command) != 5 || got.Command[0] != "go" {
		t.Fatalf("argv did not survive: %+v", got.Command)
	}
	if got.TimeoutSec == 0 {
		t.Fatal("timeout default not applied")
	}

	if err := AppendCheck(ws, check); err == nil {
		t.Fatal("duplicate name must be rejected")
	}
	if err := AppendCheck(ws, Check{Name: "no-command"}); err == nil {
		t.Fatal("a check without an argv command must be rejected")
	}
}
