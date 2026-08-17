package knowledge

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"trpc.group/trpc-go/trpc-agent-go/knowledge/reranker"
)

func TestParseHybridSearchMode(t *testing.T) {
	tests := []struct {
		input string
		want  HybridSearchMode
	}{
		{"", HybridAuto},
		{"auto", HybridAuto},
		{"dense", HybridDense},
		{"sparse", HybridSparse},
		{"rrf", HybridRRF},
		{"unknown", HybridAuto},
	}
	for _, tt := range tests {
		got := ParseHybridSearchMode(tt.input)
		if got != tt.want {
			t.Errorf("ParseHybridSearchMode(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRRFMerge(t *testing.T) {
	dense := []biz.KnowledgeChunk{
		{ID: "a", Content: "alpha", Score: 0.9},
		{ID: "b", Content: "beta", Score: 0.7},
		{ID: "c", Content: "gamma", Score: 0.5},
	}
	sparse := []biz.KnowledgeChunk{
		{ID: "b", Content: "beta", Score: 0.01},
		{ID: "d", Content: "delta", Score: 0.008},
		{ID: "a", Content: "alpha", Score: 0.005},
	}

	merged := rrfMerge(dense, sparse, 60)
	if len(merged) != 4 {
		t.Fatalf("expected 4 unique chunks, got %d", len(merged))
	}

	ids := make(map[string]bool)
	for _, ch := range merged {
		ids[ch.ID] = true
	}
	for _, want := range []string{"a", "b", "c", "d"} {
		if !ids[want] {
			t.Errorf("missing chunk %q in merged results", want)
		}
	}

	if merged[0].ID != "a" && merged[0].ID != "b" {
		t.Errorf("expected 'a' or 'b' to rank first (both appear in both lists), got %q", merged[0].ID)
	}
}

func TestMergeSearchResults(t *testing.T) {
	primary := []biz.KnowledgeChunk{
		{ID: "1", Score: 0.9},
		{ID: "2", Score: 0.7},
	}
	supplement := []biz.KnowledgeChunk{
		{ID: "2", Score: 0.8},
		{ID: "3", Score: 0.6},
	}

	merged := MergeSearchResults(primary, supplement, 5)
	if len(merged) != 3 {
		t.Fatalf("expected 3 unique chunks, got %d", len(merged))
	}
	if merged[0].Score < merged[1].Score {
		t.Errorf("results not sorted by score descending")
	}
}

func TestMergeSearchResultsTopK(t *testing.T) {
	primary := []biz.KnowledgeChunk{
		{ID: "1", Score: 0.9},
		{ID: "2", Score: 0.7},
		{ID: "3", Score: 0.5},
	}
	merged := MergeSearchResults(primary, nil, 2)
	if len(merged) != 2 {
		t.Fatalf("expected 2 chunks with topK=2, got %d", len(merged))
	}
}

type reverseReranker struct {
	called    bool
	lastInput []string
}

func (r *reverseReranker) Rerank(_ context.Context, _ *reranker.Query, in []*reranker.Result) ([]*reranker.Result, error) {
	r.called = true
	for _, item := range in {
		r.lastInput = append(r.lastInput, item.Document.ID)
	}
	out := append([]*reranker.Result(nil), in...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func TestHybridRetriever_RRFAppliesConfiguredReranker(t *testing.T) {
	repo := &stubSparseRepo{
		stubKnowledgeRepo: stubKnowledgeRepo{chunks: []biz.KnowledgeChunk{
			{ID: "dense", DocID: "d1", Content: "dense"},
		}},
		bm25Chunks: []biz.KnowledgeChunk{{ID: "sparse", DocID: "d2", Content: "sparse"}},
	}
	rr := &reverseReranker{}
	ret := NewRetriever(stubEmbedder{}, repo, rr, loggateway.NewNoop())
	h := NewHybridRetriever(ret, repo, loggateway.NewNoop())

	got, err := h.Search(context.Background(), biz.KnowledgeSearchQuery{
		CollectionID: "col", Query: "query", TopK: 2,
	}, HybridRRF)
	if err != nil {
		t.Fatal(err)
	}
	if !rr.called {
		t.Fatal("configured reranker was not called after RRF fusion")
	}
	if len(got) != 2 || len(rr.lastInput) != 2 || got[0].ID != rr.lastInput[1] {
		t.Fatalf("RRF rerank result = %+v, want reversed order", got)
	}
}

type parallelSearchRepo struct {
	stubKnowledgeRepo
	denseStarted  chan struct{}
	sparseStarted chan struct{}
	release       chan struct{}
	onceDense     sync.Once
	onceSparse    sync.Once
}

func (r *parallelSearchRepo) SearchChunks(_ context.Context, _ biz.KnowledgeSearchQuery, _ []float32) ([]biz.KnowledgeChunk, error) {
	r.onceDense.Do(func() { close(r.denseStarted) })
	<-r.release
	return []biz.KnowledgeChunk{{ID: "dense", DocID: "d1"}}, nil
}

func (r *parallelSearchRepo) SearchChunksBM25(_ context.Context, _ biz.KnowledgeSearchQuery) ([]biz.KnowledgeChunk, error) {
	r.onceSparse.Do(func() { close(r.sparseStarted) })
	<-r.release
	return []biz.KnowledgeChunk{{ID: "sparse", DocID: "d2"}}, nil
}

func TestHybridRetriever_RRFStartsDenseAndSparseConcurrently(t *testing.T) {
	repo := &parallelSearchRepo{
		denseStarted:  make(chan struct{}),
		sparseStarted: make(chan struct{}),
		release:       make(chan struct{}),
	}
	ret := NewRetriever(stubEmbedder{}, repo, nil, loggateway.NewNoop())
	h := NewHybridRetriever(ret, repo, loggateway.NewNoop())
	done := make(chan error, 1)
	safego.Go(context.Background(), "test.knowledge.rrf_parallel", func() {
		_, err := h.Search(context.Background(), biz.KnowledgeSearchQuery{
			CollectionID: "col", Query: "query", TopK: 2,
		}, HybridRRF)
		done <- err
	})

	for name, started := range map[string]<-chan struct{}{
		"dense": repo.denseStarted, "sparse": repo.sparseStarted,
	} {
		select {
		case <-started:
		case <-time.After(time.Second):
			close(repo.release)
			t.Fatalf("%s search did not start before the peer was released", name)
		}
	}
	close(repo.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
