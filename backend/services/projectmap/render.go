package projectmap

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"claude_suite/backend/models"
	"claude_suite/backend/textutil"
)

const (
	renderMaxSeeds = 12
	renderMaxNodes = 30
)

// renderStopwords are tokens too generic to seed node matching — matching on
// them would drag half the graph into every pack. Vietnamese task titles are
// normal here, so common Vietnamese words are stopwords too.
var renderStopwords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "this": true,
	"that": true, "task": true, "file": true, "files": true, "code": true,
	"fix": true, "add": true, "new": true, "update": true, "create": true,
	"make": true, "run": true, "use": true, "into": true, "from": true,
	"các": true, "cho": true, "của": true, "trong": true, "với": true,
	"tạo": true, "sửa": true, "thêm": true, "một": true, "được": true,
	"lỗi": true, "chạy": true, "dùng": true, "không": true,
}

var (
	tokenRe = regexp.MustCompile(`[A-Za-zÀ-ỹ0-9_]{3,}`)
	pathRe  = regexp.MustCompile(`[A-Za-z0-9_./\\-]+\.[A-Za-z0-9]{1,10}`)
)

// RenderPack renders the task-relevant slice of the map: seed nodes matched
// from the task text (whole-word rule), expanded one hop, bounded by budget.
// Ported shape: Understand-Anything's context-builder (seed → 1-hop → render).
func (m *Mapper) RenderPack(workspaceDir string, task *models.Task, budget int) string {
	if workspaceDir == "" || task == nil || budget <= 0 {
		return ""
	}
	ws, err := m.ws.EnsureWorkspace(workspaceDir, gitRemoteURL(workspaceDir))
	if err != nil {
		return ""
	}
	meta, err := m.graph.GetMeta(ws.WorkspaceID)
	if err != nil || meta == nil {
		return "" // no map yet — nothing to say beats saying something wrong
	}

	taskText := task.Title + " " + task.Prompt + " " + task.Description
	tokens := extractTokens(taskText)
	seeds := m.selectSeeds(ws.WorkspaceID, taskText, tokens)
	if len(seeds) == 0 {
		return ""
	}

	seedIDs := make([]string, 0, len(seeds))
	for _, n := range seeds {
		seedIDs = append(seedIDs, n.NodeID)
	}
	edges, farNodes, err := m.graph.Neighbors(ws.WorkspaceID, seedIDs, 80)
	if err != nil {
		edges, farNodes = nil, nil
	}

	all := seeds
	for _, n := range farNodes {
		if len(all) >= renderMaxNodes {
			break
		}
		all = append(all, n)
	}

	var b strings.Builder
	staleness := m.Staleness(workspaceDir)
	b.WriteString(fmt.Sprintf("## Project map (commit %s, %s)\n", shortCommit(meta.GitCommitHash), staleness.Status))
	if staleness.Status == "stale" || staleness.Status == "dirty" {
		b.WriteString(fmt.Sprintf("NOTE: map is %d commit(s) behind HEAD; trust the filesystem over this map where they disagree.\n", staleness.CommitsBehind))
	}
	b.WriteString("### Relevant components\n")
	for _, n := range all {
		loc := ""
		if n.FilePath != "" && n.LineStart > 0 {
			loc = fmt.Sprintf(" (%s:%d-%d)", n.FilePath, n.LineStart, n.LineEnd)
		} else if n.FilePath != "" {
			loc = " (" + n.FilePath + ")"
		}
		b.WriteString(fmt.Sprintf("- %s — %s%s\n", n.NodeID, textutil.Truncate(n.Summary, 160, "…"), loc))
	}
	if len(edges) > 0 {
		b.WriteString("### Relationships\n")
		shown := 0
		for _, e := range edges {
			if shown >= 20 {
				break
			}
			if e.Kind == "contains" {
				continue // implied by the node ids; imports/calls carry the signal
			}
			b.WriteString(fmt.Sprintf("- %s --[%s]--> %s\n", e.Source, e.Kind, e.Target))
			shown++
		}
	}

	return textutil.Truncate(b.String(), budget, "\n--- [project map truncated at budget] ---")
}

// selectSeeds finds the nodes the task is talking about: exact file paths
// first, then whole-word token matches ranked by how many tokens hit.
func (m *Mapper) selectSeeds(workspaceID, taskText string, tokens []string) []models.CodeNode {
	seen := make(map[string]bool)
	var seeds []models.CodeNode

	// Literal file mentions outrank keyword guesses.
	for _, raw := range pathRe.FindAllString(taskText, -1) {
		p := strings.ReplaceAll(raw, "\\", "/")
		p = strings.TrimPrefix(p, "./")
		nodes, err := m.graph.SearchNodes(workspaceID, []string{p}, 5)
		if err != nil {
			continue
		}
		for _, n := range nodes {
			if n.Kind != "file" && n.Kind != "config" {
				continue
			}
			if strings.HasSuffix(n.FilePath, p) && !seen[n.NodeID] {
				seen[n.NodeID] = true
				seeds = append(seeds, n)
			}
		}
	}

	if len(tokens) > 0 {
		candidates, err := m.graph.SearchNodes(workspaceID, tokens, 300)
		if err == nil {
			type scored struct {
				node  models.CodeNode
				score int
			}
			var ranked []scored
			for _, n := range candidates {
				if seen[n.NodeID] {
					continue
				}
				haystack := n.Name + " " + n.FilePath + " " + n.Summary
				score := 0
				for _, tok := range tokens {
					// Whole-word on top of the SQL LIKE prefilter — LIKE alone
					// is the substring bug class ("DATABASE" matches "BA").
					if textutil.KeywordMatch(haystack, tok) {
						score++
					}
				}
				if score > 0 {
					ranked = append(ranked, scored{n, score})
				}
			}
			sort.SliceStable(ranked, func(i, j int) bool {
				if ranked[i].score != ranked[j].score {
					return ranked[i].score > ranked[j].score
				}
				return ranked[i].node.NodeID < ranked[j].node.NodeID
			})
			for _, r := range ranked {
				if len(seeds) >= renderMaxSeeds {
					break
				}
				seen[r.node.NodeID] = true
				seeds = append(seeds, r.node)
			}
		}
	}

	if len(seeds) > renderMaxSeeds {
		seeds = seeds[:renderMaxSeeds]
	}
	return seeds
}

func extractTokens(text string) []string {
	seen := make(map[string]bool)
	var tokens []string
	for _, tok := range tokenRe.FindAllString(text, -1) {
		low := strings.ToLower(tok)
		if renderStopwords[low] || seen[low] {
			continue
		}
		seen[low] = true
		tokens = append(tokens, low)
		if len(tokens) >= 12 {
			break
		}
	}
	return tokens
}

func shortCommit(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	if hash == "" {
		return "unknown"
	}
	return hash
}
