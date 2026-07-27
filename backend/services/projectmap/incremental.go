package projectmap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent_center/backend/models"
)

// IncrementalUpdate re-fingerprints just the changed paths and patches the
// graph. Cost model (ported from Understand-Anything): NONE = free, COSMETIC =
// hash update only, STRUCTURAL = deterministic re-extraction of that one file.
// No LLM tokens are ever spent here.
//
// It is a no-op until a map exists — the first build is an explicit user
// action, not a per-task surprise. Serialized per workspace by the same lock
// FullBuild takes, so parallel task completions queue instead of racing.
func (m *Mapper) IncrementalUpdate(workspaceDir string, changed []string) error {
	if workspaceDir == "" || len(changed) == 0 {
		return nil
	}
	ws, err := m.ws.EnsureWorkspace(workspaceDir, gitRemoteURL(workspaceDir))
	if err != nil {
		return err
	}
	lock := m.buildLock(ws.WorkspaceID)
	lock.Lock()
	defer lock.Unlock()

	meta, err := m.graph.GetMeta(ws.WorkspaceID)
	if err != nil || meta == nil {
		return err // no map yet — nothing to keep fresh
	}
	fps, err := m.graph.GetFingerprints(ws.WorkspaceID)
	if err != nil {
		return err
	}
	// Ported LOAD-PATCH-SAVE guard: a wiped fingerprint store under a meta row
	// that claims analysis means state is corrupt — rebuild from scratch
	// rather than patching on top of nothing.
	if len(fps) == 0 && meta.AnalyzedFiles > 0 {
		_, err := m.FullBuild(workspaceDir)
		return err
	}

	modulePath := goModulePath(workspaceDir)
	scanned := make(map[string]bool, len(fps))
	for p := range fps {
		scanned[p] = true
	}

	var updatedFps []models.FileFingerprint
	for _, raw := range changed {
		rel := filepath.ToSlash(strings.TrimSpace(raw))
		if rel == "" || !mappableFile(rel) {
			continue
		}
		scanned[rel] = true

		full := filepath.Join(workspaceDir, filepath.FromSlash(rel))
		info, statErr := os.Stat(full)
		if statErr != nil {
			// Deleted (or unreadable): drop its nodes, edges and fingerprint.
			if err := m.graph.DeleteFile(ws.WorkspaceID, rel); err != nil {
				return err
			}
			delete(scanned, rel)
			continue
		}
		if info.Size() > maxScannedFileSize {
			continue
		}

		content, readErr := os.ReadFile(full)
		if readErr != nil {
			continue
		}
		lang := languageByExt[strings.ToLower(filepath.Ext(rel))]
		ex := Extract(lang, content, rel)
		newHash := ContentHash(content)
		newSig := StructureSig(ex)
		lines := strings.Count(string(content), "\n") + 1

		change := ChangeStructural // new files have no old fingerprint
		if old, known := fps[rel]; known {
			change = Classify(old, newHash, newSig)
		}
		if change == ChangeNone {
			continue
		}

		updatedFps = append(updatedFps, models.FileFingerprint{
			WorkspaceID:  ws.WorkspaceID,
			FilePath:     rel,
			ContentHash:  newHash,
			StructureSig: newSig,
			TotalLines:   lines,
		})
		if change == ChangeCosmetic {
			continue // structure identical — the stored nodes stay correct
		}

		// STRUCTURAL: replace this file's slice of the graph.
		if err := m.graph.DeleteFile(ws.WorkspaceID, rel); err != nil {
			return err
		}
		sf := ScannedFile{Path: rel, Language: lang, Size: info.Size()}
		nodes, edges := buildFileNodes(sf, ex, lines, modulePath, workspaceDir, scanned)

		dir := filepath.ToSlash(filepath.Dir(rel))
		moduleName := dir
		if dir == "." {
			moduleName = "(root)"
		}
		nodes = append(nodes, models.CodeNode{
			NodeID: "module:" + dir, Kind: "module", Name: moduleName,
			Summary: fmt.Sprintf("thư mục chứa %s", rel),
		})
		edges = append(edges, models.CodeEdge{
			Source: "module:" + dir, Target: "file:" + rel, Kind: "contains", Weight: 1.0,
		})

		if err := m.graph.UpsertNodes(ws.WorkspaceID, nodes); err != nil {
			return err
		}
		if err := m.graph.UpsertEdges(ws.WorkspaceID, dropDanglingIncremental(m, ws.WorkspaceID, nodes, edges)); err != nil {
			return err
		}
	}

	if len(updatedFps) == 0 {
		return nil
	}
	// Fingerprints first, meta second — same ordering invariant as FullBuild.
	if err := m.graph.PutFingerprints(ws.WorkspaceID, updatedFps); err != nil {
		return err
	}
	if err := m.graph.PutMeta(&models.ProjectMeta{
		WorkspaceID:   ws.WorkspaceID,
		GitCommitHash: gitHeadCommit(workspaceDir),
		AnalyzedAt:    time.Now(),
		AnalyzedFiles: len(scanned),
	}); err != nil {
		return err
	}
	m.maybeAutoSummarize(workspaceDir, ws.WorkspaceID)
	return nil
}

// mappableFile mirrors the scan filter for a single path.
func mappableFile(rel string) bool {
	for _, part := range strings.Split(filepath.ToSlash(filepath.Dir(rel)), "/") {
		if skippedDirs[part] || strings.HasPrefix(part, ".trash") {
			return false
		}
	}
	ext := strings.ToLower(filepath.Ext(rel))
	if _, known := languageByExt[ext]; !known {
		return false
	}
	base := strings.ToLower(filepath.Base(rel))
	return base != "package-lock.json" && base != "pnpm-lock.yaml" && !strings.HasSuffix(base, ".min.js")
}

// dropDanglingIncremental keeps an edge when both ends exist either in this
// batch or already in the stored graph — the incremental analogue of the
// full-build referential-integrity pass.
func dropDanglingIncremental(m *Mapper, ws string, nodes []models.CodeNode, edges []models.CodeEdge) []models.CodeEdge {
	local := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		local[n.NodeID] = true
	}

	var unknown []string
	for _, e := range edges {
		if !local[e.Source] {
			unknown = append(unknown, e.Source)
		}
		if !local[e.Target] {
			unknown = append(unknown, e.Target)
		}
	}
	stored := make(map[string]bool)
	if len(unknown) > 0 {
		if found, err := m.graph.GetNodes(ws, unknown); err == nil {
			for _, n := range found {
				stored[n.NodeID] = true
			}
		}
	}

	kept := edges[:0]
	for _, e := range edges {
		if (local[e.Source] || stored[e.Source]) && (local[e.Target] || stored[e.Target]) {
			kept = append(kept, e)
		}
	}
	return kept
}
