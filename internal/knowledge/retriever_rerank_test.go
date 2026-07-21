package knowledge

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	"trpc.group/trpc-go/trpc-agent-go/knowledge/reranker/topk"
)

type stubKnowledgeRepo struct {
	lastLimit int
	chunks    []biz.KnowledgeChunk
}

func (s *stubKnowledgeRepo) CreateCollection(context.Context, biz.KnowledgeCollection) (biz.KnowledgeCollection, error) {
	return biz.KnowledgeCollection{}, nil
}
func (s *stubKnowledgeRepo) GetCollection(context.Context, string) (biz.KnowledgeCollection, error) {
	return biz.KnowledgeCollection{}, nil
}
func (s *stubKnowledgeRepo) ListCollections(context.Context, string, int, int) ([]biz.KnowledgeCollection, int, error) {
	return nil, 0, nil
}
func (s *stubKnowledgeRepo) DeleteCollection(context.Context, string) error { return nil }
func (s *stubKnowledgeRepo) UpdateCollectionCounts(context.Context, string, int, int) error {
	return nil
}
func (s *stubKnowledgeRepo) CreateDocument(context.Context, biz.KnowledgeDocument) (biz.KnowledgeDocument, error) {
	return biz.KnowledgeDocument{}, nil
}
func (s *stubKnowledgeRepo) GetDocument(context.Context, string) (biz.KnowledgeDocument, error) {
	return biz.KnowledgeDocument{}, nil
}
func (s *stubKnowledgeRepo) UpdateDocumentStatus(context.Context, string, string, string, int) error {
	return nil
}
func (s *stubKnowledgeRepo) UpdateDocumentContent(context.Context, string, string, bool) error {
	return nil
}
func (s *stubKnowledgeRepo) ListDocuments(context.Context, string, int, int) ([]biz.KnowledgeDocument, int, error) {
	return nil, 0, nil
}
func (s *stubKnowledgeRepo) DeleteDocument(context.Context, string) error             { return nil }
func (s *stubKnowledgeRepo) InsertChunks(context.Context, []biz.KnowledgeChunk) error { return nil }
func (s *stubKnowledgeRepo) DeleteChunksByDocument(context.Context, string) error     { return nil }

func (s *stubKnowledgeRepo) SearchChunks(_ context.Context, q biz.KnowledgeSearchQuery, _ []float32) ([]biz.KnowledgeChunk, error) {
	s.lastLimit = q.TopK
	return s.chunks, nil
}

type stubEmbedder struct{}

func (stubEmbedder) EmbedSingle(context.Context, string) ([]float32, error) {
	return []float32{0.1, 0.2}, nil
}

func TestRetrieverSearchWithTopKRerank(t *testing.T) {
	t.Parallel()
	repo := &stubKnowledgeRepo{
		chunks: []biz.KnowledgeChunk{
			{ID: "a", Content: "alpha", Score: 0.9, DocID: "d1"},
			{ID: "b", Content: "beta", Score: 0.8, DocID: "d2"},
			{ID: "c", Content: "gamma", Score: 0.7, DocID: "d3"},
			{ID: "d", Content: "delta", Score: 0.6, DocID: "d4"},
			{ID: "e", Content: "epsilon", Score: 0.5, DocID: "d5"},
		},
	}
	ret := NewRetriever(stubEmbedder{}, repo, topk.New(topk.WithK(2)), loggateway.NewNoop())

	out, err := ret.Search(context.Background(), biz.KnowledgeSearchQuery{
		CollectionID: "col",
		Query:        "test",
		TopK:         2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if repo.lastLimit < 20 {
		t.Fatalf("expected oversample limit >= 20, got %d", repo.lastLimit)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(out))
	}
	if out[0].ID != "a" || out[1].ID != "b" {
		t.Fatalf("unexpected order: %v, %v", out[0].ID, out[1].ID)
	}
}

func TestRetrieverSearchDisableRerank(t *testing.T) {
	repo := &stubKnowledgeRepo{
		chunks: []biz.KnowledgeChunk{{ID: "only", Content: "x", Score: 1}},
	}
	ret := NewRetriever(stubEmbedder{}, repo, topk.New(topk.WithK(1)), loggateway.NewNoop())
	off := false
	_, err := ret.Search(context.Background(), biz.KnowledgeSearchQuery{
		CollectionID: "col",
		Query:        "q",
		TopK:         1,
		UseRerank:    &off,
	})
	if err != nil {
		t.Fatal(err)
	}
	if repo.lastLimit != 1 {
		t.Fatalf("expected limit 1 without rerank, got %d", repo.lastLimit)
	}
}
