package evaluation

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"sort"
)

// HashCases computes a stable content hash of dataset cases (P3-5 / P2 versions).
// Cases are sorted by ID so the digest depends only on content.
func HashCases(cases []Case) string {
	sorted := make([]Case, len(cases))
	copy(sorted, cases)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	h := sha256.New()
	for _, c := range sorted {
		io.WriteString(h, c.ID)
		h.Write([]byte{0})
		io.WriteString(h, c.Input)
		h.Write([]byte{0})
		io.WriteString(h, c.ExpectedOutput)
		h.Write([]byte{0})
		io.WriteString(h, c.MetadataJSON)
		h.Write([]byte{0})
	}
	if len(sorted) == 0 {
		return hex.EncodeToString(h.Sum(nil))[:16]
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
