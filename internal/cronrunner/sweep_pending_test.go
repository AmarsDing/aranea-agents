package cronrunner

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// 启动时：超过 2 小时还 pending 的执行行收尾为 failure；刚开始的那条不动。
func TestSweepStalePending(t *testing.T) {
	repo := newMissedFakeRepo()
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	_ = repo.InsertCronTaskRun(context.Background(), biz.CronTaskRunInput{ID: "old", TaskID: "t", Status: "pending", StartedAt: now.Add(-5 * time.Hour).Format(time.RFC3339)})
	_ = repo.InsertCronTaskRun(context.Background(), biz.CronTaskRunInput{ID: "fresh", TaskID: "t", Status: "pending", StartedAt: now.Add(-10 * time.Minute).Format(time.RFC3339)})
	r := NewRunner(Deps{Cron: repo}, loggateway.NewNoop())
	r.sweepStalePending(context.Background(), now)

	old, _ := repo.GetCronTaskRun(context.Background(), "old")
	fresh, _ := repo.GetCronTaskRun(context.Background(), "fresh")
	if old.Status != "failure" || old.ErrorMessage != stalePendingMessage || old.FinishedAt == "" {
		t.Fatalf("old run not swept: %+v", old)
	}
	if fresh.Status != "pending" {
		t.Fatalf("fresh run touched: %+v", fresh)
	}
}
