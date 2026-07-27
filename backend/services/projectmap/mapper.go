package projectmap

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"agent_center/backend/database"
	"agent_center/backend/models"
	"agent_center/backend/sysproc"
)

// Mapper builds and serves the project map. Git runs through sysproc (never
// bare exec.Command — a console window per call on Windows) and stays inside
// this package; importing services here would be an import cycle.
type Mapper struct {
	graph *database.CodeGraphRepository
	ws    *database.WorkspaceRepository

	mu     sync.Mutex
	builds map[string]*sync.Mutex // per-workspace build serialization

	summarizer summarizerState
}

func NewMapper(graph *database.CodeGraphRepository, ws *database.WorkspaceRepository) *Mapper {
	return &Mapper{graph: graph, ws: ws, builds: make(map[string]*sync.Mutex)}
}

func (m *Mapper) buildLock(workspaceID string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.builds[workspaceID]; !ok {
		m.builds[workspaceID] = &sync.Mutex{}
	}
	return m.builds[workspaceID]
}

// BuildReport summarizes one FullBuild for logs and the UI.
type BuildReport struct {
	WorkspaceID string        `json:"workspace_id"`
	Files       int           `json:"files"`
	Nodes       int           `json:"nodes"`
	Edges       int           `json:"edges"`
	Duration    time.Duration `json:"-"`
	DurationSec float64       `json:"duration_sec"`
}

// FullBuild scans the workspace, extracts structure, and replaces the stored
// graph. Zero LLM tokens: summaries are signature lines until the M3 summary
// pass refreshes them. Write order is load-bearing — fingerprints BEFORE meta
// (ported Understand-Anything invariant, their issue #152).
func (m *Mapper) FullBuild(workspaceDir string) (*BuildReport, error) {
	start := time.Now()
	ws, err := m.ws.EnsureWorkspace(workspaceDir, gitRemoteURL(workspaceDir))
	if err != nil {
		return nil, err
	}
	lock := m.buildLock(ws.WorkspaceID)
	lock.Lock()
	defer lock.Unlock()

	files, err := ScanWorkspace(workspaceDir)
	if err != nil {
		return nil, err
	}

	modulePath := goModulePath(workspaceDir)
	var nodes []models.CodeNode
	var edges []models.CodeEdge
	var fps []models.FileFingerprint
	scannedPaths := make(map[string]bool, len(files))
	dirs := make(map[string]int)

	for _, f := range files {
		scannedPaths[f.Path] = true
	}

	for _, f := range files {
		content, err := ReadFile(workspaceDir, f.Path)
		if err != nil {
			continue
		}
		ex := Extract(f.Language, content, f.Path)
		lines := bytes.Count(content, []byte("\n")) + 1

		fps = append(fps, models.FileFingerprint{
			WorkspaceID:  ws.WorkspaceID,
			FilePath:     f.Path,
			ContentHash:  ContentHash(content),
			StructureSig: StructureSig(ex),
			TotalLines:   lines,
		})

		fileNodes, fileEdges := buildFileNodes(f, ex, lines, modulePath, workspaceDir, scannedPaths)
		nodes = append(nodes, fileNodes...)
		edges = append(edges, fileEdges...)

		dir := filepath.ToSlash(filepath.Dir(f.Path))
		dirs[dir]++
	}

	// One module node per directory, containing its files.
	for dir, count := range dirs {
		name := dir
		if dir == "." {
			name = "(root)"
		}
		nodes = append(nodes, models.CodeNode{
			NodeID:  "module:" + dir,
			Kind:    "module",
			Name:    name,
			Summary: fmt.Sprintf("thư mục với %d file được map", count),
		})
	}
	for _, f := range files {
		dir := filepath.ToSlash(filepath.Dir(f.Path))
		edges = append(edges, models.CodeEdge{
			Source: "module:" + dir, Target: "file:" + f.Path, Kind: "contains", Weight: 1.0,
		})
	}

	edges = dropDanglingEdges(nodes, edges)

	if err := m.graph.ReplaceGraph(ws.WorkspaceID, nodes, edges); err != nil {
		return nil, err
	}
	// Fingerprints FIRST, meta SECOND — meta's commit hash must never claim a
	// freshness the fingerprints cannot back.
	if err := m.graph.PutFingerprints(ws.WorkspaceID, fps); err != nil {
		return nil, err
	}
	if err := m.graph.PutMeta(&models.ProjectMeta{
		WorkspaceID:   ws.WorkspaceID,
		GitCommitHash: gitHeadCommit(workspaceDir),
		AnalyzedAt:    time.Now(),
		AnalyzedFiles: len(files),
	}); err != nil {
		return nil, err
	}

	report := &BuildReport{
		WorkspaceID: ws.WorkspaceID,
		Files:       len(files),
		Nodes:       len(nodes),
		Edges:       len(edges),
		Duration:    time.Since(start),
		DurationSec: time.Since(start).Seconds(),
	}
	m.writeProjectMapMarkdown(workspaceDir, ws.WorkspaceID, report, dirs)
	return report, nil
}

