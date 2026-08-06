package data

import "aranea-agents/internal/biz"

// MMR implementation moved to biz (P2-R1): the composite layered recall path
// reranks merged L2+L3 hits in the usecase layer. These wrappers keep the
// legacy data-layer call sites (composite adapter, tests) unchanged.

func mmrRerankTexts(texts []string, scores []float64, limit int, lambda float64) []int {
	return biz.MMRRerankTexts(texts, scores, limit, lambda)
}

// jaccardWordSet computes Jaccard similarity between two texts using
// their word sets.
func jaccardWordSet(a, b string) float64 {
	return biz.JaccardWordSet(a, b)
}

// sortByMMRScore is a helper for debugging: sorts indices by their MMR score.
func sortByMMRScore(indices []int, scores []float64) {
	biz.SortByMMRScore(indices, scores)
}
