package repository

import (
	"database/sql"

	"arenea/backend/internal/domain"
)

type sqlExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func (r *SQLiteRepository) updateSessionModelUsage(event domain.ModelTokenUsageEvent) error {
	return updateSessionModelUsageTx(r.db, event)
}

func updateSessionModelUsageTx(exec sqlExecer, event domain.ModelTokenUsageEvent) error {
	if event.SessionID == "" {
		return nil
	}
	_, err := exec.Exec(
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
		event.CallCount, event.InputTokens, event.OutputTokens, event.TotalTokens, event.TotalCostMicroUSD,
		event.ProviderCode, event.ModelAPIID, nowISO(), event.SessionID,
	)
	return err
}

func addModelTokenUsageEventTx(exec sqlExecer, event domain.ModelTokenUsageEvent, streamEnabled int) error {
	_, err := exec.Exec(
		`INSERT INTO model_token_usage_events(
		 id, occurred_at, date_key, hour_key, workspace_id, user_id, team_id, agent_id, agent_key, session_id, message_id, request_id,
		 provider_code, provider_type, provider_display_name, model_api_id, model_display_name, model_category_json, usage_kind, call_count,
		 input_tokens, output_tokens, cached_input_tokens, reasoning_tokens, embedding_tokens, total_tokens,
		 input_price_micro_usd_per_1k, output_price_micro_usd_per_1k, cached_input_price_micro_usd_per_1k, reasoning_price_micro_usd_per_1k, embedding_price_micro_usd_per_1k,
		 input_cost_micro_usd, output_cost_micro_usd, cached_input_cost_micro_usd, reasoning_cost_micro_usd, embedding_cost_micro_usd, total_cost_micro_usd,
		 latency_ms, time_to_first_token_ms, tokens_per_second, status, error_code, error_message, retry_count,
		 prompt_mode, max_output_tokens, context_window_k, stream_enabled, metadata_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.ID, event.OccurredAt, event.DateKey, event.HourKey, event.WorkspaceID, event.UserID, event.TeamID, event.AgentID, event.AgentKey, event.SessionID, event.MessageID, event.RequestID,
		event.ProviderCode, event.ProviderType, event.ProviderDisplayName, event.ModelAPIID, event.ModelDisplayName, event.ModelCategoryJSON, event.UsageKind, event.CallCount,
		event.InputTokens, event.OutputTokens, event.CachedInputTokens, event.ReasoningTokens, event.EmbeddingTokens, event.TotalTokens,
		event.InputPriceMicroUSDPer1K, event.OutputPriceMicroUSDPer1K, event.CachedInputPriceMicroUSDPer1K, event.ReasoningPriceMicroUSDPer1K, event.EmbeddingPriceMicroUSDPer1K,
		event.InputCostMicroUSD, event.OutputCostMicroUSD, event.CachedInputCostMicroUSD, event.ReasoningCostMicroUSD, event.EmbeddingCostMicroUSD, event.TotalCostMicroUSD,
		event.LatencyMS, event.TimeToFirstTokenMS, event.TokensPerSecond, event.Status, event.ErrorCode, event.ErrorMessage, event.RetryCount,
		event.PromptMode, event.MaxOutputTokens, event.ContextWindowK, streamEnabled, event.MetadataJSON, event.CreatedAt,
	)
	return err
}
