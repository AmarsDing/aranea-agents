package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
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
	q = r.data.Dialect().RenumberPlaceholders(q)
	var v biz.UsageSummary
	err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), q, args,
		&v.CallCount, &v.RequestCount, &v.SuccessCount, &v.FailedCount, &v.CancelledCount,
		&v.InputTokens, &v.OutputTokens, &v.TotalTokens, &v.TotalCostMicroUSD, &v.AvgLatencyMS, &v.AvgTokensPerSecond)
	if err != nil {
		return biz.UsageSummary{}, entErrToBizErr(err, apierror.DomainData)
	}
	if v.RequestCount > 0 {
		v.SuccessRate = float64(v.SuccessCount) / float64(v.RequestCount)
	}
	return v, nil
}

func (r *usageRepo) ListModelUsageDailyTrends(ctx context.Context, query biz.UsageQuery) ([]biz.UsageTrendPoint, error) {
	where, args := usageDailyWhere(query)
	q := r.data.Dialect().RenumberPlaceholders(`SELECT date_key,
		 COALESCE(SUM(call_count), 0),
		 COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_tokens), 0),
		 COALESCE(SUM(total_cost_micro_usd), 0),
		 COALESCE(SUM(success_count), 0),
		 COALESCE(SUM(failed_count), 0),
		 COALESCE(SUM(cancelled_count), 0),
		 COALESCE(SUM(avg_latency_ms * request_count) / NULLIF(SUM(request_count), 0), 0),
		 COALESCE(SUM(avg_tokens_per_second * request_count) / NULLIF(SUM(request_count), 0), 0)
		 FROM model_token_usage_daily` + where + ` GROUP BY date_key ORDER BY date_key ASC`)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainData)
	}
	defer rows.Close()
	var result []biz.UsageTrendPoint
	for rows.Next() {
		var point biz.UsageTrendPoint
		if err = rows.Scan(&point.DateKey, &point.CallCount, &point.InputTokens, &point.OutputTokens, &point.TotalTokens, &point.TotalCostMicroUSD, &point.SuccessCount, &point.FailedCount, &point.CancelledCount, &point.AvgLatencyMS, &point.AvgTokensPerSecond); err != nil {
			return nil, entErrToBizErr(err, apierror.DomainData)
		}
		result = append(result, point)
	}
	return result, entErrToBizErr(rows.Err(), apierror.DomainData)
}

func (r *usageRepo) ListTopModelUsageFromDaily(ctx context.Context, query biz.UsageQuery) ([]biz.UsageBreakdownRow, error) {
	where, args := usageDailyWhere(query)
	args = append(args, usageLimit(query.Limit))
	q := r.data.Dialect().RenumberPlaceholders(`SELECT provider_code, model_api_id,
		 COALESCE(SUM(call_count), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_tokens), 0),
		 COALESCE(SUM(total_cost_micro_usd), 0),
		 COALESCE(SUM(avg_latency_ms * request_count) / NULLIF(SUM(request_count), 0), 0),
		 COALESCE(SUM(avg_tokens_per_second * request_count) / NULLIF(SUM(request_count), 0), 0),
		 COALESCE(1.0 * SUM(success_count) / NULLIF(SUM(request_count), 0), 0),
		 COALESCE(MAX(date_key), '')
		 FROM model_token_usage_daily` + where + ` GROUP BY provider_code, model_api_id ORDER BY SUM(total_cost_micro_usd) DESC, SUM(call_count) DESC LIMIT ?`)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainData)
	}
	defer rows.Close()
	var result []biz.UsageBreakdownRow
	for rows.Next() {
		var item biz.UsageBreakdownRow
		if err = rows.Scan(&item.ProviderCode, &item.ModelAPIID, &item.CallCount, &item.InputTokens, &item.OutputTokens, &item.TotalTokens, &item.TotalCostMicroUSD, &item.AvgLatencyMS, &item.AvgTokensPerSecond, &item.SuccessRate, &item.LastActiveDate); err != nil {
			return nil, entErrToBizErr(err, apierror.DomainData)
		}
		item.ModelDisplayName = item.ModelAPIID
		result = append(result, item)
	}
	return mergeUsageBreakdownByAlias(result), entErrToBizErr(rows.Err(), apierror.DomainData)
}

func (r *usageRepo) ListTopAgentUsageFromDaily(ctx context.Context, query biz.UsageQuery) ([]biz.UsageBreakdownRow, error) {
	where, args := usageDailyWhere(query)
	args = append(args, usageLimit(query.Limit))
	q := r.data.Dialect().RenumberPlaceholders(`SELECT agent_id, agent_key,
		 COALESCE(SUM(call_count), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_tokens), 0),
		 COALESCE(SUM(total_cost_micro_usd), 0),
		 COALESCE(SUM(avg_latency_ms * request_count) / NULLIF(SUM(request_count), 0), 0),
		 COALESCE(SUM(avg_tokens_per_second * request_count) / NULLIF(SUM(request_count), 0), 0),
		 COALESCE(1.0 * SUM(success_count) / NULLIF(SUM(request_count), 0), 0)
		 FROM model_token_usage_daily` + where + ` GROUP BY agent_id, agent_key ORDER BY SUM(total_cost_micro_usd) DESC, SUM(call_count) DESC LIMIT ?`)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainData)
	}
	defer rows.Close()
	var result []biz.UsageBreakdownRow
	for rows.Next() {
		var item biz.UsageBreakdownRow
		if err = rows.Scan(&item.AgentID, &item.AgentKey, &item.CallCount, &item.InputTokens, &item.OutputTokens, &item.TotalTokens, &item.TotalCostMicroUSD, &item.AvgLatencyMS, &item.AvgTokensPerSecond, &item.SuccessRate); err != nil {
			return nil, entErrToBizErr(err, apierror.DomainData)
		}
		result = append(result, item)
	}
	return result, entErrToBizErr(rows.Err(), apierror.DomainData)
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
	// NOTE: model_token_usage_daily 表无 team_id 列，不支持按 TeamID 过滤。
	// 调用方在使用 TeamID 过滤时应回退到实时查询路径（见 biz/usage Usecase.dailySupported）。
	if query.UsageKind != "" {
		parts = append(parts, "usage_kind = ?")
		args = append(args, query.UsageKind)
	}
	if clause, wsArgs := usageWorkspaceClause(query.WorkspaceID); clause != "" {
		parts = append(parts, clause)
		args = append(args, wsArgs...)
	}
	if len(parts) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(parts, " AND "), args
}
