package biz

import (
	"strings"
)

// Reranker scores query-passage relevance for memory recall.
// Implementations may use lexical similarity, Cross-Encoder models, or external APIs.
// Stability:evolving
type Reranker interface {
	// Score returns a relevance score between 0 and 1 for the query-passage pair.
	Score(query, passage string) float64
}

// CrossEncoderReranker scores query-passage relevance (lexical proxy until external CE model is wired).
type CrossEncoderReranker struct{}

func NewCrossEncoderReranker() *CrossEncoderReranker { return &CrossEncoderReranker{} }

// Compile-time check: CrossEncoderReranker implements Reranker.
var _ Reranker = (*CrossEncoderReranker)(nil)

func (CrossEncoderReranker) Score(query, passage string) float64 {
	return bigramJaccard(strings.ToLower(strings.TrimSpace(query)), strings.ToLower(strings.TrimSpace(passage)))
}

func bigramJaccard(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	ta := strings.Fields(a)
	tb := strings.Fields(b)
	// A single-token side can never intersect word-pair bigrams; fall back to
	// unigram-set Jaccard so the reranker yields signal instead of a
	// systematic false 0 (which scales recall Total by 0.85 for no reason).
	if len(ta) < 2 || len(tb) < 2 {
		return unigramJaccard(ta, tb)
	}
	ga := wordBigrams(a)
	gb := wordBigrams(b)
	if len(ga) == 0 || len(gb) == 0 {
		return 0
	}
	inter := 0
	for k := range ga {
		if _, ok := gb[k]; ok {
			inter++
		}
	}
	union := len(ga) + len(gb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// unigramJaccard computes Jaccard similarity over token sets.
func unigramJaccard(ta, tb []string) float64 {
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}
	setB := make(map[string]struct{}, len(tb))
	for _, t := range tb {
		setB[t] = struct{}{}
	}
	setA := make(map[string]struct{}, len(ta))
	inter := 0
	for _, t := range ta {
		if _, dup := setA[t]; dup {
			continue
		}
		setA[t] = struct{}{}
		if _, ok := setB[t]; ok {
			inter++
		}
	}
	union := len(setA) + len(setB) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func wordBigrams(text string) map[string]struct{} {
	tokens := strings.Fields(text)
	out := make(map[string]struct{})
	if len(tokens) == 1 {
		out[tokens[0]] = struct{}{}
		return out
	}
	for i := 0; i < len(tokens)-1; i++ {
		out[tokens[i]+" "+tokens[i+1]] = struct{}{}
	}
	return out
}
