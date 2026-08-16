package service

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── SP1-E：块级反链 / dangling API ──────────────────────────────────────────

// stubBacklinkBlockReader service 测试用 BlockLinkReader 桩（仅 GetBlockOwnerDoc
// 有意义；反链三查询走内存图，不命中桩）。
type stubBacklinkBlockReader struct {
	owner map[string]string
	err   error
}

func (s *stubBacklinkBlockReader) ListBacklinksByBlock(context.Context, string) ([]bizknowledge.KnowledgeBlockRefEdge, error) {
	return nil, s.err
}
func (s *stubBacklinkBlockReader) ListBacklinksByDoc(context.Context, string) ([]bizknowledge.KnowledgeBlockRefEdge, error) {
	return nil, s.err
}
func (s *stubBacklinkBlockReader) ListDanglingEdges(context.Context, string) ([]bizknowledge.KnowledgeBlockRefEdge, error) {
	return nil, s.err
}
func (s *stubBacklinkBlockReader) GetBlockOwnerDoc(_ context.Context, blockID string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	d, ok := s.owner[blockID]
	if !ok {
		return "", apierror.NotFound("knowledge", "block not found: %s", blockID)
	}
	return d, nil
}

type stubBacklinkDocNames struct{ names map[string]string }

func (s *stubBacklinkDocNames) ListDocumentNames(_ context.Context, _ []string) (map[string]string, error) {
	return s.names, nil
}

// newBacklinkService 构造接好内存图（含一条 tb 入边 + 一条 dangling）与端口的 svc。
func newBacklinkService(t *testing.T) (*KnowledgeService, *us14MemRepo) {
	t.Helper()
	repo := newUS14MemRepo()
	ctx := context.Background()
	if _, err := repo.CreateCollection(ctx, biz.KnowledgeCollection{ID: "c1", Name: "vault"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"d1", "d2"} {
		if _, err := repo.CreateDocument(ctx, biz.KnowledgeDocument{ID: id, CollectionID: "c1", Source: id + ".md", Status: "indexed"}); err != nil {
			t.Fatal(err)
		}
	}
	uc := biz.NewKnowledgeUsecaseFromRepo(repo)
	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())

	// 构造函数已接线空图；测试覆盖为预载图（构造函数外装载，模拟启动加载完成）。
	idx := bizknowledge.NewLinkIndex()
	idx.LoadAll([]bizknowledge.KnowledgeBlockRefEdge{{
		CollectionID: "c1", SrcBlockID: "b2", SrcDocID: "d2",
		DstCollectionID: "c1", DstDocID: "d1", DstBlockID: "tb",
		RawTarget: "D1#^a", EdgeType: "ref", Context: "见 D1#^a。",
	}, {
		CollectionID: "c1", SrcBlockID: "b9", SrcDocID: "d2",
		RawTarget: "Ghost", EdgeType: "ref", Context: "未创建笔记",
	}})
	uc.SetLinkIndex(idx, nil)
	uc.SetBacklinkRepos(&stubBacklinkBlockReader{owner: map[string]string{"tb": "d1"}}, &stubBacklinkDocNames{
		names: map[string]string{"d2": "notes/b.md"},
	})
	return svc, repo
}

// TestKnowledgeService_ListBlockBacklinks_BlockPath 块级路径：块归属解析 →
// 权限断言 → 内存图反链 → proto 映射（含 SrcDocName）。
func TestKnowledgeService_ListBlockBacklinks_BlockPath(t *testing.T) {
	svc, _ := newBacklinkService(t)
	resp, err := svc.ListBlockBacklinks(context.Background(), &v1.ListBlockBacklinksRequest{BlockId: "tb"})
	if err != nil {
		t.Fatalf("ListBlockBacklinks: %v", err)
	}
	if len(resp.GetItems()) != 1 {
		t.Fatalf("items = %d, want 1", len(resp.GetItems()))
	}
	got := resp.GetItems()[0]
	if got.GetSrcBlockId() != "b2" || got.GetSrcDocId() != "d2" || got.GetSrcDocName() != "notes/b.md" ||
		got.GetRawTarget() != "D1#^a" || got.GetEdgeType() != "ref" || got.GetContext() == "" {
		t.Errorf("proto 映射错误: %+v", got)
	}

	// 未知块 → NotFound。
	if _, err := svc.ListBlockBacklinks(context.Background(), &v1.ListBlockBacklinksRequest{BlockId: "ghost"}); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("未知块 err = %v, want NotFound", err)
	}
	// 双空 → BadRequest。
	if _, err := svc.ListBlockBacklinks(context.Background(), &v1.ListBlockBacklinksRequest{}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("双空 err = %v, want BadRequest", err)
	}
}

// TestKnowledgeService_ListBlockBacklinks_DocBinding doc_id 绑定聚合文档全部入边，
// 无需块归属解析（tb 不在 owner 桩中也能查）。
func TestKnowledgeService_ListBlockBacklinks_DocBinding(t *testing.T) {
	svc, _ := newBacklinkService(t)
	resp, err := svc.ListBlockBacklinks(context.Background(), &v1.ListBlockBacklinksRequest{DocId: "d1"})
	if err != nil {
		t.Fatalf("ListBlockBacklinks: %v", err)
	}
	if len(resp.GetItems()) != 1 || resp.GetItems()[0].GetSrcDocId() != "d2" {
		t.Fatalf("doc 聚合错误: %+v", resp.GetItems())
	}
}

// TestKnowledgeService_ListDanglingLinks dangling 聚合 + proto 映射 + 权限断言路径。
func TestKnowledgeService_ListDanglingLinks(t *testing.T) {
	svc, _ := newBacklinkService(t)
	resp, err := svc.ListDanglingLinks(context.Background(), &v1.ListDanglingLinksRequest{Id: "c1"})
	if err != nil {
		t.Fatalf("ListDanglingLinks: %v", err)
	}
	if len(resp.GetItems()) != 1 {
		t.Fatalf("items = %d, want 1", len(resp.GetItems()))
	}
	got := resp.GetItems()[0]
	if got.GetRawTarget() != "Ghost" || got.GetRefCount() != 1 || len(got.GetRefs()) != 1 {
		t.Fatalf("dangling 映射错误: %+v", got)
	}
	if got.GetRefs()[0].GetSrcDocName() != "notes/b.md" || got.GetRefs()[0].GetContext() == "" {
		t.Errorf("refs 映射错误: %+v", got.GetRefs()[0])
	}

	// 未知集合 → NotFound（GetCollection 透传）。
	if _, err := svc.ListDanglingLinks(context.Background(), &v1.ListDanglingLinksRequest{Id: "ghost"}); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("未知集合 err = %v, want NotFound", err)
	}
}
