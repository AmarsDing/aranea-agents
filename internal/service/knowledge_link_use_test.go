package service

import (
	"context"
	"testing"
	"time"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── B4 #8：wikilink 落链 recency API ─────────────────────────────────────────

type stubLinkUsageRepo struct {
	touchColl string
	touchDoc  string
	items     []bizknowledge.LinkUse
}

func (s *stubLinkUsageRepo) TouchLinkUse(_ context.Context, collectionID, docID string, _ time.Time) error {
	s.touchColl, s.touchDoc = collectionID, docID
	return nil
}

func (s *stubLinkUsageRepo) ListRecentLinkUses(_ context.Context, _ string, _ int) ([]bizknowledge.LinkUse, error) {
	return s.items, nil
}

func linkUseServiceFixture(t *testing.T) (*KnowledgeService, *stubLinkUsageRepo) {
	t.Helper()
	repo := newUS14MemRepo()
	ctx := context.Background()
	if _, err := repo.CreateCollection(ctx, biz.KnowledgeCollection{ID: "c1", Name: "vault"}); err != nil {
		t.Fatal(err)
	}
	usage := &stubLinkUsageRepo{}
	uc := biz.NewKnowledgeUsecaseFromRepo(repo)
	uc.SetLinkUsageRepo(usage)
	return NewKnowledgeService(uc, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, nil, loggateway.NewNoop()), usage
}

func TestKnowledgeService_RecordLinkUse(t *testing.T) {
	svc, usage := linkUseServiceFixture(t)
	_, err := svc.RecordLinkUse(context.Background(), &v1.RecordLinkUseRequest{CollectionId: "c1", DocId: "d1"})
	if err != nil {
		t.Fatalf("RecordLinkUse: %v", err)
	}
	if usage.touchColl != "c1" || usage.touchDoc != "d1" {
		t.Errorf("落链参数透传错误: (%q, %q)", usage.touchColl, usage.touchDoc)
	}

	// 未知 collection → NotFound。
	if _, err := svc.RecordLinkUse(context.Background(), &v1.RecordLinkUseRequest{CollectionId: "ghost", DocId: "d1"}); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("未知 collection err = %v, want NotFound", err)
	}
}

func TestKnowledgeService_ListRecentLinkUses(t *testing.T) {
	svc, usage := linkUseServiceFixture(t)
	at := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	usage.items = []bizknowledge.LinkUse{{DocID: "d2", LastUsedAt: at}, {DocID: "d1"}}

	resp, err := svc.ListRecentLinkUses(context.Background(), &v1.ListRecentLinkUsesRequest{CollectionId: "c1", Limit: 32})
	if err != nil {
		t.Fatalf("ListRecentLinkUses: %v", err)
	}
	items := resp.GetItems()
	if len(items) != 2 || items[0].GetDocId() != "d2" || items[1].GetDocId() != "d1" {
		t.Fatalf("items 顺序/映射错误: %+v", items)
	}
	if items[0].GetLastUsedAt() != "2026-08-11T12:00:00Z" {
		t.Errorf("last_used_at 必须 RFC3339, got %q", items[0].GetLastUsedAt())
	}
	if items[1].GetLastUsedAt() != "" {
		t.Errorf("零值时间必须映射为空串, got %q", items[1].GetLastUsedAt())
	}

	if _, err := svc.ListRecentLinkUses(context.Background(), &v1.ListRecentLinkUsesRequest{CollectionId: "ghost"}); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("未知 collection err = %v, want NotFound", err)
	}
}
