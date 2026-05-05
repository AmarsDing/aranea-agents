package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"

	stdsql "database/sql"
)

func (r *usageRepo) RecordTokenUsageEvent(ctx context.Context, e biz.TokenUsageEvent) (biz.TokenUsageEvent, error) {
	tx, err := r.ent().BeginTx(ctx, nil)
	if err != nil {
		return biz.TokenUsageEvent{}, err
	}
	defer func() { _ = tx.Rollback() }()

	c := tx.Client()
	streamEnabled := 0
	if e.StreamEnabled {
		streamEnabled = 1
	}

	_, err = c.ExecContext(ctx,
		`INSERT INTO model_token_usage_events(
		 id, occurred_at, date_key, hour_key, workspace_id, user_id, team_id, agent_id, agent_key, session_id, message_id, request_id,
		 provider_code, provider_type, provider_display_name, model_api_id, model_display_name, model_category_json, usage_kind, call_count,
		 input_tokens, output_tokens, cached_input_tokens, reasoning_tokens, embedding_tokens, total_tokens,
		 input_price_micro_usd_per_1k, output_price_micro_usd_per_1k, cached_input_price_micro_usd_per_1k, reasoning_price_micro_usd_per_1k, embedding_price_micro_usd_per_1k,
		 input_cost_micro_usd, output_cost_micro_usd, cached_input_cost_micro_usd, reasoning_cost_micro_usd, embedding_cost_micro_usd, total_cost_micro_usd,
		 latency_ms, time_to_first_token_ms, tokens_per_second, status, error_code, error_message, retry_count,
		 prompt_mode, max_output_tokens, context_window_k, stream_enabled, metadata_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.OccurredAt, e.DateKey, e.HourKey, e.WorkspaceID, e.UserID, e.TeamID, e.AgentID, e.AgentKey, e.SessionID, e.MessageID, e.RequestID,
		e.ProviderCode, e.ProviderType, e.ProviderDisplayName, e.ModelAPIID, e.ModelDisplayName, e.ModelCategoryJSON, e.UsageKind, e.CallCount,
		e.InputTokens, e.OutputTokens, e.CachedInputTokens, e.ReasoningTokens, e.EmbeddingTokens, e.TotalTokens,
		e.InputPriceMicroUSDPer1K, e.OutputPriceMicroUSDPer1K, e.CachedInputPriceMicroUSDPer1K, e.ReasoningPriceMicroUSDPer1K, e.EmbeddingPriceMicroUSDPer1K,
		e.InputCostMicroUSD, e.OutputCostMicroUSD, e.CachedInputCostMicroUSD, e.ReasoningCostMicroUSD, e.EmbeddingCostMicroUSD, e.TotalCostMicroUSD,
		e.LatencyMS, e.TimeToFirstTokenMS, e.TokensPerSecond, e.Status, e.ErrorCode, e.ErrorMessage, e.RetryCount,
		e.PromptMode, e.MaxOutputTokens, e.ContextWindowK, streamEnabled, e.MetadataJSON, e.CreatedAt,
	)
	if err != nil {
		return biz.TokenUsageEvent{}, err
	}

	if strings.TrimSpace(e.SessionID) != "" {
		_, err = c.ExecContext(ctx,
			`UPDATE sessions
			 SET model_call_count = model_call_count + ?,
			     input_tokens = input_tokens + ?,
			     output_tokens = output_tokens + ?,
			     total_tokens = total_tokens + ?,
			     total_cost_micro_usd = total_cost_micro_usd + ?,
			     provider = ?,
			     model = ?,
			     updated_at = ?
			 WHERE id = ? AND deleted_at = ''`,
			e.CallCount, e.InputTokens, e.OutputTokens, e.TotalTokens, e.TotalCostMicroUSD,
			e.ProviderCode, e.ModelAPIID, nowRFC3339(), e.SessionID,
		)
		if err != nil {
			return biz.TokenUsageEvent{}, err
		}
	}

	if err = upsertModelTokenUsageDaily(ctx, c, e); err != nil {
		return biz.TokenUsageEvent{}, err
	}

	if err := tx.Commit(); err != nil {
		return biz.TokenUsageEvent{}, err
	}
	return e, nil
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (stdsql.Result, error)
}

func upsertModelTokenUsageDaily(ctx context.Context, c execer, e biz.TokenUsageEvent) error {
	if strings.TrimSpace(e.DateKey) == "" {
		return nil
	}
	successCount := 0
	failedCount := 0
	cancelledCount := 0
	switch e.Status {
	case "success":
		successCount = 1
	case "cancelled":
		cancelledCount = 1
	default:
		failedCount = 1
	}
	id := strings.Join([]string{e.DateKey, e.WorkspaceID, e.AgentID, e.ProviderCode, e.ModelAPIID, e.UsageKind}, ":")
	now := nowRFC3339()
	_, err := c.ExecContext(ctx,
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
		id, e.DateKey, e.WorkspaceID, e.AgentID, e.AgentKey, e.ProviderCode, e.ModelAPIID, e.UsageKind,
		e.CallCount, successCount, failedCount, cancelledCount,
		e.InputTokens, e.OutputTokens, e.CachedInputTokens, e.ReasoningTokens, e.EmbeddingTokens, e.TotalTokens,
		e.TotalCostMicroUSD, float64(e.LatencyMS), e.TokensPerSecond, now, now,
	)
	return err
}