// buildFileNodes produces the file node, its function/class children, and its
// resolvable import edges.
func buildFileNodes(f ScannedFile, ex *Extraction, lines int, modulePath, workspaceDir string, scanned map[string]bool) ([]models.CodeNode, []models.CodeEdge) {
	var nodes []models.CodeNode
	var edges []models.CodeEdge

	fileID := "file:" + f.Path
	kind := "file"
	if f.Language == "config" {
		kind = "config"
	}
	summary := fmt.Sprintf("%d dòng (%s)", lines, f.Language)
	if ex != nil {
		summary = fmt.Sprintf("%d dòng, %d hàm, %d type (%s)", lines, len(ex.Functions), len(ex.Classes), f.Language)
	}
	nodes = append(nodes, models.CodeNode{
		NodeID:   fileID,
		Kind:     kind,
		Name:     filepath.Base(f.Path),
		FilePath: f.Path,
		LineEnd:  lines,
		Summary:  summary,
		Tags:     []string{f.Language},
		// File-level summaries start as bare counts; the summarizer enriches
		// them in batches. Function/class nodes keep their signature
		// summaries — those are already exact.
		SummaryStale: true,
	})

	if ex == nil {
		return nodes, edges
	}

	for _, fn := range ex.Functions {
		id := "function:" + f.Path + ":" + fn.Name
		nodes = append(nodes, models.CodeNode{
			NodeID:    id,
			Kind:      "function",
			Name:      fn.Name,
			FilePath:  f.Path,
			LineStart: fn.LineStart,
			LineEnd:   fn.LineEnd,
			Summary:   fn.Signature,
			Tags:      []string{f.Language},
		})
		edges = append(edges, models.CodeEdge{Source: fileID, Target: id, Kind: "contains", Weight: 1.0})
	}
	for _, c := range ex.Classes {
		id := "class:" + f.Path + ":" + c.Name
		nodes = append(nodes, models.CodeNode{
			NodeID:    id,
			Kind:      "class",
			Name:      c.Name,
			FilePath:  f.Path,
			LineStart: c.LineStart,
			LineEnd:   c.LineEnd,
			Summary:   c.Kind + " " + c.Name,
			Tags:      []string{f.Language},
		})
		edges = append(edges, models.CodeEdge{Source: fileID, Target: id, Kind: "contains", Weight: 1.0})
	}

	for _, imp := range ex.Imports {
		if target := resolveImport(imp, f.Path, modulePath, scanned); target != "" {
			edges = append(edges, models.CodeEdge{Source: fileID, Target: target, Kind: "imports", Weight: 0.7})
		}
	}
	return nodes, edges
}

