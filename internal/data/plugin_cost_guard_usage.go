package data

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"aranea-agents/internal/biz"
)

type pluginCostGuardUsageRepo struct {
	data *Data
}

func NewPluginCostGuardUsageRepo(data *Data) biz.PluginCostGuardUsageRepo {
	return &pluginCostGuardUsageRepo{data: data}
}

func (r *pluginCostGuardUsageRepo) GetTokens(ctx context.Context, usageDay, scopeKey string) (int, error) {
	if r == nil || r.data == nil || r.data.RawDB() == nil {
		return 0, nil
	}
	usageDay = strings.TrimSpace(usageDay)
	scopeKey = normalizeCostGuardScope(scopeKey)
	var tokens int
	err := r.data.RawDB().QueryRowContext(ctx,
		`SELECT tokens FROM plugin_cost_guard_usage WHERE usage_day = ? AND scope_key = ?`,
		usageDay, scopeKey,
	).Scan(&tokens)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return tokens, nil
}

func (r *pluginCostGuardUsageRepo) AddTokens(ctx context.Context, usageDay, scopeKey string, delta int) error {
	if r == nil || r.data == nil || r.data.RawDB() == nil || delta <= 0 {
		return nil
	}
	usageDay = strings.TrimSpace(usageDay)
	scopeKey = normalizeCostGuardScope(scopeKey)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.data.RawDB().ExecContext(ctx, `
INSERT INTO plugin_cost_guard_usage (usage_day, scope_key, tokens, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(usage_day, scope_key) DO UPDATE SET
  tokens = plugin_cost_guard_usage.tokens + excluded.tokens,
  updated_at = excluded.updated_at`,
		usageDay, scopeKey, delta, now,
	)
	return err
}

func normalizeCostGuardScope(scopeKey string) string {
	scopeKey = strings.TrimSpace(scopeKey)
	if scopeKey == "" {
		return "global"
	}
	return scopeKey
}
