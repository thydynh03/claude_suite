package models

import "time"

// CodeNode is one element of a workspace's project map. Node IDs are
// type-prefixed (`file:<rel-path>`, `function:<rel-path>:<name>`,
// `class:<rel-path>:<name>`, `module:<dir>`) — the convention ported from
// Understand-Anything that makes concurrent and incremental merges
// deterministic. FilePath is always workspace-relative with forward slashes.
type CodeNode struct {
	NodeID       string    `json:"node_id"`
	WorkspaceID  string    `json:"workspace_id"`
	Kind         string    `json:"kind"` // file|function|class|config|module|layer
	Name         string    `json:"name"`
	FilePath     string    `json:"file_path"`
	LineStart    int       `json:"line_start"`
	LineEnd      int       `json:"line_end"`
	Summary      string    `json:"summary"`
	Tags         []string  `json:"tags"`
	SummaryStale bool      `json:"summary_stale"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type CodeEdge struct {
	WorkspaceID string  `json:"workspace_id"`
	Source      string  `json:"source"`
	Target      string  `json:"target"`
	Kind        string  `json:"kind"` // imports|contains|calls|depends_on|related
	Weight      float64 `json:"weight"`
}

// FileFingerprint classifies changes without spending tokens: an identical
// ContentHash means NONE, a changed hash with an identical StructureSig means
// COSMETIC (formatting/comments), a changed StructureSig means STRUCTURAL.
// An empty StructureSig means no extractor exists for the file — such files
// are conservatively treated as STRUCTURAL on any change.
type FileFingerprint struct {
	WorkspaceID  string    `json:"workspace_id"`
	FilePath     string    `json:"file_path"`
	ContentHash  string    `json:"content_hash"`
	StructureSig string    `json:"structure_sig"`
	TotalLines   int       `json:"total_lines"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ProjectMeta struct {
	WorkspaceID   string    `json:"workspace_id"`
	GitCommitHash string    `json:"git_commit_hash"`
	AnalyzedAt    time.Time `json:"analyzed_at"`
	AnalyzedFiles int       `json:"analyzed_files"`
	SchemaVersion string    `json:"schema_version"`
}

// StalenessReport says how much the project map can be trusted right now.
type StalenessReport struct {
	// Status: fresh (map matches HEAD, tree clean), dirty (map matches HEAD,
	// uncommitted changes exist), stale (HEAD moved since the map was built),
	// unknown (no map yet, or not a git repo).
	Status        string `json:"status"`
	GraphCommit   string `json:"graph_commit"`
	HeadCommit    string `json:"head_commit"`
	CommitsBehind int    `json:"commits_behind"`
	Dirty         bool   `json:"dirty"`
}

// SummaryUpdate is one LLM-written enrichment applied to a node.
type SummaryUpdate struct {
	NodeID  string   `json:"id"`
	Summary string   `json:"summary"`
	Tags    []string `json:"tags"`
}

// ModuleGraph is the directory-level aggregation of the map — small enough
// for the UI to draw whole: one node per module (directory), one edge per
// import relationship, weighted by how many file imports it aggregates.
type ModuleGraph struct {
	Nodes []ModuleGraphNode `json:"nodes"`
	Edges []ModuleGraphEdge `json:"edges"`
}

type ModuleGraphNode struct {
	ID    string `json:"id"`   // module:<dir>
	Name  string `json:"name"` // dir path, "(root)" for "."
	Files int    `json:"files"`
}

type ModuleGraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Count  int    `json:"count"` // number of file-level imports aggregated
}

// ProjectMapStats is what the UI shows about a workspace's map.
type ProjectMapStats struct {
	WorkspaceID   string          `json:"workspace_id"`
	Nodes         int             `json:"nodes"`
	Edges         int             `json:"edges"`
	Files         int             `json:"files"`
	AnalyzedAt    time.Time       `json:"analyzed_at"`
	GitCommitHash string          `json:"git_commit_hash"`
	Staleness     StalenessReport `json:"staleness"`
}
