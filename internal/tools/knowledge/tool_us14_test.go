package knowledge

import (
	"context"
	"encoding/json"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/loggateway"
)

// mockCollectionMeta 实现 knowledge.CollectionMetaFetcher（SearchAll 全库枚举）。
type mockCollectionMeta struct {
	collections []biz.KnowledgeCollection
}

func (m *mockCollectionMeta) ListCollections(_ context.Context, _ string, _, _ int) ([]biz.KnowledgeCollection, int, error) {
	return m.collections, len(m.collections), nil
}

func newFederatedWithMeta(repo *mockKnowledgeRepo, cols ...biz.KnowledgeCollection) *knowledge.FederatedRetriever {
	ret := knowledge.NewRetriever(&mockQueryEmbedder{}, repo, nil, loggateway.NewNoop())
	return knowledge.NewFederatedRetrieverWithMeta(nil, ret, &mockCollectionMeta{collections: cols}, loggateway.NewNoop())
}

// US-14 规则 2：knowledge_search 留空 collection_id 且无 scoped → 全库智能路由。
func TestSearchTool_EmptyCollection_NoScope_RoutesAllCollections(t *testing.T) {
	var searched []string
	repo := &mockKnowledgeRepo{
		searchChunksFn: func(_ context.Context, q biz.KnowledgeSearchQuery, _ []float32) ([]biz.KnowledgeChunk, error) {
			searched = append(searched, q.CollectionID)
			return []biz.KnowledgeChunk{{ID: "ch-" + q.CollectionID, Content: "hit", Score: 0.9, DocID: "doc-1"}}, nil
		},
	}
	fr := newFederatedWithMeta(repo,
		biz.KnowledgeCollection{ID: "col-refund", Name: "refund policy"},
		biz.KnowledgeCollection{ID: "col-shipping", Name: "shipping info"},
	)

	tool := NewSearchTool()
	ctx := WithFederatedRetriever(context.Background(), fr)

	args, _ := json.Marshal(searchInput{CollectionID: "", Query: "refund policy"})
	result, err := tool.Call(ctx, args)
	if err != nil {
		t.Fatalf("collection-free search must not error, got %v", err)
	}
	out, ok := result.(searchOutput)
	if !ok {
		t.Fatalf("expected searchOutput, got %T", result)
	}
	if len(out.Chunks) == 0 {
		t.Fatal("expected chunks from routed collections")
	}
	// Route 策略：query 命中 col-refund 名称 → 只搜该库。
	if len(searched) != 1 || searched[0] != "col-refund" {
		t.Errorf("expected route to [col-refund], searched %v", searched)
	}
}

// US-14 规则 2：knowledge_search 留空 collection_id 且 scoped 多库 → scoped 内路由（不再报错）。
func TestSearchTool_EmptyCollection_MultiScope_RoutesWithinScoped(t *testing.T) {
	var searched []string
	repo := &mockKnowledgeRepo{
		searchChunksFn: func(_ context.Context, q biz.KnowledgeSearchQuery, _ []float32) ([]biz.KnowledgeChunk, error) {
			searched = append(searched, q.CollectionID)
			return []biz.KnowledgeChunk{{ID: "ch-" + q.CollectionID, Content: "hit", Score: 0.9, DocID: "doc-1"}}, nil
		},
	}
	fr := newFederatedWithMeta(repo,
		biz.KnowledgeCollection{ID: "col-a", Name: "alpha"},
		biz.KnowledgeCollection{ID: "col-b", Name: "beta"},
		biz.KnowledgeCollection{ID: "col-c", Name: "gamma"},
	)

	tool := NewSearchTool()
	ctx := WithFederatedRetriever(context.Background(), fr)
	ctx = WithKnowledgeCollections(ctx, []string{"col-a", "col-b"})

	args, _ := json.Marshal(searchInput{CollectionID: "", Query: "unrelated zzz"})
	result, err := tool.Call(ctx, args)
	if err != nil {
		t.Fatalf("multi-scoped collection-free search must not error, got %v", err)
	}
	if _, ok := result.(searchOutput); !ok {
		t.Fatalf("expected searchOutput, got %T", result)
	}
	// 只允许搜 scoped 内的 col-a/col-b，绝不能越权搜 col-c。
	if len(searched) != 2 {
		t.Fatalf("expected broadcast to 2 scoped collections, searched %v", searched)
	}
	for _, id := range searched {
		if id != "col-a" && id != "col-b" {
			t.Errorf("searched out-of-scope collection %q", id)
		}
	}
}

// US-14 规则 2：knowledge_reflect 留空 collection_ids 且无 scoped → 全库智能路由。
func TestReflectTool_EmptyCollections_NoScope_RoutesAllCollections(t *testing.T) {
	repo := &mockKnowledgeRepo{
		searchChunksFn: func(_ context.Context, q biz.KnowledgeSearchQuery, _ []float32) ([]biz.KnowledgeChunk, error) {
			return []biz.KnowledgeChunk{{ID: "ch-" + q.CollectionID, Content: "hit", Score: 0.9, DocID: "doc-1"}}, nil
		},
	}
	fr := newFederatedWithMeta(repo,
		biz.KnowledgeCollection{ID: "col-a", Name: "alpha"},
	)

	tool := NewReflectTool(nil)
	ctx := WithFederatedRetriever(context.Background(), fr)

	args, _ := json.Marshal(reflectInput{CollectionIDs: nil, Query: "anything"})
	result, err := tool.Call(ctx, args)
	if err != nil {
		t.Fatalf("collection-free reflect must not error, got %v", err)
	}
	out, ok := result.(reflectOutput)
	if !ok {
		t.Fatalf("expected reflectOutput, got %T", result)
	}
	if len(out.Chunks) == 0 {
		t.Fatal("expected chunks from all-collections routing")
	}
}

// US-14 零库行为：系统无任何 Collection → 工具返回空结果而非错误（LLM 可无知识回答）。
func TestSearchTool_EmptyCollection_ZeroCollections_ReturnsEmpty(t *testing.T) {
	repo := &mockKnowledgeRepo{}
	fr := newFederatedWithMeta(repo)

	tool := NewSearchTool()
	ctx := WithFederatedRetriever(context.Background(), fr)

	args, _ := json.Marshal(searchInput{CollectionID: "", Query: "anything"})
	result, err := tool.Call(ctx, args)
	if err != nil {
		t.Fatalf("zero collections must not error, got %v", err)
	}
	out, ok := result.(searchOutput)
	if !ok {
		t.Fatalf("expected searchOutput, got %T", result)
	}
	if len(out.Chunks) != 0 {
		t.Errorf("expected 0 chunks, got %d", len(out.Chunks))
	}
}
