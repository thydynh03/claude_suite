// Package contract holds cross-frontend consistency checks.
//
// Agent Center ships two frontends over one backend: the Wails desktop app
// (methods on *App in app.go) and the terminal UI (methods on
// *tui.RepositoryTaskActions). Both are hand-written adapters over the same
// services, so the same capability can — and did — end up with two names:
// GetGitStatus vs GitStatus, ExportKanbanReport vs ExportReport, and six more.
// Code review did not catch a single one of those eight.
//
// These tests read the two adapters straight from source, so they run in CI
// (the repository root cannot be compiled there: main.go embeds frontend/dist,
// which does not exist until the frontend job has built it).
package contract

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const (
	wailsAdapterPath = "../../app.go"
	tuiAdapterPath   = "../tui/task_actions.go"
	wailsReceiver    = "App"
	tuiReceiver      = "RepositoryTaskActions"
)

// tuiOnlyMethods are capabilities that deliberately exist on the terminal UI
// alone. Every entry needs a reason: the point of this list is to make "only one
// frontend has this" a decision someone wrote down, not an accident.
//
// Before adding an entry, check you are not about to re-introduce the exact
// problem this test exists to prevent — a capability the desktop app already has
// under a different name.
var tuiOnlyMethods = map[string]string{
	"Close":  "the TUI owns its own database handle; the Wails app closes on shutdown",
	"Events": "the TUI streams runtime events into its Bubble Tea loop; Wails uses EventsEmit",
}

// TestAdaptersAgreeOnCapabilityNames fails when the terminal UI exposes a
// capability the desktop app does not have under the same name.
//
// If this fails, you have three honest options:
//  1. use the name the other adapter already uses (usually right);
//  2. rename on both sides in the same change;
//  3. add the method to tuiOnlyMethods with a reason, if it truly is TUI-only.
func TestAdaptersAgreeOnCapabilityNames(t *testing.T) {
	wails := exportedMethods(t, wailsAdapterPath, wailsReceiver)
	tui := exportedMethods(t, tuiAdapterPath, tuiReceiver)

	if len(wails) == 0 || len(tui) == 0 {
		t.Fatalf("parsed no methods (wails=%d, tui=%d) — did an adapter move?", len(wails), len(tui))
	}

	var drifted []string
	for _, name := range sortedKeys(tui) {
		if wails[name] {
			continue
		}
		if _, allowed := tuiOnlyMethods[name]; allowed {
			continue
		}
		drifted = append(drifted, name)
	}

	for _, name := range drifted {
		t.Errorf("TUI method %q has no same-named method on the Wails *App.\n"+
			"  If the desktop app already does this under another name, use that name instead —\n"+
			"  that is how GetGitStatus/GitStatus and seven other pairs happened.\n"+
			"  If it is genuinely TUI-only, add it to tuiOnlyMethods with a reason.", name)
	}
}

// TestTUIOnlyAllowlistIsCurrent keeps the allowlist honest: an entry that no
// longer exists, or that both adapters now implement, is stale and would quietly
// widen the exemption.
func TestTUIOnlyAllowlistIsCurrent(t *testing.T) {
	wails := exportedMethods(t, wailsAdapterPath, wailsReceiver)
	tui := exportedMethods(t, tuiAdapterPath, tuiReceiver)

	for name, reason := range tuiOnlyMethods {
		switch {
		case reason == "":
			t.Errorf("tuiOnlyMethods[%q] has no reason", name)
		case !tui[name]:
			t.Errorf("tuiOnlyMethods[%q] is stale: the TUI adapter no longer declares it", name)
		case wails[name]:
			t.Errorf("tuiOnlyMethods[%q] is stale: both adapters implement it now, so remove the exemption", name)
		}
	}
}

// exportedMethods returns the exported method names declared on the given
// receiver type in a single Go source file.
func exportedMethods(t *testing.T, path, receiver string) map[string]bool {
	t.Helper()

	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolve %s: %v", path, err)
	}
	if _, err := os.Stat(absolute); err != nil {
		t.Fatalf("adapter not found at %s — if it moved, update the path in this test: %v", absolute, err)
	}

	file, err := parser.ParseFile(token.NewFileSet(), absolute, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", absolute, err)
	}

	methods := make(map[string]bool)
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
			continue
		}
		if !fn.Name.IsExported() || receiverName(fn.Recv.List[0].Type) != receiver {
			continue
		}
		methods[fn.Name.Name] = true
	}
	return methods
}

// receiverName reports the type name of a method receiver, ignoring the pointer.
func receiverName(expr ast.Expr) string {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return ""
	}
	return ident.Name
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// TestAdapterPathsAreRelativeToRepoRoot documents the assumption above: these
// tests read sources by relative path, so they only work when run from their own
// package directory (which is what `go test ./...` does).
func TestAdapterPathsAreRelativeToRepoRoot(t *testing.T) {
	for _, path := range []string{wailsAdapterPath, tuiAdapterPath} {
		if !strings.HasPrefix(path, "..") {
			t.Errorf("adapter path %q should be relative to this package", path)
		}
	}
}
