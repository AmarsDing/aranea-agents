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

// ── P2-7：unlinked mentions API ─────────────────────────────────────────────

type stubMentionSearcher struct{ hits []bizknowledge.DocContentHit }

func (s *stubMentionSearcher) SearchDocContentMentions(_ context.Context, _, _, _ string, _ int) ([]bizknowledge.DocContentHit, error) {
	return s.hits, nil
}

// TestKnowledgeService_ListUnlinkedMentions 权限断言 + biz 语义 + proto 映射。
func TestKnowledgeService_ListUnlinkedMentions(t *testing.T) {
	repo := newUS14MemRepo()
	ctx := context.Background()
	if _, err := repo.CreateCollection(ctx, biz.KnowledgeCollection{ID: "c1", Name: "vault"}); err != nil {
		t.Fatal(err)
	}
	for _, d := range []biz.KnowledgeDocument{
		{ID: "d1", CollectionID: "c1", Source: "目标笔记.md", Status: "indexed"},
		{ID: "d2", CollectionID: "c1", Source: "a.md", Status: "indexed"},
	} {
		if _, err := repo.CreateDocument(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	uc := biz.NewKnowledgeUsecaseFromRepo(repo)
	uc.SetMentionSearcher(&stubMentionSearcher{hits: []bizknowledge.DocContentHit{
		{DocID: "d2", DocName: "a.md", Content: "正文提到目标笔记，又见 [[目标笔记]] 与 目标笔记。"},
	}})
	svc := NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())

	resp, err := svc.ListUnlinkedMentions(ctx, &v1.ListUnlinkedMentionsRequest{DocId: "d1"})
	if err != nil {
		t.Fatalf("ListUnlinkedMentions: %v", err)
	}
	if len(resp.GetItems()) != 1 {
		t.Fatalf("items = %d, want 1", len(resp.GetItems()))
	}
	got := resp.GetItems()[0]
	if got.GetSrcDocId() != "d2" || got.GetSrcDocName() != "a.md" || got.GetCount() != 2 || got.GetSnippet() == "" {
		t.Errorf("proto 映射错误: %+v", got)
	}

	// 未知文档 → NotFound（GetDocument 透传）。
	if _, err := svc.ListUnlinkedMentions(ctx, &v1.ListUnlinkedMentionsRequest{DocId: "ghost"}); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("未知文档 err = %v, want NotFound", err)
	}
}
