package agent

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	knowledgeadapter "aranea-agents/internal/knowledge"
	knowledgetool "aranea-agents/internal/tools/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
)

// ── P2-a：框架原生 knowledge_search 收编 workspace 路由 ─────────────────────
// 契约：未指定 collection_id → 联邦 SearchAll + ReadableFilterID 租户过滤
// （fr 缺席降级空结果不阻塞）；指定 collection_id → GetCollection 租户可见性
// 校验先行（跨租户错误上抛，不进入检索）。

type stubCollectionMeta struct {
	gotWorkspace string
	cols         []biz.KnowledgeCollection
}

func (s *stubCollectionMeta) ListCollections(_ context.Context, ws string, _, _ int) ([]biz.KnowledgeCollection, int, error) {
	s.gotWorkspace = ws
	return s.cols, len(s.cols), nil
}

func TestWorkspaceRoutedSearch_NoCollectionFederated(t *testing.T) {
	meta := &stubCollectionMeta{}
	fr := knowledgeadapter.NewFederatedRetrieverWithMeta(nil, nil, meta, loggateway.NewNoop())
	fn := workspaceRoutedKnowledgeSearch(TRPCBuilderDeps{})

	// 租户 ctx：SearchAll 收到 ReadableFilterID（"ws-x"）。
	ctx := knowledgetool.WithFederatedRetriever(workspace.WithContext(context.Background(), "ws-x"), fr)
	if _, err := fn(ctx, biz.KnowledgeSearchQuery{Query: "q", TopK: 3}); err != nil {
		t.Fatal(err)
	}
	if meta.gotWorkspace != "ws-x" {
		t.Fatalf("workspace = %q, want ws-x", meta.gotWorkspace)
	}

	// system ctx：见全部（""）。
	meta.gotWorkspace = "<unset>"
	ctx = knowledgetool.WithFederatedRetriever(workspace.WithSystemWorkspace(context.Background()), fr)
	if _, err := fn(ctx, biz.KnowledgeSearchQuery{Query: "q", TopK: 3}); err != nil {
		t.Fatal(err)
	}
	if meta.gotWorkspace != "" {
		t.Fatalf("system workspace = %q, want empty", meta.gotWorkspace)
	}
}

func TestWorkspaceRoutedSearch_NoCollectionNoFederatedDegrades(t *testing.T) {
	fn := workspaceRoutedKnowledgeSearch(TRPCBuilderDeps{})
	got, err := fn(context.Background(), biz.KnowledgeSearchQuery{Query: "q", TopK: 3})
	if err != nil || got != nil {
		t.Fatalf("fr absent must degrade to empty: got %v err %v", got, err)
	}
}

func TestWorkspaceRoutedSearch_CrossTenantCollectionDenied(t *testing.T) {
	// GetCollection 跨租户拒绝（C-25 由 data 层 SQL 保证；此处模拟 biz 上抛）。
	uc := biz.NewKnowledgeUsecaseFromRepo(&stubCollRepo{err: errors.New("not found")})
	fn := workspaceRoutedKnowledgeSearch(TRPCBuilderDeps{
		TRPCMemoryKnowledgeDeps: TRPCMemoryKnowledgeDeps{KnowledgeUsecase: uc},
	})
	ctx := knowledgetool.WithFederatedRetriever(context.Background(),
		knowledgeadapter.NewFederatedRetrieverWithMeta(nil, nil, &stubCollectionMeta{}, loggateway.NewNoop()))
	_, err := fn(ctx, biz.KnowledgeSearchQuery{Query: "q", CollectionID: "c-other", TopK: 3})
	if err == nil {
		t.Fatal("cross-tenant collection must error before any search")
	}
}

// stubCollRepo 仅实现 GetCollection（其余方法 nil 嵌入不应被调到）。
type stubCollRepo struct {
	biz.KnowledgeRepo
	err error
}

func (s *stubCollRepo) GetCollection(context.Context, string) (biz.KnowledgeCollection, error) {
	return biz.KnowledgeCollection{}, s.err
}
