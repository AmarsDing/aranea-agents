package knowledge

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// stubParentResolver 记录调用并返回预置父块（backfill 单测桩）。
type stubParentResolver struct {
	calls   [][]string
	parents []biz.KnowledgeChunk
	err     error
}

func (s *stubParentResolver) ListChunksByIDs(_ context.Context, _ string, ids []string) ([]biz.KnowledgeChunk, error) {
	s.calls = append(s.calls, ids)
	if s.err != nil {
		return nil, s.err
	}
	return s.parents, nil
}

// 命中子块带 parent_chunk_id：一次批量回填，仅设置 ParentContent，不动排序/其他字段。
func TestBackfillParentContext_FillsParentContent(t *testing.T) {
	resolver := &stubParentResolver{parents: []biz.KnowledgeChunk{
		{ID: "doc1-p-0", Content: "父块零语境"},
		{ID: "doc1-p-1", Content: "父块一语境"},
	}}
	router := &AdaptiveRouter{parents: resolver, lg: loggateway.NewNoop()}
	chunks := []biz.KnowledgeChunk{
		{ID: "doc1-ch-0", Content: "子块0", ParentChunkID: "doc1-p-0", Score: 0.9},
		{ID: "doc1-ch-1", Content: "子块1", ParentChunkID: "doc1-p-1", Score: 0.8},
		{ID: "doc1-ch-2", Content: "子块2", ParentChunkID: "doc1-p-0", Score: 0.7},
		{ID: "doc2-ch-0", Content: "独立块", Score: 0.6}, // 无父引用（flat/其他策略）
	}
	got := router.BackfillParentContext(t.Context(), biz.KnowledgeSearchQuery{CollectionID: "col1"}, chunks)

	if len(resolver.calls) != 1 {
		t.Fatalf("expected exactly one batched resolve call, got %d", len(resolver.calls))
	}
	if len(resolver.calls[0]) != 2 {
		t.Fatalf("parent IDs must be deduped to one batch, got %v", resolver.calls[0])
	}
	if got[0].ParentContent != "父块零语境" || got[1].ParentContent != "父块一语境" {
		t.Fatalf("ParentContent not backfilled: %+v", got[:2])
	}
	if got[2].ParentContent != "父块零语境" {
		t.Fatalf("shared parent must backfill both children: %+v", got[2])
	}
	if got[3].ParentContent != "" || got[3].Content != "独立块" {
		t.Fatalf("flat chunk must stay untouched: %+v", got[3])
	}
	if got[0].Score != 0.9 || got[0].Content != "子块0" {
		t.Fatalf("backfill must not reorder or rewrite child content: %+v", got[0])
	}
}

// 无父引用（非 parent_child 文档）：零查询直达。
func TestBackfillParentContext_SkipsWhenNoParentIDs(t *testing.T) {
	resolver := &stubParentResolver{}
	router := &AdaptiveRouter{parents: resolver, lg: loggateway.NewNoop()}
	chunks := []biz.KnowledgeChunk{{ID: "doc1-ch-0", Content: "子块0"}}
	got := router.BackfillParentContext(t.Context(), biz.KnowledgeSearchQuery{CollectionID: "col1"}, chunks)
	if len(resolver.calls) != 0 {
		t.Fatalf("resolver must not be called without parent refs, got %d calls", len(resolver.calls))
	}
	if got[0].ParentContent != "" {
		t.Fatalf("unexpected ParentContent: %+v", got[0])
	}
}

// 查询失败仅告警跳过（检索不被回填阻断）；未取到的父块不设置空语境。
func TestBackfillParentContext_ErrorAndMissingParentsAreNonFatal(t *testing.T) {
	resolver := &stubParentResolver{err: apierror.Internal(apierror.DomainKnowledge, "boom")}
	router := &AdaptiveRouter{parents: resolver, lg: loggateway.NewNoop()}
	chunks := []biz.KnowledgeChunk{{ID: "doc1-ch-0", Content: "子块0", ParentChunkID: "doc1-p-0"}}
	got := router.BackfillParentContext(t.Context(), biz.KnowledgeSearchQuery{CollectionID: "col1"}, chunks)
	if got[0].ParentContent != "" {
		t.Fatalf("failed resolve must leave chunks untouched: %+v", got[0])
	}

	// 父块存在但正文为空：不写空覆盖（DisplayContent 始终回落子块原文）。
	resolver2 := &stubParentResolver{parents: []biz.KnowledgeChunk{{ID: "doc1-p-0", Content: ""}}}
	router2 := &AdaptiveRouter{parents: resolver2, lg: loggateway.NewNoop()}
	got2 := router2.BackfillParentContext(t.Context(), biz.KnowledgeSearchQuery{CollectionID: "col1"},
		[]biz.KnowledgeChunk{{ID: "doc1-ch-0", Content: "子块0", ParentChunkID: "doc1-p-0"}})
	if got2[0].ParentContent != "" {
		t.Fatalf("empty parent content must not be backfilled: %+v", got2[0])
	}
}
