package biz

import "context"

// UsageAnalyticsRepo reads aggregates and event lists (no writes).
type UsageAnalyticsRepo interface {
	GetModelUsageSummary(ctx context.Context, query UsageQuery) (UsageSummary, error)
	ListModelUsageTrends(ctx context.Context, query UsageQuery) ([]UsageTrendPoint, error)
	ListTopModelUsage(ctx context.Context, query UsageQuery) ([]UsageBreakdownRow, error)
	ListTopAgentUsage(ctx context.Context, query UsageQuery) ([]UsageBreakdownRow, error)
	ListModelUsageEvents(ctx context.Context, query UsageQuery) ([]TokenUsageEvent, error)
	ListModelUsageHourlyTrends(ctx context.Context, query UsageQuery) ([]UsageTrendPoint, error)
}

// UsageWriteRepo persists usage events and resolves pricing.
type UsageWriteRepo interface {
	RecordTokenUsageEvent(ctx context.Context, event TokenUsageEvent) (TokenUsageEvent, error)
	GetActiveModelPricing(ctx context.Context, providerCode, modelAPIID string) (ModelPricingSnapshot, bool, error)
}

// UsageQuotaRepo manages caps, spend sums, and budget alerts.
type UsageQuotaRepo interface {
	GetQuota(ctx context.Context, scopeType, scopeID string) (UsageQuota, error)
	SetQuota(ctx context.Context, quota UsageQuota) (UsageQuota, error)
	SumScopeCostInPeriod(ctx context.Context, scopeType, scopeID, periodStart, periodEnd string) (int64, error)
	ListActiveQuotas(ctx context.Context) ([]UsageQuota, error)
	ListBudgetAlerts(ctx context.Context, scopeType, scopeID string) ([]BudgetAlert, error)
	SetBudgetAlert(ctx context.Context, alert BudgetAlert) (BudgetAlert, error)
	UpdateBudgetAlertLastFired(ctx context.Context, id, firedAt string) error
}

// UsageRepo is the composed persistence contract for UsageUsecase.
type UsageRepo interface {
	UsageAnalyticsRepo
	UsageWriteRepo
	UsageQuotaRepo
}
