package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"arenea/backend/internal/domain"
)

func (r *SQLiteRepository) GetActiveModelPricingRule(provider string, model string, at string) (domain.ModelPricingRule, error) {
	if at == "" {
		at = nowISO()
	}
	row := r.db.QueryRow(
		`SELECT id, provider_code, model_api_id, currency, input_price_micro_usd_per_1k, output_price_micro_usd_per_1k,
		 cached_input_price_micro_usd_per_1k, reasoning_price_micro_usd_per_1k, embedding_price_micro_usd_per_1k,
		 effective_from, effective_to, is_active, source, metadata_json, created_at, updated_at
		 FROM model_pricing_rules
		 WHERE provider_code = ? AND model_api_id = ? AND is_active = 1
		   AND effective_from <= ? AND (effective_to = '' OR effective_to > ?)
		 ORDER BY effective_from DESC LIMIT 1`,
		provider, model, at, at,
	)
	rule, err := scanModelPricingRule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ModelPricingRule{ProviderCode: provider, ModelAPIID: model, Currency: "USD"}, nil
	}
	return rule, err
}

func (r *SQLiteRepository) UpsertModelPricingRule(rule domain.ModelPricingRule) (domain.ModelPricingRule, error) {
	if rule.ProviderCode == "" || rule.ModelAPIID == "" {
		return domain.ModelPricingRule{}, errors.New("provider_code and model_api_id are required")
	}
	now := nowISO()
	if rule.Currency == "" {
		rule.Currency = "USD"
	}
	if rule.EffectiveFrom == "" {
		rule.EffectiveFrom = now
	}
	if rule.Source == "" {
		rule.Source = "manual"
	}
	if rule.MetadataJSON == "" {
		rule.MetadataJSON = "{}"
	}
	result, err := r.db.Exec(
		`UPDATE model_pricing_rules SET currency = ?, input_price_micro_usd_per_1k = ?, output_price_micro_usd_per_1k = ?,
		 cached_input_price_micro_usd_per_1k = ?, reasoning_price_micro_usd_per_1k = ?, embedding_price_micro_usd_per_1k = ?,
		 source = ?, metadata_json = ?, updated_at = ?
		 WHERE provider_code = ? AND model_api_id = ? AND is_active = 1 AND effective_to = ''`,
		rule.Currency, rule.InputPriceMicroUSDPer1K, rule.OutputPriceMicroUSDPer1K, rule.CachedInputPriceMicroUSDPer1K, rule.ReasoningPriceMicroUSDPer1K, rule.EmbeddingPriceMicroUSDPer1K,
		rule.Source, rule.MetadataJSON, now, rule.ProviderCode, rule.ModelAPIID,
	)
	if err != nil {
		return domain.ModelPricingRule{}, err
	}
	affected, _ := result.RowsAffected()
	if affected > 0 {
		return r.GetActiveModelPricingRule(rule.ProviderCode, rule.ModelAPIID, now)
	}
	if rule.ID == "" {
		rule.ID = fmt.Sprintf("pricing:%s:%s:%d", rule.ProviderCode, strings.ReplaceAll(rule.ModelAPIID, "/", "_"), time.Now().UTC().UnixNano())
	}
	rule.IsActive = true
	rule.CreatedAt = now
	rule.UpdatedAt = now
	_, err = r.db.Exec(
		`INSERT INTO model_pricing_rules(id, provider_code, model_api_id, currency, input_price_micro_usd_per_1k, output_price_micro_usd_per_1k,
		 cached_input_price_micro_usd_per_1k, reasoning_price_micro_usd_per_1k, embedding_price_micro_usd_per_1k,
		 effective_from, effective_to, is_active, source, metadata_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.ID, rule.ProviderCode, rule.ModelAPIID, rule.Currency, rule.InputPriceMicroUSDPer1K, rule.OutputPriceMicroUSDPer1K,
		rule.CachedInputPriceMicroUSDPer1K, rule.ReasoningPriceMicroUSDPer1K, rule.EmbeddingPriceMicroUSDPer1K,
		rule.EffectiveFrom, rule.EffectiveTo, rule.IsActive, rule.Source, rule.MetadataJSON, rule.CreatedAt, rule.UpdatedAt,
	)
	if err != nil {
		return domain.ModelPricingRule{}, err
	}
	return rule, nil
}

func (r *SQLiteRepository) AddModelTokenUsageEvent(event domain.ModelTokenUsageEvent) (domain.ModelTokenUsageEvent, error) {
	if event.ID == "" {
		return domain.ModelTokenUsageEvent{}, errors.New("id is required")
	}
	if event.OccurredAt == "" {
		event.OccurredAt = nowISO()
	}
	if event.CreatedAt == "" {
		event.CreatedAt = event.OccurredAt
	}
	if event.DateKey == "" {
		event.DateKey = event.OccurredAt[:10]
	}
	if event.HourKey == "" {
		event.HourKey = event.OccurredAt[:13] + ":00"
	}
	if event.UsageKind == "" {
		event.UsageKind = "chat"
	}
	if event.CallCount <= 0 {
		event.CallCount = 1
	}
	if event.Status == "" {
		event.Status = "success"
	}
	if event.ModelCategoryJSON == "" {
		event.ModelCategoryJSON = "[]"
	}
	if event.MetadataJSON == "" {
		event.MetadataJSON = "{}"
	}
	streamEnabled := 0
	if event.StreamEnabled {
		streamEnabled = 1
	}
	tx, err := r.db.Begin()
	if err != nil {
		return domain.ModelTokenUsageEvent{}, err
	}
	defer tx.Rollback()
	if err = addModelTokenUsageEventTx(tx, event, streamEnabled); err != nil {
		return domain.ModelTokenUsageEvent{}, err
	}
	if err = updateSessionModelUsageTx(tx, event); err != nil {
		return domain.ModelTokenUsageEvent{}, err
	}
	if err = tx.Commit(); err != nil {
		return domain.ModelTokenUsageEvent{}, err
	}
	return event, nil
}

func (r *SQLiteRepository) UpsertModelTokenUsageDaily(event domain.ModelTokenUsageEvent) error {
	if event.DateKey == "" {
		return nil
	}
	successCount := 0
	failedCount := 0
	cancelledCount := 0
	switch event.Status {
	case "success":
		successCount = 1
	case "cancelled":
		cancelledCount = 1
	default:
		failedCount = 1
	}
	id := strings.Join([]string{event.DateKey, event.WorkspaceID, event.AgentID, event.ProviderCode, event.ModelAPIID, event.UsageKind}, ":")
	now := nowISO()
	_, err := r.db.Exec(
		`INSERT INTO model_token_usage_daily(
		 id, date_key, workspace_id, agent_id, agent_key, provider_code, model_api_id, usage_kind,
		 call_count, request_count, success_count, failed_count, cancelled_count,
		 input_tokens, output_tokens, cached_input_tokens, reasoning_tokens, embedding_tokens, total_tokens,
		 total_cost_micro_usd, avg_latency_ms, avg_tokens_per_second, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(date_key, workspace_id, agent_id, provider_code, model_api_id, usage_kind) DO UPDATE SET
		 call_count = call_count + excluded.call_count,
		 request_count = request_count + excluded.request_count,
		 success_count = success_count + excluded.success_count,
		 failed_count = failed_count + excluded.failed_count,
		 cancelled_count = cancelled_count + excluded.cancelled_count,
		 input_tokens = input_tokens + excluded.input_tokens,
		 output_tokens = output_tokens + excluded.output_tokens,
		 cached_input_tokens = cached_input_tokens + excluded.cached_input_tokens,
		 reasoning_tokens = reasoning_tokens + excluded.reasoning_tokens,
		 embedding_tokens = embedding_tokens + excluded.embedding_tokens,
		 total_tokens = total_tokens + excluded.total_tokens,
		 total_cost_micro_usd = total_cost_micro_usd + excluded.total_cost_micro_usd,
		 avg_latency_ms = ((avg_latency_ms * request_count) + excluded.avg_latency_ms) / (request_count + excluded.request_count),
		 avg_tokens_per_second = ((avg_tokens_per_second * request_count) + excluded.avg_tokens_per_second) / (request_count + excluded.request_count),
		 updated_at = excluded.updated_at`,
		id, event.DateKey, event.WorkspaceID, event.AgentID, event.AgentKey, event.ProviderCode, event.ModelAPIID, event.UsageKind,
		event.CallCount, successCount, failedCount, cancelledCount,
		event.InputTokens, event.OutputTokens, event.CachedInputTokens, event.ReasoningTokens, event.EmbeddingTokens, event.TotalTokens,
		event.TotalCostMicroUSD, float64(event.LatencyMS), event.TokensPerSecond, now, now,
	)
	return err
}

func (r *SQLiteRepository) GetModelUsageSummary(query domain.ModelUsageQuery) (domain.ModelUsageSummary, error) {
	where, args := usageWhere(query)
	row := r.db.QueryRow(
		`SELECT
		 COALESCE(SUM(call_count), 0), COUNT(*),
		 COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0),
		 COALESCE(SUM(CASE WHEN status = 'failed' OR status = 'timeout' THEN 1 ELSE 0 END), 0),
		 COALESCE(SUM(CASE WHEN status = 'cancelled' THEN 1 ELSE 0 END), 0),
		 COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_tokens), 0),
		 COALESCE(SUM(total_cost_micro_usd), 0), COALESCE(AVG(latency_ms), 0), COALESCE(AVG(tokens_per_second), 0)
		 FROM model_token_usage_events`+where,
		args...,
	)
	return scanModelUsageSummary(row)
}

func (r *SQLiteRepository) ListModelUsageTrends(query domain.ModelUsageQuery) ([]domain.ModelUsageTrendPoint, error) {
	where, args := usageWhere(query)
	rows, err := r.db.Query(
		`SELECT date_key,
		 COALESCE(SUM(call_count), 0),
		 COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_tokens), 0),
		 COALESCE(SUM(total_cost_micro_usd), 0),
		 COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0),
		 COALESCE(SUM(CASE WHEN status = 'failed' OR status = 'timeout' THEN 1 ELSE 0 END), 0),
		 COALESCE(SUM(CASE WHEN status = 'cancelled' THEN 1 ELSE 0 END), 0),
		 COALESCE(AVG(latency_ms), 0), COALESCE(AVG(tokens_per_second), 0)
		 FROM model_token_usage_events`+where+` GROUP BY date_key ORDER BY date_key ASC`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.ModelUsageTrendPoint{}
	for rows.Next() {
		var point domain.ModelUsageTrendPoint
		if err = rows.Scan(&point.DateKey, &point.CallCount, &point.InputTokens, &point.OutputTokens, &point.TotalTokens, &point.TotalCostMicroUSD, &point.SuccessCount, &point.FailedCount, &point.CancelledCount, &point.AvgLatencyMS, &point.AvgTokensPerSecond); err != nil {
			return nil, err
		}
		result = append(result, point)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) ListTopModelUsage(query domain.ModelUsageQuery) ([]domain.ModelUsageBreakdownRow, error) {
	where, args := usageWhere(query)
	args = append(args, usageLimit(query.Limit))
	rows, err := r.db.Query(
		`SELECT provider_code, model_api_id, MAX(model_display_name),
		 COALESCE(SUM(call_count), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_tokens), 0),
		 COALESCE(SUM(total_cost_micro_usd), 0), COALESCE(AVG(latency_ms), 0), COALESCE(AVG(tokens_per_second), 0),
		 COALESCE(1.0 * SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) / NULLIF(COUNT(*), 0), 0)
		 FROM model_token_usage_events`+where+` GROUP BY provider_code, model_api_id ORDER BY total_cost_micro_usd DESC, call_count DESC LIMIT ?`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.ModelUsageBreakdownRow{}
	for rows.Next() {
		var item domain.ModelUsageBreakdownRow
		if err = rows.Scan(&item.ProviderCode, &item.ModelAPIID, &item.ModelDisplayName, &item.CallCount, &item.InputTokens, &item.OutputTokens, &item.TotalTokens, &item.TotalCostMicroUSD, &item.AvgLatencyMS, &item.AvgTokensPerSecond, &item.SuccessRate); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) ListTopAgentUsage(query domain.ModelUsageQuery) ([]domain.ModelUsageBreakdownRow, error) {
	where, args := usageWhere(query)
	args = append(args, usageLimit(query.Limit))
	rows, err := r.db.Query(
		`SELECT agent_id, agent_key,
		 COALESCE(SUM(call_count), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_tokens), 0),
		 COALESCE(SUM(total_cost_micro_usd), 0), COALESCE(AVG(latency_ms), 0), COALESCE(AVG(tokens_per_second), 0),
		 COALESCE(1.0 * SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) / NULLIF(COUNT(*), 0), 0)
		 FROM model_token_usage_events`+where+` GROUP BY agent_id, agent_key ORDER BY total_cost_micro_usd DESC, call_count DESC LIMIT ?`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.ModelUsageBreakdownRow{}
	for rows.Next() {
		var item domain.ModelUsageBreakdownRow
		if err = rows.Scan(&item.AgentID, &item.AgentKey, &item.CallCount, &item.InputTokens, &item.OutputTokens, &item.TotalTokens, &item.TotalCostMicroUSD, &item.AvgLatencyMS, &item.AvgTokensPerSecond, &item.SuccessRate); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) ListModelUsageEvents(query domain.ModelUsageQuery) ([]domain.ModelTokenUsageEvent, error) {
	where, args := usageWhere(query)
	args = append(args, usageLimit(query.Limit))
	rows, err := r.db.Query(
		`SELECT id, occurred_at, date_key, hour_key, workspace_id, user_id, team_id, agent_id, agent_key, session_id, message_id, request_id,
		 provider_code, provider_type, provider_display_name, model_api_id, model_display_name, model_category_json, usage_kind, call_count,
		 input_tokens, output_tokens, cached_input_tokens, reasoning_tokens, embedding_tokens, total_tokens,
		 input_price_micro_usd_per_1k, output_price_micro_usd_per_1k, cached_input_price_micro_usd_per_1k, reasoning_price_micro_usd_per_1k, embedding_price_micro_usd_per_1k,
		 input_cost_micro_usd, output_cost_micro_usd, cached_input_cost_micro_usd, reasoning_cost_micro_usd, embedding_cost_micro_usd, total_cost_micro_usd,
		 latency_ms, time_to_first_token_ms, tokens_per_second, status, error_code, error_message, retry_count,
		 prompt_mode, max_output_tokens, context_window_k, stream_enabled, metadata_json, created_at
		 FROM model_token_usage_events`+where+` ORDER BY occurred_at DESC LIMIT ?`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.ModelTokenUsageEvent{}
	for rows.Next() {
		event, err := scanModelTokenUsageEvent(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	return result, rows.Err()
}

func scanModelPricingRule(row scanner) (domain.ModelPricingRule, error) {
	var v domain.ModelPricingRule
	err := row.Scan(&v.ID, &v.ProviderCode, &v.ModelAPIID, &v.Currency, &v.InputPriceMicroUSDPer1K, &v.OutputPriceMicroUSDPer1K, &v.CachedInputPriceMicroUSDPer1K, &v.ReasoningPriceMicroUSDPer1K, &v.EmbeddingPriceMicroUSDPer1K, &v.EffectiveFrom, &v.EffectiveTo, &v.IsActive, &v.Source, &v.MetadataJSON, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

func scanModelUsageSummary(row scanner) (domain.ModelUsageSummary, error) {
	var v domain.ModelUsageSummary
	err := row.Scan(&v.CallCount, &v.RequestCount, &v.SuccessCount, &v.FailedCount, &v.CancelledCount, &v.InputTokens, &v.OutputTokens, &v.TotalTokens, &v.TotalCostMicroUSD, &v.AvgLatencyMS, &v.AvgTokensPerSecond)
	if v.RequestCount > 0 {
		v.SuccessRate = float64(v.SuccessCount) / float64(v.RequestCount)
	}
	return v, err
}

func scanModelTokenUsageEvent(row scanner) (domain.ModelTokenUsageEvent, error) {
	var v domain.ModelTokenUsageEvent
	var streamEnabled int
	err := row.Scan(
		&v.ID, &v.OccurredAt, &v.DateKey, &v.HourKey, &v.WorkspaceID, &v.UserID, &v.TeamID, &v.AgentID, &v.AgentKey, &v.SessionID, &v.MessageID, &v.RequestID,
		&v.ProviderCode, &v.ProviderType, &v.ProviderDisplayName, &v.ModelAPIID, &v.ModelDisplayName, &v.ModelCategoryJSON, &v.UsageKind, &v.CallCount,
		&v.InputTokens, &v.OutputTokens, &v.CachedInputTokens, &v.ReasoningTokens, &v.EmbeddingTokens, &v.TotalTokens,
		&v.InputPriceMicroUSDPer1K, &v.OutputPriceMicroUSDPer1K, &v.CachedInputPriceMicroUSDPer1K, &v.ReasoningPriceMicroUSDPer1K, &v.EmbeddingPriceMicroUSDPer1K,
		&v.InputCostMicroUSD, &v.OutputCostMicroUSD, &v.CachedInputCostMicroUSD, &v.ReasoningCostMicroUSD, &v.EmbeddingCostMicroUSD, &v.TotalCostMicroUSD,
		&v.LatencyMS, &v.TimeToFirstTokenMS, &v.TokensPerSecond, &v.Status, &v.ErrorCode, &v.ErrorMessage, &v.RetryCount,
		&v.PromptMode, &v.MaxOutputTokens, &v.ContextWindowK, &streamEnabled, &v.MetadataJSON, &v.CreatedAt,
	)
	v.StreamEnabled = streamEnabled != 0
	return v, err
}

func usageWhere(query domain.ModelUsageQuery) (string, []any) {
	parts := []string{}
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
	if query.Status != "" {
		if query.Status == "abnormal" {
			parts = append(parts, "status <> 'success'")
		} else {
			parts = append(parts, "status = ?")
			args = append(args, query.Status)
		}
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
