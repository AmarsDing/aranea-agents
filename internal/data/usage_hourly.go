package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
)

func (r *usageRepo) ListModelUsageHourlyTrends(ctx context.Context, query biz.UsageQuery) ([]biz.UsageTrendPoint, error) {
	where, args := usageHourlyWhere(query)
	rows, err := r.ent().QueryContext(ctx,
		`SELECT hour_key,
		 COALESCE(SUM(call_count), 0),
		 COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_tokens), 0),
		 COALESCE(SUM(total_cost_micro_usd), 0),
		 COALESCE(SUM(success_count), 0),
		 COALESCE(SUM(failed_count), 0),
		 COALESCE(SUM(cancelled_count), 0),
		 COALESCE(AVG(avg_latency_ms), 0), COALESCE(AVG(avg_tokens_per_second), 0)
		 FROM model_token_usage_hourly`+where+` GROUP BY hour_key ORDER BY hour_key ASC`,
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

func usageHourlyWhere(query biz.UsageQuery) (string, []any) {
	parts := []string{}
	args := []any{}
	if query.StartDate != "" {
		parts = append(parts, "hour_key >= ?")
		args = append(args, query.StartDate)
	}
	if query.EndDate != "" {
		parts = append(parts, "hour_key <= ?")
		args = append(args, query.EndDate+"T23")
	}
	if query.ProviderCode != "" {
		parts = append(parts, "provider_code = ?")
		args = append(args, query.ProviderCode)
	}
	if query.ModelAPIID != "" {
		parts = append(parts, "model_api_id = ?")
		args = append(args, query.ModelAPIID)
	}
	if query.AgentID != "" {
		parts = append(parts, "agent_id = ?")
		args = append(args, query.AgentID)
	}
	if len(parts) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(parts, " AND "), args
}
