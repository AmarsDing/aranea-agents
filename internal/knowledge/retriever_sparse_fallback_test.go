package knowledge

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

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

// P0-A（2026-08-20）：jsonPOST 传输层失败（连接拒绝等）归类 Unavailable，
// 使 hybrid_retriever 的 BM25 降级（F5）在 embedding 服务宕机时真实生效。
func TestJsonPOST_TransportError_IsUnavailable(t *testing.T) {
	client := &http.Client{Timeout: 2 * time.Second}
	// 127.0.0.1:1 为保留未监听端口，连接必被拒。
	_, err := jsonPOST(context.Background(), client, "http://127.0.0.1:1/embed", "", map[string]string{"inputs": "x"})
	if err == nil {
		t.Fatal("expected transport error")
	}
	if !apierror.IsCode(err, apierror.CodeUnavailable) {
		t.Fatalf("transport error must be CodeUnavailable, got %v", err)
	}
}

// P0-A：父 ctx 取消不算服务不可达——原样上抛，避免触发无意义降级。
func TestJsonPOST_CanceledContext_NotUnavailable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &http.Client{Timeout: 2 * time.Second}
	_, err := jsonPOST(ctx, client, "http://127.0.0.1:1/embed", "", map[string]string{"inputs": "x"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if apierror.IsCode(err, apierror.CodeUnavailable) {
		t.Fatal("canceled context must NOT be classified as CodeUnavailable")
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

// §V5 降级矩阵 #3：无语义层词法库（collection.embedding_model 空）即便全局
// embedder 可用也必须直接降级 BM25——chunks 无向量，dense 检索恒空且用户无感知
// （2026-08-10 运行时事故：UX验证库未绑 embedding_model，搜索静默返回空）。

func TestRetriever_Search_SparseFallback_LexicalCollection(t *testing.T) {
	repo := &stubSparseRepo{bm25Chunks: []biz.KnowledgeChunk{{ID: "c1", Content: "lexical hit"}}}
	repo.collection = biz.KnowledgeCollection{ID: "col-lex"} // EmbeddingModel 空 = 无语义层
	ret := NewRetriever(stubEmbedder{}, repo, nil, loggateway.NewNoop())

	out, err := ret.Search(context.Background(), biz.KnowledgeSearchQuery{CollectionID: "col-lex", Query: "q", TopK: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !repo.bm25Called {
		t.Fatal("lexical collection must route to BM25 fallback")
	}
	if repo.lastLimit != 0 {
		t.Fatalf("dense SearchChunks must not run for lexical collection, lastLimit=%d", repo.lastLimit)
	}
	if len(out) != 1 || out[0].ID != "c1" {
		t.Fatalf("unexpected chunks: %+v", out)
	}
}

func TestRetriever_Search_SemanticCollection_DenseUnaffected(t *testing.T) {
	// 对照组：语义库（embedding_model 已绑定）保持 dense 路径，不受降级判定影响。
	repo := &stubSparseRepo{bm25Chunks: []biz.KnowledgeChunk{{ID: "c1"}}}
	repo.collection = biz.KnowledgeCollection{ID: "col-sem", EmbeddingModel: "m", Dim: 3}
	repo.chunks = []biz.KnowledgeChunk{{ID: "dense-1"}}
	ret := NewRetriever(stubEmbedder{}, repo, nil, loggateway.NewNoop())

	out, err := ret.Search(context.Background(), biz.KnowledgeSearchQuery{CollectionID: "col-sem", Query: "q", TopK: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.bm25Called {
		t.Fatal("semantic collection must not degrade to BM25")
	}
	if len(out) != 1 || out[0].ID != "dense-1" {
		t.Fatalf("unexpected chunks: %+v", out)
	}
}

func TestHybridRetriever_Search_LexicalCollection_SparseFallback(t *testing.T) {
	repo := &stubSparseRepo{bm25Chunks: []biz.KnowledgeChunk{{ID: "s1"}}}
	repo.collection = biz.KnowledgeCollection{ID: "col-lex"} // 无语义层
	ret := NewRetriever(stubEmbedder{}, repo, nil, loggateway.NewNoop())
	h := NewHybridRetriever(ret, repo, loggateway.NewNoop())

	for _, mode := range []HybridSearchMode{HybridDense, HybridAuto, HybridRRF} {
		repo.bm25Called = false
		repo.lastLimit = 0
		out, err := h.Search(context.Background(), biz.KnowledgeSearchQuery{CollectionID: "col-lex", Query: "q", TopK: 5}, mode)
		if err != nil {
			t.Fatalf("mode %s: unexpected error: %v", mode, err)
		}
		if !repo.bm25Called {
			t.Fatalf("mode %s: lexical collection must route to BM25", mode)
		}
		if repo.lastLimit != 0 {
			t.Fatalf("mode %s: dense SearchChunks must not run for lexical collection", mode)
		}
		if len(out) != 1 || out[0].ID != "s1" {
			t.Fatalf("mode %s: unexpected chunks: %+v", mode, out)
		}
	}
}
