package agent

import (
	"strings"
	"sync"
)

// DoomLoopDetector identifies repetitive LLM output (the "doom loop" failure
// mode where a model emits the same text repeatedly). It tracks consecutive
// similar texts and reports detection once `threshold` in a row exceed the
// similarity threshold. Pure in-memory, no I/O.
type DoomLoopDetector struct {
	mu           sync.Mutex
	window       []string
	threshold    int
	simThreshold float64
	consecutive  int
}

// NewDoomLoopDetector creates a detector. threshold is the number of
// consecutive similar texts that signals a doom loop (default 3);
// similarityThreshold is the word-Jaccard similarity floor in (0,1]
// (default 0.95).
func NewDoomLoopDetector(threshold int, similarityThreshold float64) *DoomLoopDetector {
	if threshold <= 0 {
		threshold = 3
	}
	if similarityThreshold <= 0 || similarityThreshold > 1 {
		similarityThreshold = 0.95
	}
	return &DoomLoopDetector{
		threshold:    threshold,
		simThreshold: similarityThreshold,
		consecutive:  1,
	}
}

// Observe records one text block and returns true when the last `threshold`
// consecutive blocks are pairwise similar above the similarity threshold.
// Empty/whitespace-only blocks are ignored (never poison the window).
func (d *DoomLoopDetector) Observe(text string) bool {
	norm := normalizeForDoomLoop(text)
	if norm == "" {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.window) > 0 {
		if wordJaccard(d.window[len(d.window)-1], norm) >= d.simThreshold {
			d.consecutive++
		} else {
			d.consecutive = 1
		}
	}
	d.window = append(d.window, norm)
	if len(d.window) > d.threshold {
		d.window = d.window[1:]
	}
	return d.consecutive >= d.threshold
}

func normalizeForDoomLoop(text string) string {
	return strings.Join(strings.Fields(strings.ToLower(text)), " ")
}

// wordJaccard computes Jaccard similarity over whitespace-delimited word sets.
// Identical strings fast-path to 1.0.
func wordJaccard(a, b string) float64 {
	if a == b {
		return 1.0
	}
	sa := make(map[string]struct{})
	for _, w := range strings.Fields(a) {
		sa[w] = struct{}{}
	}
	sb := make(map[string]struct{})
	for _, w := range strings.Fields(b) {
		sb[w] = struct{}{}
	}
	inter := 0
	for w := range sa {
		if _, ok := sb[w]; ok {
			inter++
		}
	}
	union := len(sa) + len(sb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}
