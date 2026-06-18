package knowledge

import (
	"context"
	"encoding/json"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/loggateway"
)

type mockQueryEmbedder struct {
	embedFn func(ctx context.Context, text string) ([]float32, error)
}

func (m *mockQueryEmbedder) EmbedSingle(ctx context.Context, text string) ([]float32, error) {
	if m.embedFn != nil {
		return m.embedFn(ctx, text)
	}
	return []float32{0.1, 0.2, 0.3}, nil
}

type mockKnowledgeRepo struct {
	searchChunksFn func(ctx context.Context, q biz.KnowledgeSearchQuery, vec []float32) ([]biz.KnowledgeChunk, error)
}

func (m *mockKnowledgeRepo) CreateCollection(_ context.Context, c biz.KnowledgeCollection) (biz.KnowledgeCollection, error) {
	return c, nil
}
func (m *mockKnowledgeRepo) GetCollection(_ context.Context, _ string) (biz.KnowledgeCollection, error) {
	return biz.KnowledgeCollection{}, biz.ErrNotFound
}
func (m *mockKnowledgeRepo) ListCollections(_ context.Context, _ string, _, _ int) ([]biz.KnowledgeCollection, int, error) {
	return nil, 0, nil
}
func (m *mockKnowledgeRepo) DeleteCollection(_ context.Context, _ string) error               { return nil }
func (m *mockKnowledgeRepo) UpdateCollectionCounts(_ context.Context, _ string, _, _ int) error { return nil }
func (m *mockKnowledgeRepo) CreateDocument(_ context.Context, d biz.KnowledgeDocument) (biz.KnowledgeDocument, error) {
	return d, nil
}
func (m *mockKnowledgeRepo) GetDocument(_ context.Context, _ string) (biz.KnowledgeDocument, error) {
	return biz.KnowledgeDocument{}, biz.ErrNotFound
}
func (m *mockKnowledgeRepo) UpdateDocumentStatus(_ context.Context, _, _, _ string, _ int) error {
	return nil
}
func (m *mockKnowledgeRepo) ListDocuments(_ context.Context, _ string, _, _ int) ([]biz.KnowledgeDocument, int, error) {
	return nil, 0, nil
}
func (m *mockKnowledgeRepo) DeleteDocument(_ context.Context, _ string) error          { return nil }
func (m *mockKnowledgeRepo) InsertChunks(_ context.Context, _ []biz.KnowledgeChunk) error { return nil }
func (m *mockKnowledgeRepo) DeleteChunksByDocument(_ context.Context, _ string) error  { return nil }
func (m *mockKnowledgeRepo) SearchChunks(ctx context.Context, q biz.KnowledgeSearchQuery, vec []float32) ([]biz.KnowledgeChunk, error) {
	if m.searchChunksFn != nil {
		return m.searchChunksFn(ctx, q, vec)
	}
	return nil, nil
}

type mockSparseSearcher struct {
	searchBM25Fn func(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error)
}

func (m *mockSparseSearcher) SearchChunksBM25(ctx context.Context, q biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error) {
	if m.searchBM25Fn != nil {
		return m.searchBM25Fn(ctx, q)
	}
	return nil, nil
}

func TestSearchTool_SuccessWithRetriever(t *testing.T) {
	repo := &mockKnowledgeRepo{
		searchChunksFn: func(_ context.Context, _ biz.KnowledgeSearchQuery, _ []float32) ([]biz.KnowledgeChunk, error) {
			return []biz.KnowledgeChunk{
				{ID: "ch-1", Content: "hello world", Score: 0.95, DocID: "doc-1"},
				{ID: "ch-2", Content: "foo bar", Score: 0.80, DocID: "doc-1"},
			}, nil
		},
	}
	embedder := &mockQueryEmbedder{}
	ret := knowledge.NewRetriever(embedder, repo, nil, loggateway.NewNoop())

	tool := NewSearchTool()
	ctx := WithRetriever(context.Background(), ret)

	args, _ := json.Marshal(searchInput{
		CollectionID: "col-1",
		Query:        "test query",
		TopK:         3,
	})

	result, err := tool.Call(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out, ok := result.(searchOutput)
	if !ok {
		t.Fatalf("expected searchOutput, got %T", result)
	}
	if len(out.Chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(out.Chunks))
	}
	if out.Chunks[0].ID != "ch-1" {
		t.Fatalf("expected chunk ID ch-1, got %s", out.Chunks[0].ID)
	}
	if out.Chunks[0].Score != 0.95 {
		t.Fatalf("expected score 0.95, got %f", out.Chunks[0].Score)
	}
}

