package biz

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// FactFingerprint computes a deterministic SHA-256 fingerprint for a fact,
// keyed by (normalized statement, scopeType, scopeID). This is the single
// source of truth for fingerprint computation — the data layer must call
// this function instead of maintaining a parallel implementation.
// Uses NormalizeForDedup for statement normalization to ensure consistency
// with cross-layer dedup comparisons.
func FactFingerprint(statement, scopeType, scopeID string) string {
	n := NormalizeForDedup(statement)
	h := sha256.Sum256([]byte(n + "\x00" + strings.TrimSpace(scopeType) + "\x00" + strings.TrimSpace(scopeID)))
	return fmt.Sprintf("%x", h[:])
}

const statementTrailingPunct = " \t。！？.!?,;；，、"

// NormalizeStatementPunctuation trims surrounding space and trailing sentence
// punctuation so write-path statements share a fingerprint with near-duplicates
// that only differ by a period / 句号.
func NormalizeStatementPunctuation(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), statementTrailingPunct)
}

// NormalizeForDedup normalizes a string for cross-layer dedup comparison:
// lowercase, trim, collapse whitespace, strip trailing punctuation.
func NormalizeForDedup(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !prevSpace {
				b.WriteRune(' ')
				prevSpace = true
			}
		} else {
			b.WriteRune(r)
			prevSpace = false
		}
	}
	return strings.TrimRight(strings.TrimSpace(b.String()), statementTrailingPunct)
}

// DedupL3WithL1 filters L3 fact rows whose normalized statement matches any
// L1 field value. Both l3Rows and the returned slice contain raw JSON bytes.
// Rows that fail JSON parsing are retained but checked via raw string search
// as a best-effort fallback so they are not silently excluded from dedup.
func DedupL3WithL1(l3Rows [][]byte, l1FieldValues []string) [][]byte {
	if len(l1FieldValues) == 0 || len(l3Rows) == 0 {
		return l3Rows
	}
	l1Set := make(map[string]struct{}, len(l1FieldValues))
	for _, v := range l1FieldValues {
		key := NormalizeForDedup(v)
		if key != "" {
			l1Set[key] = struct{}{}
		}
	}
	if len(l1Set) == 0 {
		return l3Rows
	}
	out := make([][]byte, 0, len(l3Rows))
	for _, raw := range l3Rows {
		var row map[string]any
		if json.Unmarshal(raw, &row) != nil {
			// JSON parse failed: attempt to extract "statement" field via regex
			// to avoid false positives from substring matching against the
			// entire raw JSON (e.g. L1 value "like" matching field name "like").
			stmtFallback := extractStatementFromCorruptJSON(raw)
			if stmtFallback != "" {
				stmt := NormalizeForDedup(stmtFallback)
				if _, dup := l1Set[stmt]; dup {
					continue
				}
			}
			// If statement extraction also fails, conservatively retain the
			// row rather than risk a false-positive dedup on raw JSON.
			out = append(out, raw)
			continue
		}
		stmt := NormalizeForDedup(fmt.Sprint(row["statement"]))
		if _, dup := l1Set[stmt]; dup {
			continue
		}
		out = append(out, raw)
	}
	return out
}

// stmtFieldRe extracts the "statement" field value from a potentially corrupt
// JSON byte slice using regex, avoiding false positives from substring matching
// against the entire raw JSON.
var stmtFieldRe = regexp.MustCompile(`"statement"\s*:\s*"((?:[^"\\]|\\.)*)"`)

// extractStatementFromCorruptJSON attempts to extract the "statement" field
// value from a corrupt JSON byte slice using regex. Returns empty string if
// the field cannot be found or unescaped.
func extractStatementFromCorruptJSON(raw []byte) string {
	m := stmtFieldRe.FindSubmatch(raw)
	if len(m) < 2 {
		return ""
	}
	// Unescape JSON string
	var s string
	if json.Unmarshal(m[1], &s) == nil {
		return s
	}
	return string(m[1])
}
