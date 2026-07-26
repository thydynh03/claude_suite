package projectmap

import (
	"path/filepath"
	"sort"
	"strings"

	"claude_suite/backend/models"
)

// moduleGraphMaxNodes bounds what the UI is asked to draw. Modules are ranked
// by file count; the long tail is dropped, and the drop is announced by the
// caller (no silent truncation).
const moduleGraphMaxNodes = 30

// ModuleGraph aggregates the file-level import edges up to directory level —
// the whole-project view the Memory page draws.
func (m *Mapper) ModuleGraph(workspaceDir string) (*models.ModuleGraph, error) {
	ws, err := m.ws.EnsureWorkspace(workspaceDir, gitRemoteURL(workspaceDir))
	if err != nil {
		return nil, err
	}

	modules, err := m.graph.NodesByKind(ws.WorkspaceID, "module", 500)
	if err != nil {
		return nil, err
	}
	if len(modules) == 0 {
		return &models.ModuleGraph{}, nil
	}

	// File counts come from the contains edges module→file.
	contains, err := m.graph.EdgesByKind(ws.WorkspaceID, "contains", 20000)
	if err != nil {
		return nil, err
	}
	fileCount := make(map[string]int)
	fileModule := make(map[string]string) // file node id → module node id
	for _, e := range contains {
		if strings.HasPrefix(e.Source, "module:") && strings.HasPrefix(e.Target, "file:") {
			fileCount[e.Source]++
			fileModule[e.Target] = e.Source
		}
	}

	// Rank modules and keep the top slice.
	sort.SliceStable(modules, func(i, j int) bool {
		if fileCount[modules[i].NodeID] != fileCount[modules[j].NodeID] {
			return fileCount[modules[i].NodeID] > fileCount[modules[j].NodeID]
		}
		return modules[i].NodeID < modules[j].NodeID
	})
	if len(modules) > moduleGraphMaxNodes {
		modules = modules[:moduleGraphMaxNodes]
	}
	kept := make(map[string]bool, len(modules))
	graph := &models.ModuleGraph{}
	for _, n := range modules {
		kept[n.NodeID] = true
		graph.Nodes = append(graph.Nodes, models.ModuleGraphNode{
			ID:    n.NodeID,
			Name:  n.Name,
			Files: fileCount[n.NodeID],
		})
	}

	// Aggregate imports: file→module and file→file both resolve to
	// module→module; self-imports and dropped modules are skipped.
	imports, err := m.graph.EdgesByKind(ws.WorkspaceID, "imports", 20000)
	if err != nil {
		return nil, err
	}
	counts := make(map[[2]string]int)
	for _, e := range imports {
		src := moduleOf(e.Source, fileModule)
		dst := moduleOf(e.Target, fileModule)
		if src == "" || dst == "" || src == dst || !kept[src] || !kept[dst] {
			continue
		}
		counts[[2]string{src, dst}]++
	}
	for pair, n := range counts {
		graph.Edges = append(graph.Edges, models.ModuleGraphEdge{Source: pair[0], Target: pair[1], Count: n})
	}
	sort.Slice(graph.Edges, func(i, j int) bool {
		if graph.Edges[i].Count != graph.Edges[j].Count {
			return graph.Edges[i].Count > graph.Edges[j].Count
		}
		return graph.Edges[i].Source+graph.Edges[i].Target < graph.Edges[j].Source+graph.Edges[j].Target
	})
	return graph, nil
}

func moduleOf(nodeID string, fileModule map[string]string) string {
	if strings.HasPrefix(nodeID, "module:") {
		return nodeID
	}
	if strings.HasPrefix(nodeID, "file:") {
		if m, ok := fileModule[nodeID]; ok {
			return m
		}
		// Derive from the path when the contains edge was not loaded.
		return "module:" + filepath.ToSlash(filepath.Dir(strings.TrimPrefix(nodeID, "file:")))
	}
	return ""
}
