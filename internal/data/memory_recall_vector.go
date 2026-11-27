package data

import (
	"context"

	"aranea-agents/internal/data/vector"
)

// recallVectorMinScore is the cosine floor for the recall vector legs.
const recallVectorMinScore = 0.3

// searchRecallVectors runs one recall vector leg against the shared read
// index (vector_embeddings, written by memoryFactIndexSync with agent_id /
// user_id meta). The table holds every user's and agent's facts, so with a
// filter the search is restricted to those rows; a global top-K followed by
// the SQL scope filter leaves almost nothing for one user once many share the
// table (AMC eval: hundreds of users). Stores without filter support (test
// doubles) and empty filters fall back to the unfiltered Search.
func searchRecallVectors(ctx context.Context, vs vector.VectorStore, embedding []float32, topK int, filter map[string]string) ([]vector.VectorHit, error) {
	if fs, ok := vs.(vector.FilteredVectorStore); ok && len(filter) > 0 {
		return fs.SearchFiltered(ctx, float32To64(embedding), topK, recallVectorMinScore, filter)
	}
	return vs.Search(ctx, float32To64(embedding), topK, recallVectorMinScore)
}

// keepInSet returns the ids present in set, preserving order.
func keepInSet(ids []string, set map[string]struct{}) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := set[id]; ok {
			out = append(out, id)
		}
	}
	return out
}

// uniqueIDs returns the union of the lists in first-seen order.
func uniqueIDs(lists ...[]string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, ids := range lists {
		for _, id := range ids {
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}
