package service

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/loggateway"
)

// ── G-19：DebugKnowledgeSearch ─────────────────────────────────────────────

func TestDebugKnowledgeSearch_RequiresCollection(t *testing.T) {
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)
	retriever := knowledge.NewRetriever(us14QueryEmbedder{}, repo, nil, loggateway.NewNoop())
	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{
		Retriever: retriever,
	}, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())

	if _, err := svc.DebugKnowledgeSearch(context.Background(), &v1.DebugKnowledgeSearchRequest{
		Query: "q",
	}); err == nil {
		t.Fatal("collection_id 为空必须报错（hit-test 按库排查，不走联邦路由）")
	}
}

func TestDebugKnowledgeSearch_SetsSkipAccessLog(t *testing.T) {
	repo := newUS14MemRepo()
	uc := biz.NewKnowledgeUsecase(repo, repo, repo)
	if _, err := uc.CreateCollection(context.Background(), biz.KnowledgeCollection{
		ID: "col-x", Name: "x", EmbeddingModel: "m", Dim: 3,
	}); err != nil {
		t.Fatal(err)
	}
	retriever := knowledge.NewRetriever(us14QueryEmbedder{}, repo, nil, loggateway.NewNoop())
	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{
		Retriever: retriever,
	}, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())

	resp, err := svc.DebugKnowledgeSearch(context.Background(), &v1.DebugKnowledgeSearchRequest{
		CollectionId: "col-x",
		Query:        "refund policy",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	// 关键断言：调试调用必须打跳过标记——热度日志/个性化信号不受试探污染。
	repo.mu.Lock()
	skip := repo.lastSearchQ.SkipAccessLog
	repo.mu.Unlock()
	if !skip {
		t.Error("SkipAccessLog = false，调试流量将污染 access_log")
	}
}

func TestInferDebugModeUsed(t *testing.T) {
	cases := []struct {
		name   string
		chunks []biz.KnowledgeChunk
		want   string
	}{
		{"空结果", nil, "none"},
		{"rrf 融合", []biz.KnowledgeChunk{{ID: "a", RRFScore: 0.03}}, "rrf"},
		{"纯 sparse", []biz.KnowledgeChunk{{ID: "a", SparseScore: 9}}, "sparse"},
		{"纯 dense", []biz.KnowledgeChunk{{ID: "a", DenseScore: 0.8}}, "dense"},
		{"降级 visible：请求 rrf 实走 sparse", []biz.KnowledgeChunk{{ID: "a", SparseScore: 9, RRFScore: 0}}, "sparse"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := inferDebugModeUsed(tt.chunks); got != tt.want {
				t.Errorf("inferDebugModeUsed = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDebugReranked(t *testing.T) {
	if debugReranked(nil) {
		t.Error("nil chunks 不应判为重排")
	}
	if debugReranked([]biz.KnowledgeChunk{{ID: "a", RRFScore: 0.03}}) {
		t.Error("无 RerankScore 不应判为重排")
	}
	if !debugReranked([]biz.KnowledgeChunk{{ID: "a", RerankScore: 0.7}}) {
		t.Error("有 RerankScore 应判为重排")
	}
}
