package evaluation

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"sort"

	"aranea-agents/internal/biz"
)

// hashEvalCases computes a stable content hash of a dataset's cases (P3-5).
// Cases are sorted by ID so the hash depends only on content, not listing
// order. The digest is truncated to 16 hex chars (64 bits) — sufficient for
// change detection, short enough for display.
func hashEvalCases(cases []biz.EvalCase) string {
	sorted := make([]biz.EvalCase, len(cases))
	copy(sorted, cases)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	h := sha256.New()
	for _, c := range sorted {
		// NUL separators prevent field-boundary ambiguity (e.g. ["ab","c"]
		// vs ["a","bc"] hashing identically).
		io.WriteString(h, c.ID)
		h.Write([]byte{0})
		io.WriteString(h, c.Input)
		h.Write([]byte{0})
		io.WriteString(h, c.ExpectedOutput)
		h.Write([]byte{0})
		io.WriteString(h, c.MetadataJSON)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
