package knowledge

import (
	"context"
	"testing"
	"time"
)

// G-21 二轮复核 F5/F6：ClaimDocumentForRetry 的退化契约。
// 生产 repo（实现 DocumentRetryClaimer）走守卫式 UPDATE 原子认领；
// 单测 fakes 未实现 claimer 时退化为读取判定——仅 error 可认领，
// indexing/pending/indexed 均不得重跑（防止 double-spawn 踩踏胜者终态）。
func TestUsecase_ClaimDocumentForRetry_FallbackOnlyError(t *testing.T) {
	build := func(status string) *Usecase {
		documents := &mockRepo{docGetFn: func(context.Context, string) (Document, error) {
			return Document{ID: "d1", Status: status}, nil
		}}
		return NewUsecase(nil, documents, nil)
	}
	staleBefore := time.Now().Add(-15 * time.Minute)
	for _, tt := range []struct {
		status string
		want   bool
	}{
		{"error", true},
		{"indexing", false},
		{"pending", false},
		{"indexed", false},
	} {
		u := build(tt.status)
		got, err := u.ClaimDocumentForRetry(context.Background(), "d1", staleBefore)
		if err != nil {
			t.Fatalf("%s: %v", tt.status, err)
		}
		if got != tt.want {
			t.Errorf("status=%s 认领 = %v, want %v", tt.status, got, tt.want)
		}
	}
}
