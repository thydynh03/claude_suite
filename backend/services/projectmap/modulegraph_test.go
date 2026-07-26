package projectmap

import (
	"testing"
)

func TestModuleGraphAggregatesImportsToDirectoryLevel(t *testing.T) {
	m := newTestMapper(t)
	root := writeFixtureWorkspace(t)
	if _, err := m.FullBuild(root); err != nil {
		t.Fatal(err)
	}

	graph, err := m.ModuleGraph(root)
	if err != nil {
		t.Fatal(err)
	}

	byID := map[string]int{}
	for _, n := range graph.Nodes {
		byID[n.ID] = n.Files
	}
	if byID["module:internal/store"] != 1 {
		t.Fatalf("module file counts wrong: %+v", graph.Nodes)
	}
	if byID["module:web"] != 2 {
		t.Fatalf("web module should hold 2 files: %+v", graph.Nodes)
	}

	// main.go imports internal/store (Go) and App.svelte imports state.js —
	// the first is cross-module, the second collapses inside module:web.
	foundCross := false
	for _, e := range graph.Edges {
		if e.Source == "module:." && e.Target == "module:internal/store" {
			foundCross = true
			if e.Count < 1 {
				t.Fatalf("edge count = %d, want >= 1", e.Count)
			}
		}
		if e.Source == e.Target {
			t.Fatalf("self edge leaked into the module graph: %+v", e)
		}
	}
	if !foundCross {
		t.Fatalf("cross-module import edge missing: %+v", graph.Edges)
	}
}

func TestModuleGraphOnAnUnmappedWorkspaceIsEmpty(t *testing.T) {
	m := newTestMapper(t)
	graph, err := m.ModuleGraph(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 0 || len(graph.Edges) != 0 {
		t.Fatalf("unmapped workspace produced a graph: %+v", graph)
	}
}
