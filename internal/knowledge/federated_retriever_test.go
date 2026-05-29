package knowledge

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
)

func TestFederatedRetrieverSingleCollection(t *testing.T) {
	t.Log("single collection delegates to router/retriever directly")
}

func TestFederatedRetrieverMergeOrder(t *testing.T) {
	chunks := []biz.KnowledgeChunk{
		{ID: "a-1", CollectionID: "coll-a", Score: 0.9},
		{ID: "b-1", CollectionID: "coll-b", Score: 0.8},
		{ID: "a-2", CollectionID: "coll-a", Score: 0.7},
		{ID: "b-2", CollectionID: "coll-b", Score: 0.5},
		{ID: "a-3", CollectionID: "coll-a", Score: 0.3},
	}
	merged := MergeSearchResults(nil, chunks, 3)
	if len(merged) != 3 {
		t.Errorf("expected 3 chunks after topK merge, got %d", len(merged))
	}
	if merged[0].Score < merged[1].Score {
		t.Errorf("chunks not sorted by score desc: %f >= %f expected", merged[0].Score, merged[1].Score)
	}
}

func TestFederatedRetrieverEmptyCollections(t *testing.T) {
	chunks := []biz.KnowledgeChunk{}
	merged := MergeSearchResults(nil, chunks, 5)
	if len(merged) != 0 {
		t.Errorf("expected 0 chunks, got %d", len(merged))
	}
}

type mockMetaFetcher struct {
	collections []biz.KnowledgeCollection
}

func (m *mockMetaFetcher) ListCollections(_ context.Context, _ string, _, _ int) ([]biz.KnowledgeCollection, int, error) {
	return m.collections, len(m.collections), nil
}

func TestRouteCollections(t *testing.T) {
	meta := &mockMetaFetcher{
		collections: []biz.KnowledgeCollection{
			{ID: "col-1", Name: "refund policy", Description: "refund and return policies for products"},
			{ID: "col-2", Name: "shipping info", Description: "shipping rates and delivery times"},
			{ID: "col-3", Name: "technical docs", Description: "API documentation and integration guides"},
		},
	}
	fr := &FederatedRetriever{meta: meta}

	routed, err := fr.routeCollections(context.Background(), []string{"col-1", "col-2", "col-3"}, "refund policy", FederatedSearchOptions{
		Strategy:      FederationRoute,
		RouteTopN:     2,
		RouteMinScore: 0.1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(routed) > 2 {
		t.Errorf("expected at most 2 routed collections, got %d", len(routed))
	}
	if len(routed) == 0 {
		t.Error("expected at least 1 routed collection")
	}
	found := false
	for _, id := range routed {
		if id == "col-1" {
			found = true
		}
	}
	if !found {
		t.Error("expected col-1 (refund policy) to be in routed results")
	}
}

func TestCollectionRelevanceScore(t *testing.T) {
	col := biz.KnowledgeCollection{
		ID:          "test",
		Name:        "Refund Policy",
		Description: "Contains refund and return policies",
	}
	score := collectionRelevanceScore(col, "refund", []string{"refund"})
	if score <= 0 {
		t.Errorf("expected positive score for matching query, got %f", score)
	}

	noMatch := biz.KnowledgeCollection{
		ID:          "other",
		Name:        "Shipping",
		Description: "Shipping information",
	}
	score2 := collectionRelevanceScore(noMatch, "refund", []string{"refund"})
	if score2 > 0 {
		t.Errorf("expected zero score for non-matching query, got %f", score2)
	}
}

func TestSplitTerms(t *testing.T) {
	terms := splitTerms("hello world test")
	if len(terms) != 3 {
		t.Errorf("expected 3 terms, got %d", len(terms))
	}

	empty := splitTerms("")
	if len(empty) != 0 {
		t.Errorf("expected 0 terms for empty string, got %d", len(empty))
	}
}
