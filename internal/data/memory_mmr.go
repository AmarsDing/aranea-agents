package data

import (
	"sort"
	"strings"
)

// mmrRerankTexts reorders candidate indices using Maximal Marginal Relevance
// (MMR) to balance relevance against diversity. It uses word-set Jaccard
// similarity as the redundancy measure.
//
// texts: candidate text passages (statement/summary for scoring).
// scores: relevance scores for each candidate (higher = more relevant).
// limit: maximum number of indices to return.
// lambda: trade-off parameter in [0,1]. λ=1 → pure relevance; λ=0 → pure diversity.
//
// Returns indices into texts/scores in MMR-selected order.
func mmrRerankTexts(texts []string, scores []float64, limit int, lambda float64) []int {
	n := len(texts)
	if n == 0 || limit <= 0 {
		return nil
	}
	if limit > n {
		limit = n
	}
	if lambda < 0 {
		lambda = 0
	}
	if lambda > 1 {
		lambda = 1
	}

	// Precompute word sets for Jaccard similarity.
	wordSets := make([]map[string]struct{}, n)
	for i, text := range texts {
		wordSets[i] = tokenizeToWordSet(text)
	}

	// Track which candidates have been selected.
	selected := make([]bool, n)
	var result []int

	for len(result) < limit {
		bestIdx := -1
		bestScore := -1e18

		for i := 0; i < n; i++ {
			if selected[i] {
				continue
			}
			// Compute max similarity to already-selected items.
			maxSim := 0.0
			for j := 0; j < n; j++ {
				if !selected[j] {
					continue
				}
				sim := jaccardWordSetFromSets(wordSets[i], wordSets[j])
				if sim > maxSim {
					maxSim = sim
				}
			}
			// MMR score: λ*relevance - (1-λ)*maxSimilarity
			mmrScore := lambda*scores[i] - (1-lambda)*maxSim
			if mmrScore > bestScore {
				bestScore = mmrScore
				bestIdx = i
			}
		}

		if bestIdx < 0 {
			break
		}
		selected[bestIdx] = true
		result = append(result, bestIdx)
	}

	return result
}

// tokenizeToWordSet converts text to a set of lowercase words.
func tokenizeToWordSet(text string) map[string]struct{} {
	if text == "" {
		return nil
	}
	words := strings.Fields(strings.ToLower(text))
	set := make(map[string]struct{}, len(words))
	for _, w := range words {
		// Strip common punctuation from word edges.
		w = strings.Trim(w, ".,;:!?()[]{}\"'")
		if len(w) >= 2 {
			set[w] = struct{}{}
		}
	}
	return set
}

// jaccardWordSet computes Jaccard similarity between two texts using
// their word sets.
func jaccardWordSet(a, b string) float64 {
	return jaccardWordSetFromSets(tokenizeToWordSet(a), tokenizeToWordSet(b))
}

// jaccardWordSetFromSets computes Jaccard similarity from pre-built word sets.
func jaccardWordSetFromSets(a, b map[string]struct{}) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	intersection := 0
	for w := range a {
		if _, ok := b[w]; ok {
			intersection++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// sortByMMRScore is a helper for debugging: sorts indices by their MMR score.
func sortByMMRScore(indices []int, scores []float64) {
	sort.Slice(indices, func(i, j int) bool {
		return scores[indices[i]] > scores[indices[j]]
	})
}
