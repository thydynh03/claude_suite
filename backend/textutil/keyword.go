package textutil

import (
	"regexp"
	"strings"
	"sync"
)

// kwCache keeps one compiled regexp per keyword. Callers like the project-map
// renderer match many nodes against the same few tokens per task dispatch;
// recompiling per call would dominate the matching cost.
var kwCache sync.Map // keyword → *regexp.Regexp

// KeywordMatch reports whether kw occurs in text as a whole word (single
// tokens) or as a substring (multi-word phrases like "WEB TEST").
//
// Whole-word matching is the project rule for short tags: with a substring
// check, "DATABASE" contains "BA" and routed database work to the Business
// Analyst. It lives in textutil because both orchestrator and database-side
// consumers need it, and either importing orchestrator would be a cycle.
func KeywordMatch(text, kw string) bool {
	if strings.Contains(kw, " ") {
		return strings.Contains(text, kw)
	}
	if cached, ok := kwCache.Load(kw); ok {
		return cached.(*regexp.Regexp).MatchString(text)
	}
	re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(kw) + `\b`)
	kwCache.Store(kw, re)
	return re.MatchString(text)
}
