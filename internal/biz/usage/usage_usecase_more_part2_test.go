package usage_test

import (
	usage "aranea-agents/internal/biz/usage"
	"context"
	"testing"
)

func TestCheckQuota_CurrentPeriod_NoRollover(t *testing.T) {
	setCalls := 0
	var sumStart, sumEnd string
	repo := &mockRepo{}
	repo.getQuotaFn = func(_ context.Context, _, _ string) (usage.Quota, error) {
		return usage.Quota{
			ScopeType:       "agent",
			ScopeID:         "agent-1",
			MonthlyMicroUSD: 10000,
			PeriodStart:     "2025-03-01",
			PeriodEnd:       "2025-03-31",
		}, nil
	}
	repo.setQuotaFn = func(_ context.Context, q usage.Quota) (usage.Quota, error) {
		setCalls++
		return q, nil
	}
	repo.sumScopeCostInPeriodFn = func(_ context.Context, _, _, ps, pe string) (int64, error) {
		sumStart, sumEnd = ps, pe
		return 5000, nil
	}
	uc := newTestUsecase(repo)
	if _, err := uc.CheckQuota(context.Background(), "agent", "agent-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if setCalls != 0 {
		t.Errorf("SetQuota called %d times, want 0 for unexpired period", setCalls)
	}
	if sumStart != "2025-03-01" || sumEnd != "2025-03-31" {
		t.Errorf("sum period = %s~%s, want original period", sumStart, sumEnd)
	}
}
