package textutil

import "regexp"

// Secret scrubbing for captured observations. Go's RE2 engine is linear-time
// by construction, so the catastrophic-backtracking incident class the
// original ECC regex was hardened against cannot occur here.
//
// The ECC pattern alone is NOT enough: applied to "Authorization: Bearer
// eyJ…", its `\S+` redacts only the word "Bearer" and the actual token
// survives. Header-shaped keys therefore redact to end of line, and a bare
// "Bearer <token>" anywhere is caught separately.
var (
	scrubHeaderRe = regexp.MustCompile(`(?i)(authorization|proxy-authorization|x-api-key|x-goog-api-key)(["']?\s*[:=]\s*)[^\r\n]+`)
	scrubKVRe     = regexp.MustCompile(`(?i)(api[_-]?key|token|secret|password|credentials?|auth)(["']?\s*[:=]\s*)((?:Bearer|Basic)\s+)?["']?[^\s"']+`)
	scrubBearerRe = regexp.MustCompile(`(?i)\b(?:Bearer|Basic)\s+[A-Za-z0-9._~+/=-]+`)
)

// Scrub redacts credential-shaped values from s. Applied to every observation
// payload BEFORE truncation and storage — memory must never become the place
// a token survives after the terminal scrolled past it.
func Scrub(s string) string {
	s = scrubHeaderRe.ReplaceAllString(s, "$1$2[REDACTED]")
	s = scrubKVRe.ReplaceAllString(s, "$1$2[REDACTED]")
	return scrubBearerRe.ReplaceAllString(s, "[REDACTED]")
}
