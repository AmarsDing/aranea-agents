package data

import (
	"math"
	"sort"
	"strings"
)

type evalExcerptWindow struct{ start, end int }

type evalExcerptIndex struct {
	runes   []rune
	lower   []rune
	tokens  []string
	weights []float64
	centers []int
}

// Window positions use rune counts from the lowercased string, never byte
// offsets into the original text. Go case folding keeps one rune per rune.
// Unique-token coverage prevents an early generic match displacing specific
// evidence at the tail; a second window can retain a distant supporting row.
func evalRelevantWindows(content string, queries []string, budget int) string {
	runes := []rune(content)
	if budget <= 0 {
		return ""
	}
	if len(runes) <= budget {
		return content
	}
	if budget < 24 {
		return string(runes[:budget])
	}
	index := newEvalExcerptIndex(content, runes, queries)
	return index.render(index.windows(budget))
}

func newEvalExcerptIndex(content string, runes []rune, queries []string) *evalExcerptIndex {
	low := strings.ToLower(content)
	lowRunes := []rune(low)
	var tokens []string
	for _, query := range queries {
		tokens = append(tokens, evalMeaningfulTokens(query)...)
	}
	tokens = uniqueNonEmpty(tokens, 64)
	weights := make([]float64, len(tokens))
	centers := []int{0}
	for i, token := range tokens {
		token = strings.ToLower(token)
		tokens[i] = token
		count := strings.Count(low, token)
		if count == 0 {
			continue
		}
		weights[i] = 1 / math.Log2(float64(count)+2)
		for offset, hits := 0, 0; offset < len(low) && hits < 3 && len(centers) < 128; hits++ {
			idx := strings.Index(low[offset:], token)
			if idx < 0 {
				break
			}
			idx += offset
			centers = append(centers, len([]rune(low[:idx])))
			offset = idx + len(token)
		}
		if idx := strings.LastIndex(low, token); idx >= 0 && len(centers) < 128 {
			centers = append(centers, len([]rune(low[:idx])))
		}
	}
	return &evalExcerptIndex{runes: runes, lower: lowRunes, tokens: tokens, weights: weights, centers: centers}
}

func (x *evalExcerptIndex) best(size int, exclude *evalExcerptWindow, covered map[string]bool) evalExcerptWindow {
	chosen := evalExcerptWindow{0, size}
	bestScore := -1.0
	for _, center := range x.centers {
		start := center - size/3
		if start < 0 {
			start = 0
		}
		if start+size > len(x.runes) {
			start = len(x.runes) - size
		}
		window := evalExcerptWindow{start, start + size}
		if exclude != nil && window.start < exclude.end && window.end > exclude.start {
			continue
		}
		text := string(x.lower[window.start:window.end])
		score := 0.0
		for i, token := range x.tokens {
			if !covered[token] && strings.Contains(text, token) {
				score += x.weights[i]
			}
		}
		if score > bestScore {
			bestScore = score
			chosen = window
		}
	}
	return chosen
}

func (x *evalExcerptIndex) windows(budget int) []evalExcerptWindow {
	size := budget - 16 // Reserve all omission markers inside the caller's budget.
	first := x.best(size, nil, nil)
	windows := []evalExcerptWindow{first}
	covered := map[string]bool{}
	for _, token := range x.tokens {
		covered[token] = strings.Contains(string(x.lower[first.start:first.end]), token)
	}
	missing := false
	for i, token := range x.tokens {
		missing = missing || x.weights[i] > 0 && !covered[token]
	}
	if missing && budget >= 160 {
		first = x.best(size/2, nil, nil)
		for _, token := range x.tokens {
			covered[token] = strings.Contains(string(x.lower[first.start:first.end]), token)
		}
		second := x.best(size/2, &first, covered)
		if second.end <= first.start || second.start >= first.end {
			windows = []evalExcerptWindow{first, second}
		}
	}
	sort.Slice(windows, func(i, j int) bool { return windows[i].start < windows[j].start })
	return windows
}

func (x *evalExcerptIndex) render(windows []evalExcerptWindow) string {
	var out strings.Builder
	for i, window := range windows {
		if window.start > 0 && (i == 0 || windows[i-1].end < window.start) {
			out.WriteString("[…] ")
		}
		out.WriteString(string(x.runes[window.start:window.end]))
	}
	if windows[len(windows)-1].end < len(x.runes) {
		out.WriteString(" […]")
	}
	return out.String()
}
