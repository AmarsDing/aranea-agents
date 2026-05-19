package data

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"

	"github.com/google/uuid"
)

func EnsureUsageQuotaSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS usage_quotas (
		id TEXT PRIMARY KEY,
		scope_type TEXT NOT NULL,
		scope_id TEXT NOT NULL,
		monthly_micro_usd INTEGER NOT NULL DEFAULT 0,
		period_start TEXT NOT NULL,
		period_end TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		UNIQUE(scope_type, scope_id)
	)`)
	return err
}

func (r *usageRepo) GetQuota(ctx context.Context, scopeType, scopeID string) (biz.UsageQuota, error) {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" {
		return biz.UsageQuota{}, fmt.Errorf("scope_type and scope_id are required")
	}
	var q biz.UsageQuota
	err := entQueryRowScan(r.ent(), ctx,
		`SELECT id, scope_type, scope_id, monthly_micro_usd, period_start, period_end, created_at, updated_at
		 FROM usage_quotas WHERE scope_type = ? AND scope_id = ?`,
		[]any{scopeType, scopeID},
		&q.ID, &q.ScopeType, &q.ScopeID, &q.MonthlyMicroUSD, &q.PeriodStart, &q.PeriodEnd, &q.CreatedAt, &q.UpdatedAt)
	if err != nil {
		return biz.UsageQuota{}, err
	}
	return q, nil
}

func (r *usageRepo) SetQuota(ctx context.Context, quota biz.UsageQuota) (biz.UsageQuota, error) {
	scopeType := strings.TrimSpace(quota.ScopeType)
	scopeID := strings.TrimSpace(quota.ScopeID)
	if scopeType == "" || scopeID == "" {
		return biz.UsageQuota{}, fmt.Errorf("scope_type and scope_id are required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	id := strings.TrimSpace(quota.ID)
	if id == "" {
		id = uuid.NewString()
	}
	periodStart := strings.TrimSpace(quota.PeriodStart)
	periodEnd := strings.TrimSpace(quota.PeriodEnd)
	if periodStart == "" || periodEnd == "" {
		t := time.Now().UTC()
		periodStart = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		periodEnd = time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	}
	_, err := r.ent().ExecContext(ctx,
		`INSERT INTO usage_quotas (id, scope_type, scope_id, monthly_micro_usd, period_start, period_end, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(scope_type, scope_id) DO UPDATE SET
		   monthly_micro_usd = excluded.monthly_micro_usd,
		   period_start = excluded.period_start,
		   period_end = excluded.period_end,
		   updated_at = excluded.updated_at`,
		id, scopeType, scopeID, quota.MonthlyMicroUSD, periodStart, periodEnd, now, now,
	)
	if err != nil {
		return biz.UsageQuota{}, err
	}
	return r.GetQuota(ctx, scopeType, scopeID)
}

func (r *usageRepo) SumAgentCostInPeriod(ctx context.Context, agentID, periodStart, periodEnd string) (int64, error) {
	var spent int64
	err := entQueryRowScan(r.ent(), ctx,
		`SELECT COALESCE(SUM(total_cost_micro_usd), 0)
		 FROM model_token_usage_events
		 WHERE agent_id = ? AND date_key >= ? AND date_key <= ?`,
		[]any{strings.TrimSpace(agentID), strings.TrimSpace(periodStart), strings.TrimSpace(periodEnd)},
		&spent)
	if err != nil {
		return 0, err
	}
	return spent, nil
}
