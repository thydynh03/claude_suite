package services

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
	"time"
)

// chromedp binds a target's lifetime to the context passed to the FIRST Run on
// it. Allocating under a deadline therefore kills the session when that deadline
// passes, mid-run, with "context canceled" on whatever step happened to be next.
//
// This is a source check rather than a behavioural one on purpose: reproducing it
// needs a real Chrome and slightly over a minute of wall clock, and the property
// worth protecting is structural — the first Run must be on a context with no
// deadline.
func TestTheFirstChromedpRunHasNoDeadline(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "browser.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse browser.go: %v", err)
	}

	// Scoped to the multi-step loop. RunBrowserTask does one Run under a timeout
	// and returns, so its target dying with that context is correct there; only
	// a session meant to outlive a single batch is at risk.
	var loop ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if ok && fn.Name.Name == "RunAutonomousBrowserTask" {
			loop = fn
			return false
		}
		return true
	})
	if loop == nil {
		t.Fatal("RunAutonomousBrowserTask not found — did it get renamed?")
	}

	var firstRunArg string
	ast.Inspect(loop, func(n ast.Node) bool {
		if firstRunArg != "" {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Run" {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "chromedp" || len(call.Args) == 0 {
			return true
		}
		if ident, ok := call.Args[0].(*ast.Ident); ok {
			firstRunArg = ident.Name
		}
		return false
	})

	if firstRunArg == "" {
		t.Fatal("found no chromedp.Run call in browser.go — did the file move?")
	}
	if strings.Contains(strings.ToLower(firstRunArg), "init") ||
		strings.Contains(strings.ToLower(firstRunArg), "timeout") ||
		strings.Contains(strings.ToLower(firstRunArg), "run") {
		t.Errorf("the first chromedp.Run uses %q, which is a derived timeout context; "+
			"the target is then torn down when that deadline passes and every later "+
			"action fails with \"context canceled\"", firstRunArg)
	}
	if firstRunArg != "cdpCtx" {
		t.Errorf("first chromedp.Run uses %q, want the undeadlined session context cdpCtx", firstRunArg)
	}
}

// Guards the reasoning above: a context derived with WithTimeout does carry a
// deadline that its parent does not, which is precisely why allocating on one is
// fatal to the session.
func TestADerivedTimeoutContextCarriesADeadlineItsParentDoesNot(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()

	if _, ok := parent.Deadline(); ok {
		t.Fatal("the parent context unexpectedly has a deadline")
	}

	child, cancelChild := context.WithTimeout(parent, 60*time.Second)
	defer cancelChild()

	deadline, ok := child.Deadline()
	if !ok {
		t.Fatal("the derived context has no deadline")
	}
	if time.Until(deadline) > 61*time.Second {
		t.Errorf("deadline is %v away, want about a minute", time.Until(deadline))
	}
}
