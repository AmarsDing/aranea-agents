package biz

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"
)

type stubUsageRepo struct {
	quota     UsageQuota
	hasQuota  bool
	spent     int64
	topModels []UsageBreakdownRow
}

func (s *stubUsageRepo) GetModelUsageSummary(context.Context, UsageQuery) (UsageSummary, error) {
	panic("not implemented")
}
func (s *stubUsageRepo) ListModelUsageTrends(context.Context, UsageQuery) ([]UsageTrendPoint, error) {
	panic("not implemented")
}
func (s *stubUsageRepo) ListTopModelUsage(context.Context, UsageQuery) ([]UsageBreakdownRow, error) {
	return s.topModels, nil
}
func (s *stubUsageRepo) ListTopAgentUsage(context.Context, UsageQuery) ([]UsageBreakdownRow, error) {
	panic("not implemented")
}
func (s *stubUsageRepo) ListModelUsageEvents(context.Context, UsageQuery) ([]TokenUsageEvent, error) {
	panic("not implemented")
}
func (s *stubUsageRepo) RecordTokenUsageEvent(context.Context, TokenUsageEvent) (TokenUsageEvent, error) {
	panic("not implemented")
}
func (s *stubUsageRepo) GetActiveModelPricing(context.Context, string, string) (ModelPricingSnapshot, bool, error) {
	return ModelPricingSnapshot{}, false, nil
}
func (s *stubUsageRepo) GetQuota(_ context.Context, _, _ string) (UsageQuota, error) {
	if !s.hasQuota {
		return UsageQuota{}, ErrQuotaNotFound
	}
	return s.quota, nil
}
func (s *stubUsageRepo) SetQuota(context.Context, UsageQuota) (UsageQuota, error) {
	panic("not implemented")
}
func (s *stubUsageRepo) SumScopeCostInPeriod(_ context.Context, scopeType, _ string, _, _ string) (int64, error) {
	if scopeType == "agent" {
		return s.spent, nil
	}
	return 0, nil
}
func (s *stubUsageRepo) ListActiveQuotas(context.Context) ([]UsageQuota, error) {
	return nil, nil
}
func (s *stubUsageRepo) BatchSumScopeCost(_ context.Context, _ []UsageQuota) (map[string]int64, error) {
	return map[string]int64{}, nil
}
func (s *stubUsageRepo) ListBudgetAlerts(context.Context, string, string) ([]BudgetAlert, error) {
	return nil, nil
}
func (s *stubUsageRepo) SetBudgetAlert(context.Context, BudgetAlert) (BudgetAlert, error) {
	panic("not implemented")
}
func (s *stubUsageRepo) UpdateBudgetAlertLastFired(context.Context, string, string) error {
	return nil
}
func (s *stubUsageRepo) ListModelUsageHourlyTrends(context.Context, UsageQuery) ([]UsageTrendPoint, error) {
	return nil, nil
}
func (s *stubUsageRepo) GetModelUsageSummaryFromDaily(context.Context, UsageQuery) (UsageSummary, error) {
	panic("not implemented")
}
func (s *stubUsageRepo) ListModelUsageDailyTrends(context.Context, UsageQuery) ([]UsageTrendPoint, error) {
	return nil, nil
}
func (s *stubUsageRepo) ListTopModelUsageFromDaily(context.Context, UsageQuery) ([]UsageBreakdownRow, error) {
	return s.topModels, nil
}
func (s *stubUsageRepo) ListTopAgentUsageFromDaily(context.Context, UsageQuery) ([]UsageBreakdownRow, error) {
	panic("not implemented")
}
func (s *stubUsageRepo) PurgeUsageEventsOlderThan(context.Context, int) (int64, error) {
	return 0, nil
}

func TestCheckQuota_noConfigAllowed(t *testing.T) {
	uc := NewUsageUsecase(&stubUsageRepo{hasQuota: false}, loggateway.Global())
	check, err := uc.CheckQuota(context.Background(), "agent", "a1")
	if err != nil {
		t.Fatal(err)
	}
	if !check.Allowed {
		t.Fatalf("expected allowed without quota, got %q", check.Reason)
	}
}

func TestCheckQuota_userScope(t *testing.T) {
	uc := NewUsageUsecase(&stubUsageRepo{
		hasQuota: true,
		quota: UsageQuota{
			ScopeType:       "user",
			ScopeID:         "u1",
			MonthlyMicroUSD: 5_000_000,
			PeriodStart:     "2026-05-01",
			PeriodEnd:       "2026-05-31",
		},
		spent: 1_000_000,
	}, loggateway.Global())
	check, err := uc.CheckQuota(context.Background(), "user", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if !check.Allowed {
		t.Fatalf("expected allowed for user scope, got %q", check.Reason)
	}
}

func TestCheckQuota_exceededBlocked(t *testing.T) {
	uc := NewUsageUsecase(&stubUsageRepo{
		hasQuota: true,
		quota: UsageQuota{
			ScopeType:       "agent",
			ScopeID:         "a1",
			MonthlyMicroUSD: 1_000_000,
			PeriodStart:     "2026-05-01",
			PeriodEnd:       "2026-05-31",
		},
		spent: 2_000_000,
	}, loggateway.Global())
	check, err := uc.CheckQuota(context.Background(), "agent", "a1")
	if err != nil {
		t.Fatal(err)
	}
	if check.Allowed {
		t.Fatal("expected blocked when spent >= cap")
	}
}
