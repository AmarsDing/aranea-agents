package data

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	bizplugin "aranea-agents/internal/biz/plugin"
	"aranea-agents/pkg/apierror"
)

type pluginCostGuardUsageRepo struct {
	data *Data
}

var _ bizplugin.CostGuardUsageRepo = (*pluginCostGuardUsageRepo)(nil)

func NewPluginCostGuardUsageRepo(data *Data) biz.PluginCostGuardUsageRepo {
	return &pluginCostGuardUsageRepo{data: data}
}

func (r *pluginCostGuardUsageRepo) GetTokens(ctx context.Context, usageDay, scopeKey string) (int, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil {
		return 0, nil
	}
	usageDay = strings.TrimSpace(usageDay)
	scopeKey = normalizeCostGuardScope(scopeKey)
	var tokens int
	err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx),
		r.data.Dialect().RenumberPlaceholders(`SELECT tokens FROM plugin_cost_guard_usage WHERE usage_day = ? AND scope_key = ?`),
		[]any{usageDay, scopeKey}, &tokens)
	if err != nil {
		if ae, ok := apierror.From(err); ok && ae.Code == apierror.CodeNotFound {
			return 0, nil
		}
		return 0, entErrToBizErr(err, "PLUGIN")
	}
	return tokens, nil
}

func (r *pluginCostGuardUsageRepo) AddTokens(ctx context.Context, usageDay, scopeKey string, delta int) error {
	if r == nil || r.data == nil || r.data.RWDB() == nil || delta <= 0 {
		return nil
	}
	usageDay = strings.TrimSpace(usageDay)
	scopeKey = normalizeCostGuardScope(scopeKey)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(`
INSERT INTO plugin_cost_guard_usage (usage_day, scope_key, tokens, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(usage_day, scope_key) DO UPDATE SET
  tokens = plugin_cost_guard_usage.tokens + excluded.tokens,
  updated_at = excluded.updated_at`),
		usageDay, scopeKey, delta, now,
	)
	return entErrToBizErr(err, "PLUGIN")
}

func normalizeCostGuardScope(scopeKey string) string {
	scopeKey = strings.TrimSpace(scopeKey)
	if scopeKey == "" {
		return "global"
	}
	return scopeKey
}