// resolveImport maps an import string onto a node id inside the workspace.
// External dependencies resolve to "" and are dropped — the map is about this
// project, not the module proxy.
func resolveImport(imp, fromPath, modulePath string, scanned map[string]bool) string {
	// Go: an internal package import maps to its directory's module node.
	if modulePath != "" && (imp == modulePath || strings.HasPrefix(imp, modulePath+"/")) {
		dir := strings.TrimPrefix(imp, modulePath)
		dir = strings.TrimPrefix(dir, "/")
		if dir == "" {
			dir = "."
		}
		return "module:" + dir
	}

	// TS/JS/Svelte: relative imports resolve to sibling files.
	if strings.HasPrefix(imp, "./") || strings.HasPrefix(imp, "../") {
		base := filepath.ToSlash(filepath.Join(filepath.Dir(fromPath), imp))
		candidates := []string{
			base,
			base + ".ts", base + ".tsx", base + ".js", base + ".jsx", base + ".svelte",
			base + "/index.ts", base + "/index.js",
		}
		for _, c := range candidates {
			c = filepath.ToSlash(filepath.Clean(c))
			if scanned[c] {
				return "file:" + c
			}
		}
	}
	return ""
}

// dropDanglingEdges enforces referential integrity before anything is stored —
// the ported validate step: an edge to a node that does not exist renders as a
// lie in the pack.
func dropDanglingEdges(nodes []models.CodeNode, edges []models.CodeEdge) []models.CodeEdge {
	ids := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		ids[n.NodeID] = true
	}
	kept := edges[:0]
	for _, e := range edges {
		if ids[e.Source] && ids[e.Target] {
			kept = append(kept, e)
		}
	}
	return kept
}

// Staleness reports how far the stored map lags the working tree.
func (m *Mapper) Staleness(workspaceDir string) models.StalenessReport {
	report := models.StalenessReport{Status: "unknown"}

	ws, err := m.ws.EnsureWorkspace(workspaceDir, gitRemoteURL(workspaceDir))
	if err != nil {
		return report
	}
	meta, err := m.graph.GetMeta(ws.WorkspaceID)
	if err != nil || meta == nil {
		return report
	}
	report.GraphCommit = meta.GitCommitHash

	head := gitHeadCommit(workspaceDir)
	if head == "" {
		return report
	}
	report.HeadCommit = head
	report.Dirty = gitIsDirty(workspaceDir)

	if meta.GitCommitHash == head {
		if report.Dirty {
			report.Status = "dirty"
		} else {
			report.Status = "fresh"
		}
		return report
	}

	report.Status = "stale"
	report.CommitsBehind = gitCommitsBetween(workspaceDir, meta.GitCommitHash, head)
	return report
}

// Stats reports what the UI shows about the map.
func (m *Mapper) Stats(workspaceDir string) (*models.ProjectMapStats, error) {
	ws, err := m.ws.EnsureWorkspace(workspaceDir, gitRemoteURL(workspaceDir))
	if err != nil {
		return nil, err
	}
	nodes, edges, files, err := m.graph.Counts(ws.WorkspaceID)
	if err != nil {
		return nil, err
	}
	stats := &models.ProjectMapStats{
		WorkspaceID: ws.WorkspaceID,
		Nodes:       nodes,
		Edges:       edges,
		Files:       files,
		Staleness:   m.Staleness(workspaceDir),
	}
	if meta, err := m.graph.GetMeta(ws.WorkspaceID); err == nil && meta != nil {
		stats.AnalyzedAt = meta.AnalyzedAt
		stats.GitCommitHash = meta.GitCommitHash
	}
	return stats, nil
}

