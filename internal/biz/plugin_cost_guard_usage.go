package biz

import "context"

// PluginCostGuardUsageRepo persists daily token totals for cost_guard (cross-process).
type PluginCostGuardUsageRepo interface {
	GetTokens(ctx context.Context, usageDay, scopeKey string) (int, error)
	AddTokens(ctx context.Context, usageDay, scopeKey string, delta int) error
}
