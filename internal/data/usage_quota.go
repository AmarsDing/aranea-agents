package data

import (
	"context"
	"fmt"
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
	row, err := r.data.RW().Read(ctx).UsageQuota.Query().
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
	err := r.data.RW().Write(ctx).UsageQuota.Create().
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
	err := entQueryRowScan(r.data.RW().Read(ctx), ctx, r.data.Dialect().RenumberPlaceholders(q), args, &spent)
	if err != nil {
		return 0, err
	}
	return spent, nil
}

func (r *usageRepo) ListActiveQuotas(ctx context.Context) ([]biz.UsageQuota, error) {
	rows, err := r.data.RW().Read(ctx).UsageQuota.Query().
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

type quotaGroupKey struct {
	scopeType   string
	periodStart string
	periodEnd   string
}

func (r *usageRepo) BatchSumScopeCost(ctx context.Context, quotas []biz.UsageQuota) (map[string]int64, error) {
	result := make(map[string]int64, len(quotas))
	if len(quotas) == 0 {
		return result, nil
	}
	groups := make(map[quotaGroupKey][]biz.UsageQuota)
	for _, q := range quotas {
		key := quotaGroupKey{scopeType: q.ScopeType, periodStart: q.PeriodStart, periodEnd: q.PeriodEnd}
		groups[key] = append(groups[key], q)
	}
	for gk, gq := range groups {
		switch gk.scopeType {
		case "agent":
			ids := make([]string, 0, len(gq))
			for _, q := range gq {
				ids = append(ids, q.ScopeID)
			}
			sql := fmt.Sprintf(
				`SELECT agent_id, COALESCE(SUM(total_cost_micro_usd), 0) FROM model_token_usage_events WHERE date_key >= ? AND date_key <= ? AND %s AND agent_id IN (%s) GROUP BY agent_id`,
				sqlUsageBillableKind, placeholders(len(ids)),
			)
			args := []any{gk.periodStart, gk.periodEnd}
			for _, id := range ids {
				args = append(args, id)
			}
			rows, err := r.data.RW().Read(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(sql), args...)
			if err != nil {
				return result, err
			}
			for rows.Next() {
				var id string
				var spent int64
				if err := rows.Scan(&id, &spent); err != nil {
					rows.Close()
					return result, err
				}
				result["agent:"+id] = spent
			}
			rows.Close()
		case "user":
			ids := make([]string, 0, len(gq))
			for _, q := range gq {
				ids = append(ids, q.ScopeID)
			}
			sql := fmt.Sprintf(
				`SELECT user_id, COALESCE(SUM(total_cost_micro_usd), 0) FROM model_token_usage_events WHERE date_key >= ? AND date_key <= ? AND %s AND user_id IN (%s) GROUP BY user_id`,
				sqlUsageBillableKind, placeholders(len(ids)),
			)
			args := []any{gk.periodStart, gk.periodEnd}
			for _, id := range ids {
				args = append(args, id)
			}
			rows, err := r.data.RW().Read(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(sql), args...)
			if err != nil {
				return result, err
			}
			for rows.Next() {
				var id string
				var spent int64
				if err := rows.Scan(&id, &spent); err != nil {
					rows.Close()
					return result, err
				}
				result["user:"+id] = spent
			}
			rows.Close()
		case "global":
			var spent int64
			sql := fmt.Sprintf(
				`SELECT COALESCE(SUM(total_cost_micro_usd), 0) FROM model_token_usage_events WHERE date_key >= ? AND date_key <= ? AND %s`,
				sqlUsageBillableKind,
			)
			if err := entQueryRowScan(r.data.RW().Read(ctx), ctx, r.data.Dialect().RenumberPlaceholders(sql), []any{gk.periodStart, gk.periodEnd}, &spent); err != nil {
				return result, err
			}
			result["global:global"] = spent
		}
	}
	return result, nil
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	s := "?"
	for i := 1; i < n; i++ {
		s += ",?"
	}
	return s
}
