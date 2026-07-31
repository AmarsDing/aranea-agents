package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	bizusage "aranea-agents/internal/biz/usage"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
)

type usageRepo struct {
	data *Data
}

var _ bizusage.Repo = (*usageRepo)(nil)

// NewUsageRepo implements biz.UsageRepo (SQLite model_token_usage_events aggregates).
func NewUsageRepo(d *Data) biz.UsageRepo {
	return &usageRepo{data: d}
}

func (r *usageRepo) GetModelUsageSummary(ctx context.Context, query biz.UsageQuery) (biz.UsageSummary, error) {
	where, args := usageWhere(query, true)
	q := `SELECT
		 COALESCE(SUM(call_count), 0), COUNT(*),
		 COALESCE(SUM(CASE WHEN ` + sqlUsageStatusSuccess + ` THEN 1 ELSE 0 END), 0),
		 COALESCE(SUM(CASE WHEN ` + sqlUsageStatusFailed + ` THEN 1 ELSE 0 END), 0),
		 COALESCE(SUM(CASE WHEN status = 'cancelled' THEN 1 ELSE 0 END), 0),
		 COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_tokens), 0),
		 COALESCE(SUM(total_cost_micro_usd), 0), COALESCE(AVG(latency_ms), 0), COALESCE(AVG(tokens_per_second), 0)
		 FROM model_token_usage_events` + where
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

func (r *usageRepo) ListModelUsageTrends(ctx context.Context, query biz.UsageQuery) ([]biz.UsageTrendPoint, error) {
	where, args := usageWhere(query, true)
	q := r.data.Dialect().RenumberPlaceholders(`SELECT date_key,
		 COALESCE(SUM(call_count), 0),
		 COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_tokens), 0),
		 COALESCE(SUM(total_cost_micro_usd), 0),
		 COALESCE(SUM(CASE WHEN ` + sqlUsageStatusSuccess + ` THEN 1 ELSE 0 END), 0),
		 COALESCE(SUM(CASE WHEN ` + sqlUsageStatusFailed + ` THEN 1 ELSE 0 END), 0),
		 COALESCE(SUM(CASE WHEN status = 'cancelled' THEN 1 ELSE 0 END), 0),
		 COALESCE(AVG(latency_ms), 0), COALESCE(AVG(tokens_per_second), 0)
		 FROM model_token_usage_events` + where + ` GROUP BY date_key ORDER BY date_key ASC`)
	rows, err := r.data.RW().Read(ctx).QueryContext(ctx, q, args...)
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

func (r *usageRepo) ListTopModelUsage(ctx context.Context, query biz.UsageQuery) ([]biz.UsageBreakdownRow, error) {
	where, args := usageWhere(query, true)
	args = append(args, usageLimit(query.Limit))
	q := r.data.Dialect().RenumberPlaceholders(`SELECT provider_code, model_api_id, MAX(model_display_name),
		 COALESCE(SUM(call_count), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_tokens), 0),
		 COALESCE(SUM(total_cost_micro_usd), 0), COALESCE(AVG(latency_ms), 0), COALESCE(AVG(tokens_per_second), 0),
		 COALESCE(1.0 * SUM(CASE WHEN ` + sqlUsageStatusSuccess + ` THEN 1 ELSE 0 END) / NULLIF(COUNT(*), 0), 0)
		 FROM model_token_usage_events` + where + ` GROUP BY provider_code, model_api_id ORDER BY SUM(total_cost_micro_usd) DESC, SUM(call_count) DESC LIMIT ?`)
	rows, err := r.data.RW().Read(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainData)
	}
	defer rows.Close()
	var result []biz.UsageBreakdownRow
	for rows.Next() {
		var item biz.UsageBreakdownRow
		if err = rows.Scan(&item.ProviderCode, &item.ModelAPIID, &item.ModelDisplayName, &item.CallCount, &item.InputTokens, &item.OutputTokens, &item.TotalTokens, &item.TotalCostMicroUSD, &item.AvgLatencyMS, &item.AvgTokensPerSecond, &item.SuccessRate); err != nil {
			return nil, entErrToBizErr(err, apierror.DomainData)
		}
		result = append(result, item)
	}
	return mergeUsageBreakdownByAlias(result), entErrToBizErr(rows.Err(), apierror.DomainData)
}

// sqlUsageEventNames resolves human-readable names via scalar subqueries
// (agents/teams/sessions are Ent-managed tables in the same database).
// Subqueries instead of JOINs keep the WHERE clause columns unambiguous and
// never inflate the COUNT query. agent_id/team_id may carry either the row id
// or the *_key, so both are matched; agent_name falls back to the session's
// agent (same pattern as monitor traces, see sqlMonitorTracesNames).
const sqlUsageEventNames = `,
	 COALESCE(
	   (SELECT a.display_name FROM agents a WHERE (a.id = model_token_usage_events.agent_id OR a.agent_key = model_token_usage_events.agent_id) AND a.deleted_at = '' LIMIT 1),
	   (SELECT a2.display_name FROM sessions s2 JOIN agents a2 ON (a2.id = s2.agent_id OR a2.agent_key = s2.agent_id)
	     WHERE s2.id = model_token_usage_events.session_id AND s2.deleted_at = '' AND a2.deleted_at = '' LIMIT 1),
	   ''
	 ) AS agent_name,
	 COALESCE((SELECT s3.title FROM sessions s3 WHERE s3.id = model_token_usage_events.session_id AND s3.deleted_at = '' LIMIT 1), '') AS session_title,
	 COALESCE((SELECT tm.display_name FROM teams tm WHERE (tm.id = model_token_usage_events.team_id OR tm.team_key = model_token_usage_events.team_id) AND tm.deleted_at = '' LIMIT 1), '') AS team_name`

func (r *usageRepo) ListModelUsageEvents(ctx context.Context, query biz.UsageQuery) ([]biz.TokenUsageEvent, error) {
	where, args := usageWhere(query, false)
	args = append(args, usageLimit(query.Limit), usageOffset(query.Offset))
	q := r.data.Dialect().RenumberPlaceholders(`SELECT id, occurred_at, date_key, hour_key, workspace_id, user_id, team_id, agent_id, agent_key, session_id, message_id, request_id,
	 provider_code, COALESCE(canonical_provider_code, ''), provider_type, provider_display_name, model_api_id, model_display_name, model_category_json, usage_kind, call_count,
	 input_tokens, output_tokens, cached_input_tokens, COALESCE(cache_write_tokens, 0), reasoning_tokens, embedding_tokens, total_tokens,
	 input_price_micro_usd_per_1k, output_price_micro_usd_per_1k, cached_input_price_micro_usd_per_1k, COALESCE(cache_write_price_micro_usd_per_1k, 0), reasoning_price_micro_usd_per_1k, embedding_price_micro_usd_per_1k,
	 input_cost_micro_usd, output_cost_micro_usd, cached_input_cost_micro_usd, COALESCE(cache_write_cost_micro_usd, 0), reasoning_cost_micro_usd, embedding_cost_micro_usd, total_cost_micro_usd,
	 latency_ms, time_to_first_token_ms, tokens_per_second, status, error_code, error_message, retry_count,
	 prompt_mode, max_output_tokens, context_window_k, stream_enabled, metadata_json, created_at` +
		sqlUsageEventNames + `
	 FROM model_token_usage_events` + where + ` ORDER BY occurred_at DESC LIMIT ? OFFSET ?`)
	rows, err := r.data.RW().Read(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainData)
	}
	defer rows.Close()
	var result []biz.TokenUsageEvent
	for rows.Next() {
		event, err := scanTokenUsageEvent(rows)
		if err != nil {
			return nil, entErrToBizErr(err, apierror.DomainData)
		}
		result = append(result, aliasUsageEvent(event))
	}
	return result, entErrToBizErr(rows.Err(), apierror.DomainData)
}

