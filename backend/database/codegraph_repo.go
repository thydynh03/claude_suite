package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"claude_suite/backend/models"
)

// CodeGraphRepository stores the project map — the relational form of the
// Understand-Anything knowledge-graph schema. Everything is keyed per
// workspace so several projects share one database without mixing.
type CodeGraphRepository struct {
	db *sql.DB
	// hasFTS caches whether the optional code_nodes_fts index exists in this
	// database — SearchNodes prefers MATCH, falls back to LIKE without it.
	hasFTS bool
}

func NewCodeGraphRepository(db *sql.DB) *CodeGraphRepository {
	r := &CodeGraphRepository{db: db}
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'code_nodes_fts'`,
	).Scan(&n); err == nil && n > 0 {
		r.hasFTS = true
	}
	return r
}

// ReplaceGraph swaps a workspace's entire node/edge set in one transaction —
// the full-build path. Incremental updates use UpsertNodes/DeleteFile instead.
func (r *CodeGraphRepository) ReplaceGraph(ws string, nodes []models.CodeNode, edges []models.CodeEdge) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM code_nodes WHERE workspace_id = ?`, ws); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM code_edges WHERE workspace_id = ?`, ws); err != nil {
		return err
	}
	if err := upsertNodesTx(tx, ws, nodes); err != nil {
		return err
	}
	if err := upsertEdgesTx(tx, ws, edges); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *CodeGraphRepository) UpsertNodes(ws string, nodes []models.CodeNode) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := upsertNodesTx(tx, ws, nodes); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *CodeGraphRepository) UpsertEdges(ws string, edges []models.CodeEdge) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := upsertEdgesTx(tx, ws, edges); err != nil {
		return err
	}
	return tx.Commit()
}

func upsertNodesTx(tx *sql.Tx, ws string, nodes []models.CodeNode) error {
	stmt, err := tx.Prepare(
		`INSERT INTO code_nodes (node_id, workspace_id, kind, name, file_path, line_start, line_end, summary, tags, summary_stale, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(workspace_id, node_id) DO UPDATE SET
		   kind = excluded.kind, name = excluded.name, file_path = excluded.file_path,
		   line_start = excluded.line_start, line_end = excluded.line_end,
		   summary = excluded.summary, tags = excluded.tags,
		   summary_stale = excluded.summary_stale, updated_at = excluded.updated_at`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now()
	for _, n := range nodes {
		tags, _ := json.Marshal(n.Tags)
		stale := 0
		if n.SummaryStale {
			stale = 1
		}
		if _, err := stmt.Exec(n.NodeID, ws, n.Kind, n.Name, n.FilePath, n.LineStart, n.LineEnd, n.Summary, string(tags), stale, now); err != nil {
			return err
		}
	}
	return nil
}

func upsertEdgesTx(tx *sql.Tx, ws string, edges []models.CodeEdge) error {
	stmt, err := tx.Prepare(
		`INSERT INTO code_edges (workspace_id, source, target, kind, weight) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(workspace_id, source, target, kind) DO UPDATE SET weight = excluded.weight`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, e := range edges {
		if _, err := stmt.Exec(ws, e.Source, e.Target, e.Kind, e.Weight); err != nil {
			return err
		}
	}
	return nil
}

// DeleteFile removes a file's nodes and every edge touching them — the
// referential-integrity step of the ported Understand-Anything validate pass.
func (r *CodeGraphRepository) DeleteFile(ws, filePath string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(
		`DELETE FROM code_edges WHERE workspace_id = ?1 AND (
		   source IN (SELECT node_id FROM code_nodes WHERE workspace_id = ?1 AND file_path = ?2)
		   OR target IN (SELECT node_id FROM code_nodes WHERE workspace_id = ?1 AND file_path = ?2)
		 )`, ws, filePath); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM code_nodes WHERE workspace_id = ? AND file_path = ?`, ws, filePath); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM file_fingerprints WHERE workspace_id = ? AND file_path = ?`, ws, filePath); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *CodeGraphRepository) GetFingerprints(ws string) (map[string]models.FileFingerprint, error) {
	rows, err := r.db.Query(
		`SELECT workspace_id, file_path, content_hash, structure_sig, total_lines, updated_at FROM file_fingerprints WHERE workspace_id = ?`, ws)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]models.FileFingerprint)
	for rows.Next() {
		var f models.FileFingerprint
		var rawUpdated interface{}
		if err := rows.Scan(&f.WorkspaceID, &f.FilePath, &f.ContentHash, &f.StructureSig, &f.TotalLines, &rawUpdated); err != nil {
			return nil, err
		}
		f.UpdatedAt = parseTimeValue(rawUpdated)
		out[f.FilePath] = f
	}
	return out, rows.Err()
}

func (r *CodeGraphRepository) PutFingerprints(ws string, fps []models.FileFingerprint) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(
		`INSERT INTO file_fingerprints (workspace_id, file_path, content_hash, structure_sig, total_lines, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(workspace_id, file_path) DO UPDATE SET
		   content_hash = excluded.content_hash, structure_sig = excluded.structure_sig,
		   total_lines = excluded.total_lines, updated_at = excluded.updated_at`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now()
	for _, f := range fps {
		if _, err := stmt.Exec(ws, f.FilePath, f.ContentHash, f.StructureSig, f.TotalLines, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetMeta returns (nil, nil) when the workspace has never been analyzed.
func (r *CodeGraphRepository) GetMeta(ws string) (*models.ProjectMeta, error) {
	var m models.ProjectMeta
	var rawAnalyzed interface{}
	err := r.db.QueryRow(
		`SELECT workspace_id, git_commit_hash, analyzed_at, analyzed_files, schema_version FROM project_meta WHERE workspace_id = ?`, ws,
	).Scan(&m.WorkspaceID, &m.GitCommitHash, &rawAnalyzed, &m.AnalyzedFiles, &m.SchemaVersion)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m.AnalyzedAt = parseTimeValue(rawAnalyzed)
	return &m, nil
}

// PutMeta refuses to record "analyzed" while the fingerprint store is empty.
// Ported invariant (Understand-Anything issue #152): meta's commit hash must
// never claim a freshness the fingerprints cannot back — write fingerprints
// first, meta second, and a wiped fingerprint store forces a full rebuild
// instead of silently corrupting incremental updates.
func (r *CodeGraphRepository) PutMeta(m *models.ProjectMeta) error {
	var fingerprints int
	if err := r.db.QueryRow(
		`SELECT COUNT(*) FROM file_fingerprints WHERE workspace_id = ?`, m.WorkspaceID,
	).Scan(&fingerprints); err != nil {
		return err
	}
	if m.AnalyzedFiles > 0 && fingerprints == 0 {
		return fmt.Errorf("refusing to write project_meta for %s: %d files claimed analyzed but the fingerprint store is empty (write fingerprints first)", m.WorkspaceID, m.AnalyzedFiles)
	}
	if m.SchemaVersion == "" {
		m.SchemaVersion = "1.0.0"
	}
	_, err := r.db.Exec(
		`INSERT INTO project_meta (workspace_id, git_commit_hash, analyzed_at, analyzed_files, schema_version)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(workspace_id) DO UPDATE SET
		   git_commit_hash = excluded.git_commit_hash, analyzed_at = excluded.analyzed_at,
		   analyzed_files = excluded.analyzed_files, schema_version = excluded.schema_version`,
		m.WorkspaceID, m.GitCommitHash, m.AnalyzedAt, m.AnalyzedFiles, m.SchemaVersion,
	)
	return err
}

// SearchNodes returns candidate nodes matching any query token: full-text
// (FTS5) when the optional index exists, LIKE otherwise. Either way this is a
// prefilter — callers apply the whole-word rule (textutil.KeywordMatch) on
// top, or SQL matching alone would reintroduce the "DATABASE" matches "BA"
// class of bug.
func (r *CodeGraphRepository) SearchNodes(ws string, tokens []string, limit int) ([]models.CodeNode, error) {
	if len(tokens) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 200
	}

	if r.hasFTS {
		nodes, err := r.searchNodesFTS(ws, tokens, limit)
		if err == nil {
			return nodes, nil
		}
		// A malformed MATCH expression must degrade, not fail the pack build.
	}
	return r.searchNodesLike(ws, tokens, limit)
}

func (r *CodeGraphRepository) searchNodesFTS(ws string, tokens []string, limit int) ([]models.CodeNode, error) {
	quoted := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		// Quoting each token neutralizes FTS5 query syntax (-, *, NEAR, …).
		quoted = append(quoted, `"`+strings.ReplaceAll(tok, `"`, ``)+`"`)
	}
	match := strings.Join(quoted, " OR ")

	rows, err := r.db.Query(
		`SELECT n.node_id, n.workspace_id, n.kind, n.name, n.file_path, n.line_start, n.line_end, n.summary, n.tags, n.summary_stale
		 FROM code_nodes_fts f
		 JOIN code_nodes n ON n.workspace_id = f.workspace_id AND n.node_id = f.node_id
		 WHERE f.workspace_id = ? AND code_nodes_fts MATCH ?
		 LIMIT ?`, ws, match, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCodeNodes(rows)
}

