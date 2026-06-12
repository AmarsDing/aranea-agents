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