func (r *usageRepo) CountModelUsageEvents(ctx context.Context, query biz.UsageQuery) (int, error) {
	where, args := usageWhere(query, false)
	q := r.data.Dialect().RenumberPlaceholders(`SELECT COUNT(*) FROM model_token_usage_events` + where)
	var n int
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), q, args, &n); err != nil {
		return 0, entErrToBizErr(err, apierror.DomainData)
	}
	return n, nil
}

func (r *usageRepo) ListTopAgentUsage(ctx context.Context, query biz.UsageQuery) ([]biz.UsageBreakdownRow, error) {
	where, args := usageWhere(query, true)
	args = append(args, usageLimit(query.Limit))
	q := r.data.Dialect().RenumberPlaceholders(`SELECT agent_id, agent_key,
		 COALESCE(SUM(call_count), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_tokens), 0),
		 COALESCE(SUM(total_cost_micro_usd), 0), COALESCE(AVG(latency_ms), 0), COALESCE(AVG(tokens_per_second), 0),
		 COALESCE(1.0 * SUM(CASE WHEN ` + sqlUsageStatusSuccess + ` THEN 1 ELSE 0 END) / NULLIF(COUNT(*), 0), 0)
		 FROM model_token_usage_events` + where + ` GROUP BY agent_id, agent_key ORDER BY SUM(total_cost_micro_usd) DESC, SUM(call_count) DESC LIMIT ?`)
	rows, err := r.data.RW().Read(ctx).QueryContext(ctx, q, args...)
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

type scanner interface {
	Scan(dest ...any) error
}

func scanTokenUsageEvent(row scanner) (biz.TokenUsageEvent, error) {
	var v biz.TokenUsageEvent
	var streamEnabled int
	err := row.Scan(
		&v.ID, &v.OccurredAt, &v.DateKey, &v.HourKey, &v.WorkspaceID, &v.UserID, &v.TeamID, &v.AgentID, &v.AgentKey, &v.SessionID, &v.MessageID, &v.RequestID,
		&v.ProviderCode, &v.CanonicalProviderCode, &v.ProviderType, &v.ProviderDisplayName, &v.ModelAPIID, &v.ModelDisplayName, &v.ModelCategoryJSON, &v.UsageKind, &v.CallCount,
		&v.InputTokens, &v.OutputTokens, &v.CachedInputTokens, &v.CacheWriteTokens, &v.ReasoningTokens, &v.EmbeddingTokens, &v.TotalTokens,
		&v.InputPriceMicroUSDPer1K, &v.OutputPriceMicroUSDPer1K, &v.CachedInputPriceMicroUSDPer1K, &v.CacheWritePriceMicroUSDPer1K, &v.ReasoningPriceMicroUSDPer1K, &v.EmbeddingPriceMicroUSDPer1K,
		&v.InputCostMicroUSD, &v.OutputCostMicroUSD, &v.CachedInputCostMicroUSD, &v.CacheWriteCostMicroUSD, &v.ReasoningCostMicroUSD, &v.EmbeddingCostMicroUSD, &v.TotalCostMicroUSD,
		&v.LatencyMS, &v.TimeToFirstTokenMS, &v.TokensPerSecond, &v.Status, &v.ErrorCode, &v.ErrorMessage, &v.RetryCount,
		&v.PromptMode, &v.MaxOutputTokens, &v.ContextWindowK, &streamEnabled, &v.MetadataJSON, &v.CreatedAt,
		&v.AgentName, &v.SessionTitle, &v.TeamName,
	)
	v.StreamEnabled = streamEnabled != 0
	return v, err
}

func usageWhere(query biz.UsageQuery, billableOnly bool) (string, []any) {
	parts := []string{}
	args := []any{}
	if billableOnly {
		parts = append(parts, sqlUsageBillableKind)
	}
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
		parts = append(parts, "team_id = ?")
		args = append(args, query.TeamID)
	}
	if query.UsageKind != "" {
		parts = append(parts, "usage_kind = ?")
		args = append(args, query.UsageKind)
	}
	if query.Status != "" {
		switch query.Status {
		case "abnormal", "error":
			parts = append(parts, "(status NOT IN ('success', 'ok'))")
		case "success":
			parts = append(parts, "(status IN ('success', 'ok'))")
		default:
			parts = append(parts, "status = ?")
			args = append(args, query.Status)
		}
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

func usageLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	if limit > 200 {
		return 200
	}
	return limit
}

// usageWorkspaceClause 返回用量事件的 workspace 过滤片段。
// 与 evaluation.go evalRunsWorkspaceFilter 的 legacy 兼容语义一致：
//   - 空 WorkspaceID（system caller）→ 无过滤
//   - default 租户 → workspace_id IN ('default', ”)，兼容历史空行
//     （用量事件写入路径不填 workspace_id，全部 1185 行均为空串）
//   - 其他租户 → 严格相等，防止跨租户泄漏
func usageWorkspaceClause(workspaceID string) (string, []any) {
	if workspaceID == "" {
		return "", nil
	}
	if workspaceID == workspace.DefaultWorkspaceID {
		return "workspace_id IN (?, ?)", []any{workspaceID, ""}
	}
	return "workspace_id = ?", []any{workspaceID}
}

func usageOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}
