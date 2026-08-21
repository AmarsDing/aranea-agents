package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
)

// breakdownQueryWhere builds the WHERE clause for the all-models breakdown query.
// Provider filter uses usageProviderWhere (alias expansion) for consistency with usageDailyWhere.
// Supports an optional LIKE search across provider_code + model_api_id.
func breakdownQueryWhere(query biz.UsageBreakdownQuery) (string, []any) {
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
	if s := strings.TrimSpace(query.Search); s != "" {
		parts = append(parts, "(provider_code LIKE ? OR model_api_id LIKE ?)")
		like := "%" + s + "%"
		args = append(args, like, like)
	}
	if clause, wsArgs := usageWorkspaceClause(query.WorkspaceID); clause != "" {
		parts = append(parts, clause)
		args = append(args, wsArgs...)
	}
	return " WHERE " + strings.Join(parts, " AND "), args
}

// breakdownSortFieldWhitelist maps user-facing sort field names to SQL expressions.
// Only keys in this map are allowed; unknown fields fall back to call_count DESC.
// This prevents SQL injection via sort_field parameter.
// Daily-table variant: avg_* / success_rate are stored columns maintained by rollup.
var breakdownSortFieldWhitelist = map[string]string{
	"call_count":           "SUM(call_count)",
	"total_tokens":         "SUM(total_tokens)",
	"total_cost_micro_usd": "SUM(total_cost_micro_usd)",
	"success_rate":         "success_rate",
	"avg_latency_ms":       "avg_latency_ms",
}

// breakdownSortFieldWhitelistRealtime is the events-table variant: averages and
// success_rate must be computed from raw rows (same expressions as ListTopModelUsage).
var breakdownSortFieldWhitelistRealtime = map[string]string{
	"call_count":           "SUM(call_count)",
	"total_tokens":         "SUM(total_tokens)",
	"total_cost_micro_usd": "SUM(total_cost_micro_usd)",
	"success_rate":         "1.0 * SUM(CASE WHEN " + sqlUsageStatusSuccess + " THEN 1 ELSE 0 END) / NULLIF(COUNT(*), 0)",
	"avg_latency_ms":       "AVG(latency_ms)",
}

// breakdownSortClause returns the ORDER BY clause for the breakdown query.
// field must be in the whitelist; dir must be "asc" or "desc" (case-insensitive).
// Unknown values fall back to "SUM(call_count) DESC" to ensure deterministic ordering.
func breakdownSortClause(field, dir string) string {
	return breakdownSortClauseFrom(breakdownSortFieldWhitelist, field, dir)
}

func breakdownSortClauseFrom(whitelist map[string]string, field, dir string) string {
	expr, ok := whitelist[strings.ToLower(strings.TrimSpace(field))]
	if !ok {
		expr = "SUM(call_count)"
	}
	d := strings.ToLower(strings.TrimSpace(dir))
	if d != "asc" && d != "desc" {
		d = "desc"
	}
	// Secondary sort by call_count to ensure stable ordering when primary field ties.
	secondary := "SUM(call_count) DESC"
	if expr == "SUM(call_count)" {
		// Avoid duplicating the primary field; use total_cost_micro_usd as tiebreaker.
		secondary = "SUM(total_cost_micro_usd) DESC"
	}
	return expr + " " + strings.ToUpper(d) + ", " + secondary
}

