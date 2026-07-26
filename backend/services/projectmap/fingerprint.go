package projectmap

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"claude_suite/backend/models"
)

// ChangeClass says how much a file changed since its last fingerprint —
// the decision that keeps incremental updates at zero token cost.
type ChangeClass int

const (
	// ChangeNone: identical content.
	ChangeNone ChangeClass = iota
	// ChangeCosmetic: content differs, structure signature identical —
	// formatting, comments, string literals. No re-extraction needed.
	ChangeCosmetic
	// ChangeStructural: the structural skeleton moved, or the file has no
	// extractor so nothing can prove it did not.
	ChangeStructural
)

func ContentHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// StructureSig hashes the sorted signature lines. "" means "no extractor" and
// classifies every change as STRUCTURAL — the conservative default ported
// from Understand-Anything.
func StructureSig(ex *Extraction) string {
	lines := ex.StructureStrings()
	if lines == nil {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

// Classify compares a file against its stored fingerprint.
func Classify(old models.FileFingerprint, newHash, newSig string) ChangeClass {
	if old.ContentHash == newHash {
		return ChangeNone
	}
	if old.StructureSig != "" && old.StructureSig == newSig {
		return ChangeCosmetic
	}
	return ChangeStructural
}
