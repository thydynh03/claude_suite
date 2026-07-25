// Package textutil trims strings to a size budget without corrupting them.
//
// Slicing a Go string by byte index cuts multi-byte characters in half. The
// pieces that come out are no longer valid UTF-8, and this project moves that
// text somewhere it shows: page content sent to the UI, a git diff handed to the
// model to write a commit message, build output fed back to an agent so it can
// fix the error. Vietnamese text corrupts on almost every cut.
package textutil

import "unicode/utf8"

// Truncate returns s limited to at most max bytes, cut on a character boundary
// so the result stays valid UTF-8. When s is cut, suffix is appended — and the
// suffix is not counted against max, because these budgets bound payload size
// rather than describing an exact length.
//
// A max of zero or less, or a string already within budget, is returned as is.
func Truncate(s string, max int, suffix string) string {
	if max <= 0 || len(s) <= max {
		return s
	}

	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + suffix
}

// TruncateTail keeps the last max bytes of s, starting on a character boundary.
// Use it where the end of the text is the useful part — a compiler's error
// summary sits at the bottom of its output.
func TruncateTail(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}

	start := len(s) - max
	for start < len(s) && !utf8.RuneStart(s[start]) {
		start++
	}
	return s[start:]
}