// writeProjectMapMarkdown renders a human/CLI-readable digest into the
// workspace's .agent-center dir so the Claude CLI can @-reference it and the
// Plan Builder can inject it. Best-effort: a write failure never fails a build.
func (m *Mapper) writeProjectMapMarkdown(workspaceDir, workspaceID string, report *BuildReport, dirs map[string]int) {
	var b strings.Builder
	b.WriteString("# Project Map\n\n")
	b.WriteString(fmt.Sprintf("Sinh tự động bởi Agent Center (workspace %s) — %d file, %d node, %d edge.\n", workspaceID, report.Files, report.Nodes, report.Edges))
	b.WriteString("KHÔNG sửa tay: file này bị ghi đè sau mỗi lần build lại project map.\n\n## Thư mục\n")

	type dirCount struct {
		dir   string
		count int
	}
	sorted := make([]dirCount, 0, len(dirs))
	for d, c := range dirs {
		sorted = append(sorted, dirCount{d, c})
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].count != sorted[j].count {
			return sorted[i].count > sorted[j].count
		}
		return sorted[i].dir < sorted[j].dir
	})
	for i, dc := range sorted {
		if i >= 40 {
			b.WriteString(fmt.Sprintf("- … và %d thư mục khác\n", len(sorted)-i))
			break
		}
		b.WriteString(fmt.Sprintf("- `%s` — %d file\n", dc.dir, dc.count))
	}

	dir := filepath.Join(workspaceDir, ".agent-center")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "project-map.md"), []byte(b.String()), 0o644)
}

// ── git helpers (sysproc only) ─────────────────────────────────────────────

func gitOutput(dir string, args ...string) string {
	cmd := sysproc.Command("git", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(out.String())
}

// WorkspaceRemoteIdentity resolves the remote-based identity source for a
// workspace directory. It is THE resolver — the orchestrator's observation
// capture and the mapper must agree on it or the same folder's memory splits
// across two workspace rows.
//
// Rules, each one verified against a live failure mode on this machine:
//   - Not inside a git work tree → "": outside a repo, `git config --get
//     remote.origin.url` falls back to the user's GLOBAL config, and a value
//     stored there would give every non-repo folder the same identity.
//   - Workspace IS the repo toplevel → the normalized remote (identity
//     follows the repo across clones).
//   - Workspace is DEEPER than the toplevel (monorepo sub-folder — or a
//     stray `git init` in the user's home directory swallowing everything
//     under it) → remote + "#" + sub-path, so sibling sub-workspaces do not
//     merge their memories into one.
func WorkspaceRemoteIdentity(dir string) string {
	if dir == "" {
		return ""
	}
	toplevel := gitOutput(dir, "rev-parse", "--show-toplevel")
	if toplevel == "" {
		return ""
	}
	remote := gitOutput(dir, "config", "--get", "remote.origin.url")
	if remote == "" {
		return ""
	}
	identity := database.NormalizeRemoteURL(remote)

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return identity
	}
	top := strings.ToLower(filepath.Clean(filepath.FromSlash(toplevel)))
	cur := strings.ToLower(filepath.Clean(absDir))
	if rel, err := filepath.Rel(top, cur); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		identity += "#" + filepath.ToSlash(rel)
	}
	return identity
}

func gitRemoteURL(dir string) string {
	return WorkspaceRemoteIdentity(dir)
}

func gitHeadCommit(dir string) string {
	if dir == "" {
		return ""
	}
	return gitOutput(dir, "rev-parse", "HEAD")
}

var porcelainAgentCenterRe = regexp.MustCompile(`(?m)^..\s+\.agent-center/`)

func gitIsDirty(dir string) bool {
	out := gitOutput(dir, "status", "--porcelain")
	// The map's own artifacts must not count as dirt, or every build marks
	// the workspace dirty forever.
	out = porcelainAgentCenterRe.ReplaceAllString(out, "")
	return strings.TrimSpace(out) != ""
}

func gitCommitsBetween(dir, from, to string) int {
	if from == "" || to == "" {
		return 0
	}
	out := gitOutput(dir, "rev-list", "--count", from+".."+to)
	n, err := strconv.Atoi(out)
	if err != nil {
		return 0
	}
	return n
}

// goModulePath reads the module path from go.mod, "" when absent.
func goModulePath(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}