// ListAllModelsBreakdown returns a paginated, searchable, sortable breakdown of all
// models in the daily usage table. Unlike ListTopModelUsageFromDaily (which is hard-capped
// at 200 rows and sorted by cost), this method supports server-side pagination, dynamic
// sorting, and LIKE search — suitable for a full-table overview UI.
//
// Known limitation: mergeUsageBreakdownByAlias is applied AFTER pagination, so when
// multiple raw rows share a canonical alias (e.g., "openai" + "openai-proxy" → "openai"),
// the merged page may show fewer rows than pageSize. Total reflects raw (pre-merge) count.
// Acceptable for v1 because alias collisions are rare in practice.
func (r *usageRepo) ListAllModelsBreakdown(ctx context.Context, query biz.UsageBreakdownQuery) (biz.UsageBreakdownResult, error) {
	where, args := breakdownQueryWhere(query)
	limit, offset, pageOut, pageSizeOut := shared.PageToLimitOffset(query.Page, query.PageSize)

	// 1) Count total matching rows (before pagination).
	countSQL := r.data.Dialect().RenumberPlaceholders(
		"SELECT COUNT(*) FROM (SELECT 1 FROM model_token_usage_daily" + where +
			" GROUP BY provider_code, model_api_id) AS _sub")
	var total int32
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), countSQL, args, &total); err != nil {
		return biz.UsageBreakdownResult{}, entErrToBizErr(err, apierror.DomainData)
	}

	// 2) Fetch the paginated rows.
	order := breakdownSortClause(query.SortField, query.SortDir)
	dataArgs := append(append([]any{}, args...), limit, offset)
	dataSQL := r.data.Dialect().RenumberPlaceholders(`SELECT provider_code, model_api_id,
		 COALESCE(SUM(call_count), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_tokens), 0),
		 COALESCE(SUM(total_cost_micro_usd), 0),
		 COALESCE(SUM(avg_latency_ms * request_count) / NULLIF(SUM(request_count), 0), 0),
		 COALESCE(SUM(avg_tokens_per_second * request_count) / NULLIF(SUM(request_count), 0), 0),
		 COALESCE(1.0 * SUM(success_count) / NULLIF(SUM(request_count), 0), 0)
		 FROM model_token_usage_daily` + where +
		" GROUP BY provider_code, model_api_id ORDER BY " + order + " LIMIT ? OFFSET ?")
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, dataSQL, dataArgs...)
	if err != nil {
		return biz.UsageBreakdownResult{}, entErrToBizErr(err, apierror.DomainData)
	}
	defer rows.Close()

	items := make([]biz.UsageBreakdownRow, 0, pageSizeOut)
	for rows.Next() {
		var item biz.UsageBreakdownRow
		if err = rows.Scan(&item.ProviderCode, &item.ModelAPIID, &item.CallCount, &item.InputTokens, &item.OutputTokens, &item.TotalTokens, &item.TotalCostMicroUSD, &item.AvgLatencyMS, &item.AvgTokensPerSecond, &item.SuccessRate); err != nil {
			return biz.UsageBreakdownResult{}, entErrToBizErr(err, apierror.DomainData)
		}
		item.ModelDisplayName = item.ModelAPIID
		items = append(items, item)
	}
	if err = entErrToBizErr(rows.Err(), apierror.DomainData); err != nil {
		return biz.UsageBreakdownResult{}, err
	}
	return biz.UsageBreakdownResult{
		Items:    mergeUsageBreakdownByAlias(items),
		Total:    total,
		Page:     pageOut,
		PageSize: pageSizeOut,
	}, nil
}

// ListAllModelsBreakdownRealtime is the events-table counterpart of
// ListAllModelsBreakdown. It exists because the daily rollup is asynchronous
// (event-bus consumer): when the queried range includes today, the daily table
// lags the events table and the overview page's summary/top-models blocks
// (which query events in that case) would disagree with this table. Aggregates
// and success semantics mirror ListTopModelUsage so all three blocks stay
// consistent regardless of range.
func (r *usageRepo) ListAllModelsBreakdownRealtime(ctx context.Context, query biz.UsageBreakdownQuery) (biz.UsageBreakdownResult, error) {
	where, args := breakdownQueryWhere(query)
	limit, offset, pageOut, pageSizeOut := shared.PageToLimitOffset(query.Page, query.PageSize)

	countSQL := r.data.Dialect().RenumberPlaceholders(
		"SELECT COUNT(*) FROM (SELECT 1 FROM model_token_usage_events" + where +
			" GROUP BY provider_code, model_api_id) AS _sub")
	var total int32
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), countSQL, args, &total); err != nil {
		return biz.UsageBreakdownResult{}, entErrToBizErr(err, apierror.DomainData)
	}

	order := breakdownSortClauseFrom(breakdownSortFieldWhitelistRealtime, query.SortField, query.SortDir)
	dataArgs := append(append([]any{}, args...), limit, offset)
	dataSQL := r.data.Dialect().RenumberPlaceholders(`SELECT provider_code, model_api_id,
		 COALESCE(SUM(call_count), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_tokens), 0),
		 COALESCE(SUM(total_cost_micro_usd), 0),
		 COALESCE(AVG(latency_ms), 0), COALESCE(AVG(tokens_per_second), 0),
		 COALESCE(1.0 * SUM(CASE WHEN ` + sqlUsageStatusSuccess + ` THEN 1 ELSE 0 END) / NULLIF(COUNT(*), 0), 0)
		 FROM model_token_usage_events` + where +
		" GROUP BY provider_code, model_api_id ORDER BY " + order + " LIMIT ? OFFSET ?")
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, dataSQL, dataArgs...)
	if err != nil {
		return biz.UsageBreakdownResult{}, entErrToBizErr(err, apierror.DomainData)
	}
	defer rows.Close()

	items := make([]biz.UsageBreakdownRow, 0, pageSizeOut)
	for rows.Next() {
		var item biz.UsageBreakdownRow
		if err = rows.Scan(&item.ProviderCode, &item.ModelAPIID, &item.CallCount, &item.InputTokens, &item.OutputTokens, &item.TotalTokens, &item.TotalCostMicroUSD, &item.AvgLatencyMS, &item.AvgTokensPerSecond, &item.SuccessRate); err != nil {
			return biz.UsageBreakdownResult{}, entErrToBizErr(err, apierror.DomainData)
		}
		item.ModelDisplayName = item.ModelAPIID
		items = append(items, item)
	}
	if err = entErrToBizErr(rows.Err(), apierror.DomainData); err != nil {
		return biz.UsageBreakdownResult{}, err
	}
	return biz.UsageBreakdownResult{
		Items:    mergeUsageBreakdownByAlias(items),
		Total:    total,
		Page:     pageOut,
		PageSize: pageSizeOut,
	}, nil
}
