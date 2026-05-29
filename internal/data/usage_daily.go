package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
)

func (r *usageRepo) GetModelUsageSummaryFromDaily(ctx context.Context, query biz.UsageQuery) (biz.UsageSummary, error) {
	where, args := usageDailyWhere(query)
	q := `SELECT
		 COALESCE(SUM(call_count), 0), COALESCE(SUM(request_count), 0),
		 COALESCE(SUM(success_count), 0), COALESCE(SUM(failed_count), 0), COALESCE(SUM(cancelled_count), 0),
		 COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_tokens), 0),
		 COALESCE(SUM(total_cost_micro_usd), 0),
		 COALESCE(SUM(avg_latency_ms * request_count) / NULLIF(SUM(request_count), 0), 0),
		 COALESCE(SUM(avg_tokens_per_second * request_count) / NULLIF(SUM(request_count), 0), 0)
		 FROM model_token_usage_daily` + where
	var v biz.UsageSummary
	err := entQueryRowScan(r.ent(), ctx, q, args,
		&v.CallCount, &v.RequestCount, &v.SuccessCount, &v.FailedCount, &v.CancelledCount,
		&v.InputTokens, &v.OutputTokens, &v.TotalTokens, &v.TotalCostMicroUSD, &v.AvgLatencyMS, &v.AvgTokensPerSecond)
	if err != nil {
		return biz.UsageSummary{}, err
	}
	if v.RequestCount > 0 {
		v.SuccessRate = float64(v.SuccessCount) / float64(v.RequestCount)
	}
	return v, nil
}

func (r *usageRepo) ListModelUsageDailyTrends(ctx context.Context, query biz.UsageQuery) ([]biz.UsageTrendPoint, error) {
	where, args := usageDailyWhere(query)
	rows, err := r.ent().QueryContext(ctx,
		`SELECT date_key,
		 COALESCE(SUM(call_count), 0),
		 COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_tokens), 0),
		 COALESCE(SUM(total_cost_micro_usd), 0),
		 COALESCE(SUM(success_count), 0),
		 COALESCE(SUM(failed_count), 0),
		 COALESCE(SUM(cancelled_count), 0),
		 COALESCE(SUM(avg_latency_ms * request_count) / NULLIF(SUM(request_count), 0), 0),
		 COALESCE(SUM(avg_tokens_per_second * request_count) / NULLIF(SUM(request_count), 0), 0)
		 FROM model_token_usage_daily`+where+` GROUP BY date_key ORDER BY date_key ASC`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []biz.UsageTrendPoint
	for rows.Next() {
		var point biz.UsageTrendPoint
		if err = rows.Scan(&point.DateKey, &point.CallCount, &point.InputTokens, &point.OutputTokens, &point.TotalTokens, &point.TotalCostMicroUSD, &point.SuccessCount, &point.FailedCount, &point.CancelledCount, &point.AvgLatencyMS, &point.AvgTokensPerSecond); err != nil {
			return nil, err
		}
		result = append(result, point)
	}
	return result, rows.Err()
}

func (r *usageRepo) ListTopModelUsageFromDaily(ctx context.Context, query biz.UsageQuery) ([]biz.UsageBreakdownRow, error) {
	where, args := usageDailyWhere(query)
	args = append(args, usageLimit(query.Limit))
	rows, err := r.ent().QueryContext(ctx,
		`SELECT provider_code, model_api_id,
		 COALESCE(SUM(call_count), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_tokens), 0),
		 COALESCE(SUM(total_cost_micro_usd), 0),
		 COALESCE(SUM(avg_latency_ms * request_count) / NULLIF(SUM(request_count), 0), 0),
		 COALESCE(SUM(avg_tokens_per_second * request_count) / NULLIF(SUM(request_count), 0), 0),
		 COALESCE(1.0 * SUM(success_count) / NULLIF(SUM(request_count), 0), 0)
		 FROM model_token_usage_daily`+where+` GROUP BY provider_code, model_api_id ORDER BY total_cost_micro_usd DESC, call_count DESC LIMIT ?`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []biz.UsageBreakdownRow
	for rows.Next() {
		var item biz.UsageBreakdownRow
		if err = rows.Scan(&item.ProviderCode, &item.ModelAPIID, &item.CallCount, &item.InputTokens, &item.OutputTokens, &item.TotalTokens, &item.TotalCostMicroUSD, &item.AvgLatencyMS, &item.AvgTokensPerSecond, &item.SuccessRate); err != nil {
			return nil, err
		}
		item.ModelDisplayName = item.ModelAPIID
		result = append(result, item)
	}
	return mergeUsageBreakdownByAlias(result), rows.Err()
}

func (r *usageRepo) ListTopAgentUsageFromDaily(ctx context.Context, query biz.UsageQuery) ([]biz.UsageBreakdownRow, error) {
	where, args := usageDailyWhere(query)
	args = append(args, usageLimit(query.Limit))
	rows, err := r.ent().QueryContext(ctx,
		`SELECT agent_id, agent_key,
		 COALESCE(SUM(call_count), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_tokens), 0),
		 COALESCE(SUM(total_cost_micro_usd), 0),
		 COALESCE(SUM(avg_latency_ms * request_count) / NULLIF(SUM(request_count), 0), 0),
		 COALESCE(SUM(avg_tokens_per_second * request_count) / NULLIF(SUM(request_count), 0), 0),
		 COALESCE(1.0 * SUM(success_count) / NULLIF(SUM(request_count), 0), 0)
		 FROM model_token_usage_daily`+where+` GROUP BY agent_id, agent_key ORDER BY total_cost_micro_usd DESC, call_count DESC LIMIT ?`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []biz.UsageBreakdownRow
	for rows.Next() {
		var item biz.UsageBreakdownRow
		if err = rows.Scan(&item.AgentID, &item.AgentKey, &item.CallCount, &item.InputTokens, &item.OutputTokens, &item.TotalTokens, &item.TotalCostMicroUSD, &item.AvgLatencyMS, &item.AvgTokensPerSecond, &item.SuccessRate); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func usageDailyWhere(query biz.UsageQuery) (string, []any) {
	parts := []string{sqlUsageBillableKind}
	args := []any{}
	if query.StartDate != "" {
		parts = append(parts, "date_key >= ?")
		args = append(args, query.StartDate)
	}
	if query.EndDate != "" {
		parts = append(parts, "date_key <= ?")
		args = append(args, query.EndDate)
	}
	if query.ProviderCode != "" {
		clause, provArgs := usageProviderWhere(query.ProviderCode)
		parts = append(parts, clause)
		args = append(args, provArgs...)
	}
	if query.ModelAPIID != "" {
		parts = append(parts, "model_api_id = ?")
		args = append(args, query.ModelAPIID)
	}
	if query.AgentID != "" {
		parts = append(parts, "agent_id = ?")
		args = append(args, query.AgentID)
	}
	if query.TeamID != "" {
		parts = append(parts, "workspace_id = ?")
		args = append(args, query.TeamID)
	}
	if query.UsageKind != "" {
		parts = append(parts, "usage_kind = ?")
		args = append(args, query.UsageKind)
	}
	if len(parts) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(parts, " AND "), args
}
