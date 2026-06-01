package biz

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"
)

func BenchmarkCheckQuota(b *testing.B) {
	repo := &stubUsageRepo{
		hasQuota: true,
		quota: UsageQuota{
			ScopeType:       "agent",
			ScopeID:         "a1",
			MonthlyMicroUSD: 10_000_000,
			PeriodStart:     "2026-05-01",
			PeriodEnd:       "2026-05-31",
		},
		spent: 1_000_000,
	}
	uc := NewUsageUsecase(repo, loggateway.Global())
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = uc.CheckQuota(ctx, "agent", "a1")
	}
}