func TestSearchTool_TopKDefault(t *testing.T) {
	var capturedTopK int
	repo := &mockKnowledgeRepo{
		searchChunksFn: func(_ context.Context, q biz.KnowledgeSearchQuery, _ []float32) ([]biz.KnowledgeChunk, error) {
			capturedTopK = q.TopK
			return nil, nil
		},
	}
	embedder := &mockQueryEmbedder{}
	ret := knowledge.NewRetriever(embedder, repo, nil, loggateway.NewNoop())

	tool := NewSearchTool()
	ctx := WithRetriever(context.Background(), ret)

	args, _ := json.Marshal(searchInput{
		CollectionID: "col-1",
		Query:        "test",
		TopK:         0,
	})

	_, err := tool.Call(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedTopK != 5 {
		t.Fatalf("expected TopK default 5, got %d", capturedTopK)
	}
}

func TestSearchTool_CollectionIDFromScopedContext(t *testing.T) {
	repo := &mockKnowledgeRepo{
		searchChunksFn: func(_ context.Context, _ biz.KnowledgeSearchQuery, _ []float32) ([]biz.KnowledgeChunk, error) {
			return []biz.KnowledgeChunk{{ID: "ch-1", Content: "result", Score: 0.9, DocID: "doc-1"}}, nil
		},
	}
	embedder := &mockQueryEmbedder{}
	ret := knowledge.NewRetriever(embedder, repo, nil, loggateway.NewNoop())

	tool := NewSearchTool()
	ctx := WithRetriever(context.Background(), ret)
	ctx = WithKnowledgeCollections(ctx, []string{"scoped-col"})

	args, _ := json.Marshal(searchInput{
		CollectionID: "",
		Query:        "test",
	})

	result, err := tool.Call(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out, ok := result.(searchOutput)
	if !ok {
		t.Fatalf("expected searchOutput, got %T", result)
	}
	if len(out.Chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(out.Chunks))
	}
}

func TestSearchTool_MultipleScopedCollectionsRequireID(t *testing.T) {
	tool := NewSearchTool()
	ctx := WithKnowledgeCollections(context.Background(), []string{"col-a", "col-b"})

	args, _ := json.Marshal(searchInput{
		CollectionID: "",
		Query:        "test",
	})

	_, err := tool.Call(ctx, args)
	if err == nil {
		t.Fatal("expected error when multiple scoped collections and no collection_id")
	}
}

func TestSearchTool_SuccessWithAdaptiveRouter(t *testing.T) {
	repo := &mockKnowledgeRepo{
		searchChunksFn: func(_ context.Context, _ biz.KnowledgeSearchQuery, _ []float32) ([]biz.KnowledgeChunk, error) {
			return []biz.KnowledgeChunk{
				{ID: "ch-router", Content: "routed result", Score: 0.88, DocID: "doc-1"},
			}, nil
		},
	}
	embedder := &mockQueryEmbedder{}
	ret := knowledge.NewRetriever(embedder, repo, nil, loggateway.NewNoop())
	hybrid := knowledge.NewHybridRetriever(ret, nil, loggateway.NewNoop())
	router := knowledge.NewAdaptiveRouter(hybrid, nil, loggateway.NewNoop())

	tool := NewSearchTool()
	ctx := WithAdaptiveRouter(context.Background(), router)

	args, _ := json.Marshal(searchInput{
		CollectionID: "col-1",
		Query:        "adaptive query",
		TopK:         5,
	})

	result, err := tool.Call(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out, ok := result.(searchOutput)
	if !ok {
		t.Fatalf("expected searchOutput, got %T", result)
	}
	if len(out.Chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(out.Chunks))
	}
	if out.Chunks[0].ID != "ch-router" {
		t.Fatalf("expected chunk ID ch-router, got %s", out.Chunks[0].ID)
	}
}

func TestReflectTool_SuccessWithFederatedRetriever(t *testing.T) {
	repo := &mockKnowledgeRepo{
		searchChunksFn: func(_ context.Context, _ biz.KnowledgeSearchQuery, _ []float32) ([]biz.KnowledgeChunk, error) {
			return []biz.KnowledgeChunk{
				{ID: "ch-fed", Content: "federated result", Score: 0.92, DocID: "doc-1"},
			}, nil
		},
	}
	embedder := &mockQueryEmbedder{}
	ret := knowledge.NewRetriever(embedder, repo, nil, loggateway.NewNoop())
	fr := knowledge.NewFederatedRetriever(nil, ret, loggateway.NewNoop())

	tool := NewReflectTool(nil)
	ctx := WithFederatedRetriever(context.Background(), fr)

	args, _ := json.Marshal(reflectInput{
		CollectionIDs: []string{"col-1"},
		Query:         "reflect query",
		TopK:          3,
	})

	result, err := tool.Call(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out, ok := result.(reflectOutput)
	if !ok {
		t.Fatalf("expected reflectOutput, got %T", result)
	}
	if len(out.Chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(out.Chunks))
	}
	if out.Chunks[0].ID != "ch-fed" {
		t.Fatalf("expected chunk ID ch-fed, got %s", out.Chunks[0].ID)
	}
	if !out.Sufficient {
		t.Fatal("expected Sufficient=true by default")
	}
}

func TestReflectTool_TopKDefault(t *testing.T) {
	var capturedTopK int
	repo := &mockKnowledgeRepo{
		searchChunksFn: func(_ context.Context, q biz.KnowledgeSearchQuery, _ []float32) ([]biz.KnowledgeChunk, error) {
			capturedTopK = q.TopK
			return nil, nil
		},
	}
	embedder := &mockQueryEmbedder{}
	ret := knowledge.NewRetriever(embedder, repo, nil, loggateway.NewNoop())
	fr := knowledge.NewFederatedRetriever(nil, ret, loggateway.NewNoop())

	tool := NewReflectTool(nil)
	ctx := WithFederatedRetriever(context.Background(), fr)

	args, _ := json.Marshal(reflectInput{
		CollectionIDs: []string{"col-1"},
		Query:         "test",
		TopK:          0,
	})

	_, err := tool.Call(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedTopK != 5 {
		t.Fatalf("expected TopK default 5, got %d", capturedTopK)
	}
}

func TestReflectTool_WithEvaluator(t *testing.T) {
	repo := &mockKnowledgeRepo{
		searchChunksFn: func(_ context.Context, _ biz.KnowledgeSearchQuery, _ []float32) ([]biz.KnowledgeChunk, error) {
			return []biz.KnowledgeChunk{
				{ID: "ch-1", Content: "result", Score: 0.9, DocID: "doc-1"},
			}, nil
		},
	}
	embedder := &mockQueryEmbedder{}
	ret := knowledge.NewRetriever(embedder, repo, nil, loggateway.NewNoop())
	fr := knowledge.NewFederatedRetriever(nil, ret, loggateway.NewNoop())
	ev := knowledge.NewRetrievalEvaluator(nil, nil, nil, loggateway.NewNoop())

	tool := NewReflectTool(nil)
	ctx := WithFederatedRetriever(context.Background(), fr)
	ctx = WithRetrievalEvaluator(ctx, ev)

	args, _ := json.Marshal(reflectInput{
		CollectionIDs: []string{"col-1"},
		Query:         "test",
	})

	result, err := tool.Call(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out, ok := result.(reflectOutput)
	if !ok {
		t.Fatalf("expected reflectOutput, got %T", result)
	}
	if !out.Sufficient {
		t.Fatal("expected Sufficient=true from nil-LLM evaluator degradation")
	}
	if out.Confidence != 1.0 {
		t.Fatalf("expected Confidence=1.0 from nil-LLM evaluator, got %f", out.Confidence)
	}
}

func TestReflectTool_CollectionIDsFromScopedContext(t *testing.T) {
	repo := &mockKnowledgeRepo{
		searchChunksFn: func(_ context.Context, _ biz.KnowledgeSearchQuery, _ []float32) ([]biz.KnowledgeChunk, error) {
			return nil, nil
		},
	}
	embedder := &mockQueryEmbedder{}
	ret := knowledge.NewRetriever(embedder, repo, nil, loggateway.NewNoop())
	fr := knowledge.NewFederatedRetriever(nil, ret, loggateway.NewNoop())

	tool := NewReflectTool(nil)
	ctx := WithFederatedRetriever(context.Background(), fr)
	ctx = WithRetrievalEvaluator(ctx, nil)
	ctx = WithKnowledgeCollections(ctx, []string{"col-1", "col-2"})

	args, _ := json.Marshal(reflectInput{
		CollectionIDs: nil,
		Query:         "test",
	})

	result, err := tool.Call(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out, ok := result.(reflectOutput)
	if !ok {
		t.Fatalf("expected reflectOutput, got %T", result)
	}
	if !out.Sufficient {
		t.Fatal("expected Sufficient=true")
	}
}

func TestContextHelpers_NilValues(t *testing.T) {
	ctx := context.Background()

	if got := RetrieverFromContext(ctx); got != nil {
		t.Fatalf("expected nil retriever, got %v", got)
	}
	if got := AdaptiveRouterFromContext(ctx); got != nil {
		t.Fatalf("expected nil router, got %v", got)
	}
	if got := FederatedRetrieverFromContext(ctx); got != nil {
		t.Fatalf("expected nil federated retriever, got %v", got)
	}
	if got := RetrievalEvaluatorFromContext(ctx); got != nil {
		t.Fatalf("expected nil evaluator, got %v", got)
	}
	if got := knowledgeCollectionsFromContext(ctx); got != nil {
		t.Fatalf("expected nil collections, got %v", got)
	}
}

func TestContextHelpers_WrongTypeAssertion(t *testing.T) {
	ctx := context.WithValue(context.Background(), contextKey{}, "not-a-retriever")
	if got := RetrieverFromContext(ctx); got != nil {
		t.Fatalf("expected nil on wrong type, got %v", got)
	}

	ctx = context.WithValue(context.Background(), routerKey{}, 42)
	if got := AdaptiveRouterFromContext(ctx); got != nil {
		t.Fatalf("expected nil on wrong type, got %v", got)
	}

	ctx = context.WithValue(context.Background(), federatedKey{}, []string{"wrong"})
	if got := FederatedRetrieverFromContext(ctx); got != nil {
		t.Fatalf("expected nil on wrong type, got %v", got)
	}

	ctx = context.WithValue(context.Background(), evaluatorKey{}, map[string]any{})
	if got := RetrievalEvaluatorFromContext(ctx); got != nil {
		t.Fatalf("expected nil on wrong type, got %v", got)
	}

	ctx = context.WithValue(context.Background(), collectionsKey{}, "not-a-slice")
	if got := knowledgeCollectionsFromContext(ctx); got != nil {
		t.Fatalf("expected nil on wrong type, got %v", got)
	}
}

func TestSearchTool_EmptyQuery(t *testing.T) {
	tool := NewSearchTool()
	ctx := context.Background()

	args, _ := json.Marshal(searchInput{
		CollectionID: "col-1",
		Query:        "",
	})

	_, err := tool.Call(ctx, args)
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestSearchTool_ScopedCollectionNotAllowed(t *testing.T) {
	tool := NewSearchTool()
	ctx := WithKnowledgeCollections(context.Background(), []string{"col-1"})

	args, _ := json.Marshal(searchInput{
		CollectionID: "col-other",
		Query:        "test",
	})

	_, err := tool.Call(ctx, args)
	if err == nil {
		t.Fatal("expected error when collection_id not in scoped list")
	}
}

func TestReflectTool_WithRetriever_SingleCollection(t *testing.T) {
	repo := &mockKnowledgeRepo{
		searchChunksFn: func(_ context.Context, _ biz.KnowledgeSearchQuery, _ []float32) ([]biz.KnowledgeChunk, error) {
			return []biz.KnowledgeChunk{
				{ID: "ch-1", Content: "result", Score: 0.9, DocID: "doc-1"},
			}, nil
		},
	}
	embedder := &mockQueryEmbedder{}
	ret := knowledge.NewRetriever(embedder, repo, nil, loggateway.NewNoop())

	tool := NewReflectTool(nil)
	ctx := WithRetriever(context.Background(), ret)

	args, _ := json.Marshal(reflectInput{
		CollectionIDs: []string{"col-1"},
		Query:         "test",
	})

	result, err := tool.Call(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out, ok := result.(reflectOutput)
	if !ok {
		t.Fatalf("expected reflectOutput, got %T", result)
	}
	if len(out.Chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(out.Chunks))
	}
}

func TestReflectTool_WithRetriever_MultiCollection_Error(t *testing.T) {
	ret := knowledge.NewRetriever(&mockQueryEmbedder{}, &mockKnowledgeRepo{}, nil, loggateway.NewNoop())

	tool := NewReflectTool(nil)
	ctx := WithRetriever(context.Background(), ret)

	args, _ := json.Marshal(reflectInput{
		CollectionIDs: []string{"col-1", "col-2"},
		Query:         "test",
	})

	_, err := tool.Call(ctx, args)
	if err == nil {
		t.Fatal("expected error for multi-collection without federated retriever")
	}
}
