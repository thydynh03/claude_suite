package services

import (
	"strings"
	"testing"
)

func TestMCPServerNeedsEitherACommandOrAURL(t *testing.T) {
	store := NewMCPStore(t.TempDir())

	if _, err := store.Add(MCPServer{Name: "Trống"}); err == nil {
		t.Error("a server with neither a command nor a URL was accepted")
	}
	// Both is not "extra safe", it is ambiguous: nothing decides which one runs.
	if _, err := store.Add(MCPServer{Name: "Cả hai", Command: "npx", URL: "http://x"}); err == nil {
		t.Error("a server with both a command and a URL was accepted")
	}
	if _, err := store.Add(MCPServer{Command: "npx"}); err == nil {
		t.Error("a server with no name was accepted")
	}
}

func TestMCPServersSurviveARestart(t *testing.T) {
	dir := t.TempDir()

	first := NewMCPStore(dir)
	if _, err := store_add(first, "Filesystem", "npx"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// A fresh store over the same directory is what the next app launch sees.
	second := NewMCPStore(dir)
	got := second.List()
	if len(got) != 1 || got[0].Name != "Filesystem" {
		t.Fatalf("after restart: %+v, want the server that was added", got)
	}
}

func TestDuplicateNamesAreRefused(t *testing.T) {
	store := NewMCPStore(t.TempDir())
	if _, err := store_add(store, "Git", "npx"); err != nil {
		t.Fatal(err)
	}
	// Case-insensitive: two entries differing only in case are indistinguishable
	// in the list the user reads.
	if _, err := store_add(store, "git", "npx"); err == nil {
		t.Error("a duplicate name differing only in case was accepted")
	}
}

// A missing `npx` is the common failure, and it used to surface only when a task
// ran and silently had no tools.
func TestTestServerNamesAMissingCommand(t *testing.T) {
	store := NewMCPStore(t.TempDir())
	added, err := store_add(store, "Hỏng", "khong-co-lenh-nay-tren-may")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.TestServer(added.ID); err == nil {
		t.Fatal("TestServer passed for a command that does not exist")
	}

	got := store.List()[0]
	if got.LastStatus != "error" || !strings.Contains(got.LastError, "PATH") {
		t.Errorf("status = %q, error = %q; want the missing command explained", got.LastStatus, got.LastError)
	}
	if got.LastCheckedAt.IsZero() {
		t.Error("last_checked_at not recorded")
	}
}

func TestRemoveAndEnableReportUnknownIDs(t *testing.T) {
	store := NewMCPStore(t.TempDir())

	if err := store.Remove("khong-ton-tai"); err == nil {
		t.Error("Remove silently accepted an unknown id")
	}
	if err := store.SetEnabled("khong-ton-tai", true); err == nil {
		t.Error("SetEnabled silently accepted an unknown id")
	}
}

func TestCatalogueEntriesAreUsableAsWritten(t *testing.T) {
	for _, srv := range MCPCatalogue() {
		if srv.Name == "" || srv.Desc == "" {
			t.Errorf("%+v: catalogue entries need a name and a description", srv)
		}
		if srv.Command == "" && srv.URL == "" {
			t.Errorf("%s: neither command nor URL — it could not be added", srv.Name)
		}
		if !srv.Builtin {
			t.Errorf("%s: catalogue entries must be marked builtin", srv.Name)
		}
	}
}

func store_add(s *MCPStore, name, command string) (MCPServer, error) {
	return s.Add(MCPServer{Name: name, Command: command})
}