func (r *CodeGraphRepository) searchNodesLike(ws string, tokens []string, limit int) ([]models.CodeNode, error) {
	var conds []string
	var args []interface{}
	args = append(args, ws)
	for _, tok := range tokens {
		pattern := "%" + strings.ToLower(tok) + "%"
		conds = append(conds, `(LOWER(name) LIKE ? OR LOWER(file_path) LIKE ? OR LOWER(summary) LIKE ? OR LOWER(tags) LIKE ?)`)
		args = append(args, pattern, pattern, pattern, pattern)
	}
	args = append(args, limit)

	rows, err := r.db.Query(
		`SELECT node_id, workspace_id, kind, name, file_path, line_start, line_end, summary, tags, summary_stale
		 FROM code_nodes WHERE workspace_id = ? AND (`+strings.Join(conds, " OR ")+`) LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCodeNodes(rows)
}

// EdgesByKind returns a workspace's edges of one kind — the module-graph
// aggregation reads the imports layer.
func (r *CodeGraphRepository) EdgesByKind(ws, kind string, limit int) ([]models.CodeEdge, error) {
	if limit <= 0 {
		limit = 2000
	}
	rows, err := r.db.Query(
		`SELECT workspace_id, source, target, kind, weight FROM code_edges WHERE workspace_id = ? AND kind = ? LIMIT ?`,
		ws, kind, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []models.CodeEdge
	for rows.Next() {
		var e models.CodeEdge
		if err := rows.Scan(&e.WorkspaceID, &e.Source, &e.Target, &e.Kind, &e.Weight); err != nil {
			return nil, err
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// NodesByKind returns a workspace's nodes of one kind.
func (r *CodeGraphRepository) NodesByKind(ws, kind string, limit int) ([]models.CodeNode, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := r.db.Query(
		`SELECT node_id, workspace_id, kind, name, file_path, line_start, line_end, summary, tags, summary_stale
		 FROM code_nodes WHERE workspace_id = ? AND kind = ? ORDER BY node_id LIMIT ?`, ws, kind, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCodeNodes(rows)
}

// Neighbors returns the edges touching the given nodes and the nodes on the
// far end — the 1-hop expansion of the ported context-builder pattern.
func (r *CodeGraphRepository) Neighbors(ws string, nodeIDs []string, limit int) ([]models.CodeEdge, []models.CodeNode, error) {
	if len(nodeIDs) == 0 {
		return nil, nil, nil
	}
	if limit <= 0 {
		limit = 100
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(nodeIDs)), ",")
	args := make([]interface{}, 0, len(nodeIDs)*2+2)
	args = append(args, ws)
	for _, id := range nodeIDs {
		args = append(args, id)
	}
	for _, id := range nodeIDs {
		args = append(args, id)
	}
	args = append(args, limit)

	rows, err := r.db.Query(
		`SELECT workspace_id, source, target, kind, weight FROM code_edges
		 WHERE workspace_id = ? AND (source IN (`+placeholders+`) OR target IN (`+placeholders+`)) LIMIT ?`, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var edges []models.CodeEdge
	far := make(map[string]bool)
	seed := make(map[string]bool)
	for _, id := range nodeIDs {
		seed[id] = true
	}
	for rows.Next() {
		var e models.CodeEdge
		if err := rows.Scan(&e.WorkspaceID, &e.Source, &e.Target, &e.Kind, &e.Weight); err != nil {
			return nil, nil, err
		}
		edges = append(edges, e)
		if !seed[e.Source] {
			far[e.Source] = true
		}
		if !seed[e.Target] {
			far[e.Target] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var farNodes []models.CodeNode
	if len(far) > 0 {
		ids := make([]string, 0, len(far))
		for id := range far {
			ids = append(ids, id)
		}
		farNodes, err = r.GetNodes(ws, ids)
		if err != nil {
			return nil, nil, err
		}
	}
	return edges, farNodes, nil
}

func (r *CodeGraphRepository) GetNodes(ws string, nodeIDs []string) ([]models.CodeNode, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(nodeIDs)), ",")
	args := make([]interface{}, 0, len(nodeIDs)+1)
	args = append(args, ws)
	for _, id := range nodeIDs {
		args = append(args, id)
	}
	rows, err := r.db.Query(
		`SELECT node_id, workspace_id, kind, name, file_path, line_start, line_end, summary, tags, summary_stale
		 FROM code_nodes WHERE workspace_id = ? AND node_id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCodeNodes(rows)
}

// StaleFileNodes returns file-level nodes whose summaries await LLM
// enrichment, oldest path first so refresh order is deterministic.
func (r *CodeGraphRepository) StaleFileNodes(ws string, limit int) ([]models.CodeNode, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.db.Query(
		`SELECT node_id, workspace_id, kind, name, file_path, line_start, line_end, summary, tags, summary_stale
		 FROM code_nodes WHERE workspace_id = ? AND summary_stale = 1 AND kind IN ('file','config')
		 ORDER BY file_path LIMIT ?`, ws, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCodeNodes(rows)
}

func (r *CodeGraphRepository) StaleSummaryCount(ws string) (int, error) {
	var n int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM code_nodes WHERE workspace_id = ? AND summary_stale = 1 AND kind IN ('file','config')`, ws,
	).Scan(&n)
	return n, err
}

// ApplySummaries writes LLM summaries and clears the stale flag for every id
// in freshIDs — including ones the model skipped, so a stubborn file cannot
// wedge the refresh loop forever.
func (r *CodeGraphRepository) ApplySummaries(ws string, updates []models.SummaryUpdate, freshIDs []string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, u := range updates {
		tags, _ := json.Marshal(u.Tags)
		if _, err := tx.Exec(
			`UPDATE code_nodes SET summary = ?, tags = ?, summary_stale = 0, updated_at = ? WHERE workspace_id = ? AND node_id = ?`,
			u.Summary, string(tags), time.Now(), ws, u.NodeID,
		); err != nil {
			return err
		}
	}
	for _, id := range freshIDs {
		if _, err := tx.Exec(
			`UPDATE code_nodes SET summary_stale = 0 WHERE workspace_id = ? AND node_id = ?`, ws, id,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Counts reports node/edge/file totals for the stats view.
func (r *CodeGraphRepository) Counts(ws string) (nodes, edges, files int, err error) {
	if err = r.db.QueryRow(`SELECT COUNT(*) FROM code_nodes WHERE workspace_id = ?`, ws).Scan(&nodes); err != nil {
		return
	}
	if err = r.db.QueryRow(`SELECT COUNT(*) FROM code_edges WHERE workspace_id = ?`, ws).Scan(&edges); err != nil {
		return
	}
	err = r.db.QueryRow(`SELECT COUNT(*) FROM file_fingerprints WHERE workspace_id = ?`, ws).Scan(&files)
	return
}

func scanCodeNodes(rows *sql.Rows) ([]models.CodeNode, error) {
	var out []models.CodeNode
	for rows.Next() {
		var n models.CodeNode
		var tagsJSON string
		var stale int
		if err := rows.Scan(&n.NodeID, &n.WorkspaceID, &n.Kind, &n.Name, &n.FilePath, &n.LineStart, &n.LineEnd, &n.Summary, &tagsJSON, &stale); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(tagsJSON), &n.Tags)
		n.SummaryStale = stale != 0
		out = append(out, n)
	}
	return out, rows.Err()
}
