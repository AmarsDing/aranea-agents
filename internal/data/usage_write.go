package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/modelregistry"
)

func (r *usageRepo) RecordTokenUsageEvent(ctx context.Context, e biz.TokenUsageEvent) (biz.TokenUsageEvent, error) {
	streamEnabled := 0
	if e.StreamEnabled {
		streamEnabled = 1
	}
	if strings.TrimSpace(e.CanonicalProviderCode) == "" {
		e.CanonicalProviderCode = modelregistry.MigrateProviderCode(e.ProviderCode)
	}

	_, err := r.data.RW().Write(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`INSERT INTO model_token_usage_events(
		 id, occurred_at, date_key, hour_key, workspace_id, user_id, team_id, agent_id, agent_key, session_id, message_id, request_id,
		 provider_code, canonical_provider_code, provider_type, provider_display_name, model_api_id, model_display_name, model_category_json, usage_kind, call_count,
		 input_tokens, output_tokens, cached_input_tokens, cache_write_tokens, reasoning_tokens, embedding_tokens, total_tokens,
		 input_price_micro_usd_per_1k, output_price_micro_usd_per_1k, cached_input_price_micro_usd_per_1k, cache_write_price_micro_usd_per_1k, reasoning_price_micro_usd_per_1k, embedding_price_micro_usd_per_1k,
		 input_cost_micro_usd, output_cost_micro_usd, cached_input_cost_micro_usd, cache_write_cost_micro_usd, reasoning_cost_micro_usd, embedding_cost_micro_usd, total_cost_micro_usd,
		 latency_ms, time_to_first_token_ms, tokens_per_second, status, error_code, error_message, retry_count,
		 prompt_mode, max_output_tokens, context_window_k, stream_enabled, metadata_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		e.ID, e.OccurredAt, e.DateKey, e.HourKey, e.WorkspaceID, e.UserID, e.TeamID, e.AgentID, e.AgentKey, e.SessionID, e.MessageID, e.RequestID,
		e.ProviderCode, e.CanonicalProviderCode, e.ProviderType, e.ProviderDisplayName, e.ModelAPIID, e.ModelDisplayName, e.ModelCategoryJSON, e.UsageKind, e.CallCount,
		e.InputTokens, e.OutputTokens, e.CachedInputTokens, e.CacheWriteTokens, e.ReasoningTokens, e.EmbeddingTokens, e.TotalTokens,
		e.InputPriceMicroUSDPer1K, e.OutputPriceMicroUSDPer1K, e.CachedInputPriceMicroUSDPer1K, e.CacheWritePriceMicroUSDPer1K, e.ReasoningPriceMicroUSDPer1K, e.EmbeddingPriceMicroUSDPer1K,
		e.InputCostMicroUSD, e.OutputCostMicroUSD, e.CachedInputCostMicroUSD, e.CacheWriteCostMicroUSD, e.ReasoningCostMicroUSD, e.EmbeddingCostMicroUSD, e.TotalCostMicroUSD,
		e.LatencyMS, e.TimeToFirstTokenMS, e.TokensPerSecond, e.Status, e.ErrorCode, e.ErrorMessage, e.RetryCount,
		e.PromptMode, e.MaxOutputTokens, e.ContextWindowK, streamEnabled, e.MetadataJSON, e.CreatedAt,
	)
	if err != nil {
		return biz.TokenUsageEvent{}, err
	}

	return e, nil
}

func (r *usageRepo) RollupDailyHourly(ctx context.Context, e biz.TokenUsageEvent) error {
	c := r.data.RW().Write(ctx)
	dialect := r.data.Dialect()
	if err := upsertModelTokenUsageDaily(ctx, c, dialect, e); err != nil {
		return err
	}
	if err := upsertModelTokenUsageHourly(ctx, c, dialect, e); err != nil {
		return err
	}
	return nil
}

func (r *usageRepo) PurgeUsageEventsOlderThan(ctx context.Context, retainDays int) (int64, error) {
	result, err := r.data.RW().Write(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`DELETE FROM model_token_usage_events WHERE date_key < date('now', '-'||?||' days')`),
		retainDays,
	)
	if err != nil {
		return 0, err
	}
	affected, _ := result.RowsAffected()
	return affected, nil
}

func upsertModelTokenUsageDaily(ctx context.Context, c execer, dialect Dialect, e biz.TokenUsageEvent) error {
	if strings.TrimSpace(e.DateKey) == "" {
		return nil
	}
	successCount := 0
	failedCount := 0
	cancelledCount := 0
	switch e.Status {
	case "success":
		successCount = 1
	case biz.SessionRunPhaseCancelled:
		cancelledCount = 1
	default:
		failedCount = 1
	}
	id := strings.Join([]string{e.DateKey, e.WorkspaceID, e.AgentID, e.ProviderCode, e.ModelAPIID, e.UsageKind}, ":")
	now := nowRFC3339()
	_, err := c.ExecContext(ctx,
		dialect.RenumberPlaceholders(`INSERT INTO model_token_usage_daily(
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
		 avg_latency_ms = (avg_latency_ms * request_count + excluded.avg_latency_ms * excluded.request_count) / (request_count + excluded.request_count),
		 avg_tokens_per_second = (avg_tokens_per_second * request_count + excluded.avg_tokens_per_second * excluded.request_count) / (request_count + excluded.request_count),
		 updated_at = excluded.updated_at`),
		id, e.DateKey, e.WorkspaceID, e.AgentID, e.AgentKey, e.ProviderCode, e.ModelAPIID, e.UsageKind,
		e.CallCount, successCount, failedCount, cancelledCount,
		e.InputTokens, e.OutputTokens, e.CachedInputTokens, e.ReasoningTokens, e.EmbeddingTokens, e.TotalTokens,
		e.TotalCostMicroUSD, float64(e.LatencyMS), e.TokensPerSecond, now, now,
	)
	return err
}

func upsertModelTokenUsageHourly(ctx context.Context, c execer, dialect Dialect, e biz.TokenUsageEvent) error {
	hourKey := strings.TrimSpace(e.HourKey)
	if hourKey == "" {
		return nil
	}
	successCount := 0
	failedCount := 0
	cancelledCount := 0
	switch e.Status {
	case "success":
		successCount = 1
	case biz.SessionRunPhaseCancelled:
		cancelledCount = 1
	default:
		failedCount = 1
	}
	id := strings.Join([]string{hourKey, e.WorkspaceID, e.AgentID, e.ProviderCode, e.ModelAPIID, e.UsageKind}, ":")
	now := nowRFC3339()
	_, err := c.ExecContext(ctx,
		dialect.RenumberPlaceholders(`INSERT INTO model_token_usage_hourly(
		 id, hour_key, workspace_id, agent_id, agent_key, provider_code, model_api_id, usage_kind,
		 call_count, request_count, success_count, failed_count, cancelled_count,
		 input_tokens, output_tokens, cached_input_tokens, reasoning_tokens, embedding_tokens, total_tokens,
		 total_cost_micro_usd, avg_latency_ms, avg_tokens_per_second, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(hour_key, workspace_id, agent_id, provider_code, model_api_id, usage_kind) DO UPDATE SET
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
		 avg_latency_ms = (avg_latency_ms * request_count + excluded.avg_latency_ms * excluded.request_count) / (request_count + excluded.request_count),
		 avg_tokens_per_second = (avg_tokens_per_second * request_count + excluded.avg_tokens_per_second * excluded.request_count) / (request_count + excluded.request_count),
		 updated_at = excluded.updated_at`),
		id, hourKey, e.WorkspaceID, e.AgentID, e.AgentKey, e.ProviderCode, e.ModelAPIID, e.UsageKind,
		e.CallCount, successCount, failedCount, cancelledCount,
		e.InputTokens, e.OutputTokens, e.CachedInputTokens, e.ReasoningTokens, e.EmbeddingTokens, e.TotalTokens,
		e.TotalCostMicroUSD, float64(e.LatencyMS), e.TokensPerSecond, now, now,
	)
	return err
}
