package data

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	entpkg "aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/usagequota"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
)

func entUsageQuotaToBiz(row *entpkg.UsageQuota) biz.UsageQuota {
	if row == nil {
		return biz.UsageQuota{}
	}
	return biz.UsageQuota{
		ID:              row.ID,
		ScopeType:       row.ScopeType,
		ScopeID:         row.ScopeID,
		MonthlyMicroUSD: row.MonthlyMicroUsd,
		PeriodStart:     row.PeriodStart,
		PeriodEnd:       row.PeriodEnd,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

func (r *usageRepo) GetQuota(ctx context.Context, scopeType, scopeID string) (biz.UsageQuota, error) {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" {
		return biz.UsageQuota{}, biz.ErrUsageScopeRequired
	}
	row, err := r.ent().UsageQuota.Query().
		Where(usagequota.ScopeTypeEQ(scopeType), usagequota.ScopeIDEQ(scopeID)).
		Only(ctx)
	if err != nil {
		if entpkg.IsNotFound(err) {
			return biz.UsageQuota{}, biz.ErrQuotaNotFound
		}
		return biz.UsageQuota{}, err
	}
	return entUsageQuotaToBiz(row), nil
}

func (r *usageRepo) SetQuota(ctx context.Context, quota biz.UsageQuota) (biz.UsageQuota, error) {
	scopeType := strings.TrimSpace(quota.ScopeType)
	scopeID := strings.TrimSpace(quota.ScopeID)
	if scopeType == "" || scopeID == "" {
		return biz.UsageQuota{}, biz.ErrUsageScopeRequired
	}
	now := nowRFC3339()
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
	err := r.ent().UsageQuota.Create().
		SetID(id).
		SetScopeType(scopeType).
		SetScopeID(scopeID).
		SetMonthlyMicroUsd(quota.MonthlyMicroUSD).
		SetPeriodStart(periodStart).
		SetPeriodEnd(periodEnd).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		OnConflict(
			entsql.ConflictColumns(usagequota.FieldScopeType, usagequota.FieldScopeID),
			entsql.ResolveWithNewValues(),
		).
		UpdateNewValues().
		Exec(ctx)
	if err != nil {
		return biz.UsageQuota{}, err
	}
	return r.GetQuota(ctx, scopeType, scopeID)
}

func (r *usageRepo) SumScopeCostInPeriod(ctx context.Context, scopeType, scopeID, periodStart, periodEnd string) (int64, error) {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	periodStart = strings.TrimSpace(periodStart)
	periodEnd = strings.TrimSpace(periodEnd)
	q := `SELECT COALESCE(SUM(total_cost_micro_usd), 0) FROM model_token_usage_events WHERE date_key >= ? AND date_key <= ? AND ` + sqlUsageBillableKind
	args := []any{periodStart, periodEnd}
	switch scopeType {
	case "agent":
		q += ` AND agent_id = ?`
		args = append(args, scopeID)
	case "user":
		q += ` AND user_id = ?`
		args = append(args, scopeID)
	case "global":
		// all events in period
	default:
		return 0, biz.ErrUsageScopeRequired
	}
	var spent int64
	err := entQueryRowScan(r.ent(), ctx, q, args, &spent)
	if err != nil {
		return 0, err
	}
	return spent, nil
}

func (r *usageRepo) ListActiveQuotas(ctx context.Context) ([]biz.UsageQuota, error) {
	rows, err := r.ent().UsageQuota.Query().
		Where(usagequota.MonthlyMicroUsdGT(0)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.UsageQuota, 0, len(rows))
	for _, row := range rows {
		out = append(out, entUsageQuotaToBiz(row))
	}
	return out, nil
}
