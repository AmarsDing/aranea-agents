package knowledge

import (
	"context"
	"errors"
	"testing"
	"time"
)

// ── B4 #8：wikilink 落链 recency（最近引用排序） ─────────────────────────────

type stubLinkUsageRepo struct {
	touchCalls int
	gotColl    string
	gotDoc     string
	gotAt      time.Time
	listLimit  int
	items      []LinkUse
	err        error
}

func (s *stubLinkUsageRepo) TouchLinkUse(_ context.Context, collectionID, docID string, at time.Time) error {
	s.touchCalls++
	s.gotColl, s.gotDoc, s.gotAt = collectionID, docID, at
	return s.err
}

func (s *stubLinkUsageRepo) ListRecentLinkUses(_ context.Context, _ string, limit int) ([]LinkUse, error) {
	s.listLimit = limit
	return s.items, s.err
}

func TestUsecase_RecordLinkUse(t *testing.T) {
	repo := &stubLinkUsageRepo{}
	u := NewUsecaseFromRepo(noOpMockRepo())
	u.SetLinkUsageRepo(repo)

	before := time.Now()
	if err := u.RecordLinkUse(context.Background(), " c1 ", " d1 "); err != nil {
		t.Fatalf("RecordLinkUse: %v", err)
	}
	if repo.touchCalls != 1 {
		t.Fatalf("touchCalls = %d, want 1", repo.touchCalls)
	}
	if repo.gotColl != "c1" || repo.gotDoc != "d1" {
		t.Errorf("args 必须 trim, got (%q, %q)", repo.gotColl, repo.gotDoc)
	}
	if repo.gotAt.Before(before) || repo.gotAt.After(time.Now()) {
		t.Errorf("at 必须为服务端当前时间, got %v", repo.gotAt)
	}
}

func TestUsecase_RecordLinkUse_Validation(t *testing.T) {
	repo := &stubLinkUsageRepo{}
	u := NewUsecaseFromRepo(noOpMockRepo())
	u.SetLinkUsageRepo(repo)

	if err := u.RecordLinkUse(context.Background(), "", "d1"); !errors.Is(err, ErrIDRequired) {
		t.Errorf("空 collection = %v, want ErrIDRequired", err)
	}
	if err := u.RecordLinkUse(context.Background(), "c1", "  "); !errors.Is(err, ErrIDRequired) {
		t.Errorf("空 doc = %v, want ErrIDRequired", err)
	}
	if repo.touchCalls != 0 {
		t.Errorf("非法输入不得触达端口, touchCalls = %d", repo.touchCalls)
	}
}

func TestUsecase_RecordLinkUse_PortNotWired(t *testing.T) {
	u := NewUsecaseFromRepo(noOpMockRepo()) // 未接线降级 no-op
	if err := u.RecordLinkUse(context.Background(), "c1", "d1"); err != nil {
		t.Fatalf("未接线必须 no-op 成功, got %v", err)
	}
}

func TestUsecase_ListRecentLinkUses_LimitClamp(t *testing.T) {
	repo := &stubLinkUsageRepo{}
	u := NewUsecaseFromRepo(noOpMockRepo())
	u.SetLinkUsageRepo(repo)

	cases := []struct{ in, want int }{
		{0, 32},   // 缺省 32（SiYuan 截断 32 语义）
		{-5, 32},  // 负数回落缺省
		{10, 10},  // 合法值透传
		{500, 128}, // 上限 128
	}
	for _, c := range cases {
		if _, err := u.ListRecentLinkUses(context.Background(), "c1", c.in); err != nil {
			t.Fatalf("ListRecentLinkUses(%d): %v", c.in, err)
		}
		if repo.listLimit != c.want {
			t.Errorf("limit(%d) = %d, want %d", c.in, repo.listLimit, c.want)
		}
	}
}

func TestUsecase_ListRecentLinkUses_Passthrough(t *testing.T) {
	now := time.Now()
	repo := &stubLinkUsageRepo{items: []LinkUse{
		{DocID: "d2", LastUsedAt: now},
		{DocID: "d1", LastUsedAt: now.Add(-time.Hour)},
	}}
	u := NewUsecaseFromRepo(noOpMockRepo())
	u.SetLinkUsageRepo(repo)

	got, err := u.ListRecentLinkUses(context.Background(), "c1", 32)
	if err != nil {
		t.Fatalf("ListRecentLinkUses: %v", err)
	}
	if len(got) != 2 || got[0].DocID != "d2" || got[1].DocID != "d1" {
		t.Errorf("端口结果必须原样透传, got %+v", got)
	}
}

func TestUsecase_ListRecentLinkUses_PortNotWired(t *testing.T) {
	u := NewUsecaseFromRepo(noOpMockRepo())
	got, err := u.ListRecentLinkUses(context.Background(), "c1", 32)
	if err != nil || got != nil {
		t.Errorf("未接线必须降级为空, got (%v, %v)", got, err)
	}
}
