package knowledge

import (
	"context"
	"fmt"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// F5：embedder 缺失/不可用时降级 BM25 词法检索（V2：embedding 为可选增强）。

type stubSparseRepo struct {
	stubKnowledgeRepo
	bm25Chunks []biz.KnowledgeChunk
	bm25Called bool
}

func (s *stubSparseRepo) SearchChunksBM25(_ context.Context, _ biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error) {
	s.bm25Called = true
	return s.bm25Chunks, nil
}

type unavailableEmbedder struct{}

func (unavailableEmbedder) EmbedSingle(context.Context, string) ([]float32, error) {
	return nil, apierror.Unavailable(apierror.DomainKnowledge, "embedder not configured")
}

type internalErrEmbedder struct{}

func (internalErrEmbedder) EmbedSingle(context.Context, string) ([]float32, error) {
	return nil, fmt.Errorf("provider http 500")
}

func TestRetriever_Search_SparseFallback_NilEmbedder(t *testing.T) {
	repo := &stubSparseRepo{bm25Chunks: []biz.KnowledgeChunk{{ID: "c1", Content: "lexical hit"}}}
	ret := NewRetriever(nil, repo, nil, loggateway.NewNoop())

	out, err := ret.Search(context.Background(), biz.KnowledgeSearchQuery{CollectionID: "col-1", Query: "q", TopK: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !repo.bm25Called {
		t.Fatal("expected BM25 fallback to be called")
	}
	if len(out) != 1 || out[0].ID != "c1" {
		t.Fatalf("unexpected chunks: %+v", out)
	}
}

func TestRetriever_Search_SparseFallback_EmbedUnavailable(t *testing.T) {
	repo := &stubSparseRepo{bm25Chunks: []biz.KnowledgeChunk{{ID: "c1"}}}
	ret := NewRetriever(unavailableEmbedder{}, repo, nil, loggateway.NewNoop())

	out, err := ret.Search(context.Background(), biz.KnowledgeSearchQuery{CollectionID: "col-1", Query: "q", TopK: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !repo.bm25Called || len(out) != 1 {
		t.Fatalf("expected BM25 fallback result, bm25Called=%v out=%+v", repo.bm25Called, out)
	}
}

func TestRetriever_Search_EmbedInternalError_NoFallback(t *testing.T) {
	repo := &stubSparseRepo{bm25Chunks: []biz.KnowledgeChunk{{ID: "c1"}}}
	ret := NewRetriever(internalErrEmbedder{}, repo, nil, loggateway.NewNoop())

	_, err := ret.Search(context.Background(), biz.KnowledgeSearchQuery{CollectionID: "col-1", Query: "q", TopK: 5})
	if err == nil {
		t.Fatal("expected error for non-Unavailable embed failure")
	}
	if repo.bm25Called {
		t.Fatal("BM25 fallback must NOT trigger for non-Unavailable embed errors")
	}
	if !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("expected CodeInternal, got %v", err)
	}
}

func TestRetriever_Search_NilEmbedder_RepoWithoutBM25(t *testing.T) {
	repo := &stubKnowledgeRepo{} // 未实现 SearchChunksBM25
	ret := NewRetriever(nil, repo, nil, loggateway.NewNoop())

	_, err := ret.Search(context.Background(), biz.KnowledgeSearchQuery{CollectionID: "col-1", Query: "q", TopK: 5})
	if err == nil {
		t.Fatal("expected error when repo has no BM25 fallback")
	}
	if !apierror.IsCode(err, apierror.CodeUnavailable) {
		t.Fatalf("expected CodeUnavailable, got %v", err)
	}
}

func TestHybridRetriever_Search_NilEmbedder_SparseFallback(t *testing.T) {
	repo := &stubSparseRepo{bm25Chunks: []biz.KnowledgeChunk{{ID: "s1", Content: "bm25"}}}
	ret := NewRetriever(nil, repo, nil, loggateway.NewNoop())
	h := NewHybridRetriever(ret, repo, loggateway.NewNoop())

	out, err := h.Search(context.Background(), biz.KnowledgeSearchQuery{CollectionID: "col-1", Query: "q", TopK: 5}, HybridDense)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !repo.bm25Called || len(out) != 1 || out[0].ID != "s1" {
		t.Fatalf("expected sparse fallback, bm25Called=%v out=%+v", repo.bm25Called, out)
	}
}

func TestHybridRetriever_Search_EmbedUnavailable_DenseFallsBack(t *testing.T) {
	repo := &stubSparseRepo{bm25Chunks: []biz.KnowledgeChunk{{ID: "s1"}}}
	ret := NewRetriever(unavailableEmbedder{}, repo, nil, loggateway.NewNoop())
	h := NewHybridRetriever(ret, repo, loggateway.NewNoop())

	out, err := h.Search(context.Background(), biz.KnowledgeSearchQuery{CollectionID: "col-1", Query: "q", TopK: 5}, HybridDense)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !repo.bm25Called || len(out) != 1 {
		t.Fatalf("expected dense→sparse fallback, bm25Called=%v out=%+v", repo.bm25Called, out)
	}
}

func TestHybridRetriever_Search_EmbedUnavailable_RRFFallsBack(t *testing.T) {
	repo := &stubSparseRepo{bm25Chunks: []biz.KnowledgeChunk{{ID: "s1"}}}
	ret := NewRetriever(unavailableEmbedder{}, repo, nil, loggateway.NewNoop())
	h := NewHybridRetriever(ret, repo, loggateway.NewNoop())

	out, err := h.Search(context.Background(), biz.KnowledgeSearchQuery{CollectionID: "col-1", Query: "q", TopK: 5}, HybridRRF)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !repo.bm25Called || len(out) != 1 {
		t.Fatalf("expected rrf→sparse fallback, bm25Called=%v out=%+v", repo.bm25Called, out)
	}
}
