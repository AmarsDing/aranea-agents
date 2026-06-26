package biz

import (
	"context"
	"time"

	"aranea-agents/internal/event/contract"
)

// usageRollupConsumer rolls up token usage statistics from ActivityEvents.
//
// Phase 5 Blocker B: migrated from legacy Envelope-based SessionBus to
// ActivityEventBus. The token_usage publisher (biz.PublishTokenUsageEnvelope)
// emits ActivityEvent{Stage:"token_usage", Meta:{"token_usage":<EnvelopeTokenUsage>}}.
// This consumer filters at the bus level by Stage=="token_usage" and extracts
// the EnvelopeTokenUsage from Meta.
type usageRollupConsumer struct {
	bus    ActivityEventBus
	usage  *UsageUsecase
	logger SessionLogWriter
}

func newUsageRollupConsumer(bus ActivityEventBus, usage *UsageUsecase, logger SessionLogWriter) *usageRollupConsumer {
	if usage == nil || bus == nil {
		return nil
	}
	return &usageRollupConsumer{bus: bus, usage: usage, logger: logger}
}

func (c *usageRollupConsumer) Start(ctx context.Context) {
	if c == nil {
		return
	}
	runActivityConsumerWithOpts(ctx, "event-bus-usage-rollup", c.bus, ActivityEventSubscribeOptions{
		BufferSize: 256,
		GlobalMode: true,
		Filter: func(ev ActivityEvent) bool {
			return ev.Activity.Stage == "token_usage"
		},
	}, c.handle, offerOption[ActivityEvent]{FallbackSync: true, FallbackFn: c.handle}, c.logger)
}

func (c *usageRollupConsumer) handle(ctx context.Context, ev ActivityEvent) {
	if c == nil || c.usage == nil {
		return
	}
	tu, ok := ev.Activity.Meta["token_usage"]
	if !ok || tu == nil {
		return
	}
	envTU, ok := tu.(contract.EnvelopeTokenUsage)
	if !ok {
		return
	}
	e := tokenUsageFromEnvelope(&envTU)
	if err := c.usage.RollupDailyHourly(ctx, e); err != nil && c.logger != nil {
		c.logger.LogSessionWarn(ctx, ev.Activity.SessionID, "usage.rollup_failed", "用量汇总写入失败",
			LogPair{Key: "error", Value: err})
	}
}

func tokenUsageFromEnvelope(tu *contract.EnvelopeTokenUsage) TokenUsageEvent {
	return TokenUsageEvent{
		ID:                            tu.ID,
		OccurredAt:                    tu.OccurredAt,
		DateKey:                       tu.DateKey,
		HourKey:                       tu.HourKey,
		WorkspaceID:                   tu.WorkspaceID,
		UserID:                        tu.UserID,
		TeamID:                        tu.TeamID,
		AgentID:                       tu.AgentID,
		AgentKey:                      tu.AgentKey,
		SessionID:                     tu.SessionID,
		MessageID:                     tu.MessageID,
		RequestID:                     tu.RequestID,
		ProviderCode:                  tu.ProviderCode,
		CanonicalProviderCode:         tu.CanonicalProviderCode,
		ProviderType:                  tu.ProviderType,
		ProviderDisplayName:           tu.ProviderDisplayName,
		ModelAPIID:                    tu.ModelAPIID,
		ModelDisplayName:              tu.ModelDisplayName,
		ModelCategoryJSON:             tu.ModelCategoryJSON,
		UsageKind:                     tu.UsageKind,
		CallCount:                     tu.CallCount,
		InputTokens:                   tu.InputTokens,
		OutputTokens:                  tu.OutputTokens,
		CachedInputTokens:             tu.CachedInputTokens,
		CacheWriteTokens:              tu.CacheWriteTokens,
		ReasoningTokens:               tu.ReasoningTokens,
		EmbeddingTokens:               tu.EmbeddingTokens,
		TotalTokens:                   tu.TotalTokens,
		InputPriceMicroUSDPer1K:       tu.InputPriceMicroUSDPer1K,
		OutputPriceMicroUSDPer1K:      tu.OutputPriceMicroUSDPer1K,
		CachedInputPriceMicroUSDPer1K: tu.CachedInputPriceMicroUSDPer1K,
		CacheWritePriceMicroUSDPer1K:  tu.CacheWritePriceMicroUSDPer1K,
		ReasoningPriceMicroUSDPer1K:   tu.ReasoningPriceMicroUSDPer1K,
		EmbeddingPriceMicroUSDPer1K:   tu.EmbeddingPriceMicroUSDPer1K,
		InputCostMicroUSD:             tu.InputCostMicroUSD,
		OutputCostMicroUSD:            tu.OutputCostMicroUSD,
		CachedInputCostMicroUSD:       tu.CachedInputCostMicroUSD,
		CacheWriteCostMicroUSD:        tu.CacheWriteCostMicroUSD,
		ReasoningCostMicroUSD:         tu.ReasoningCostMicroUSD,
		EmbeddingCostMicroUSD:         tu.EmbeddingCostMicroUSD,
		TotalCostMicroUSD:             tu.TotalCostMicroUSD,
		LatencyMS:                     tu.LatencyMS,
		TimeToFirstTokenMS:            tu.TimeToFirstTokenMS,
		TokensPerSecond:               tu.TokensPerSecond,
		Status:                        tu.Status,
		ErrorCode:                     tu.ErrorCode,
		ErrorMessage:                  tu.ErrorMessage,
		RetryCount:                    tu.RetryCount,
		PromptMode:                    tu.PromptMode,
		MaxOutputTokens:               tu.MaxOutputTokens,
		ContextWindowK:                tu.ContextWindowK,
		StreamEnabled:                 tu.StreamEnabled,
		MetadataJSON:                  tu.MetadataJSON,
		CreatedAt:                     tu.CreatedAt,
	}
}

func TokenUsageEventToEnvelope(e TokenUsageEvent) contract.EnvelopeTokenUsage {
	return contract.EnvelopeTokenUsage{
		ID:                            e.ID,
		OccurredAt:                    e.OccurredAt,
		DateKey:                       e.DateKey,
		HourKey:                       e.HourKey,
		WorkspaceID:                   e.WorkspaceID,
		UserID:                        e.UserID,
		TeamID:                        e.TeamID,
		AgentID:                       e.AgentID,
		AgentKey:                      e.AgentKey,
		SessionID:                     e.SessionID,
		MessageID:                     e.MessageID,
		RequestID:                     e.RequestID,
		ProviderCode:                  e.ProviderCode,
		CanonicalProviderCode:         e.CanonicalProviderCode,
		ProviderType:                  e.ProviderType,
		ProviderDisplayName:           e.ProviderDisplayName,
		ModelAPIID:                    e.ModelAPIID,
		ModelDisplayName:              e.ModelDisplayName,
		ModelCategoryJSON:             e.ModelCategoryJSON,
		UsageKind:                     e.UsageKind,
		CallCount:                     e.CallCount,
		InputTokens:                   e.InputTokens,
		OutputTokens:                  e.OutputTokens,
		CachedInputTokens:             e.CachedInputTokens,
		CacheWriteTokens:              e.CacheWriteTokens,
		ReasoningTokens:               e.ReasoningTokens,
		EmbeddingTokens:               e.EmbeddingTokens,
		TotalTokens:                   e.TotalTokens,
		InputPriceMicroUSDPer1K:       e.InputPriceMicroUSDPer1K,
		OutputPriceMicroUSDPer1K:      e.OutputPriceMicroUSDPer1K,
		CachedInputPriceMicroUSDPer1K: e.CachedInputPriceMicroUSDPer1K,
		CacheWritePriceMicroUSDPer1K:  e.CacheWritePriceMicroUSDPer1K,
		ReasoningPriceMicroUSDPer1K:   e.ReasoningPriceMicroUSDPer1K,
		EmbeddingPriceMicroUSDPer1K:   e.EmbeddingPriceMicroUSDPer1K,
		InputCostMicroUSD:             e.InputCostMicroUSD,
		OutputCostMicroUSD:            e.OutputCostMicroUSD,
		CachedInputCostMicroUSD:       e.CachedInputCostMicroUSD,
		CacheWriteCostMicroUSD:        e.CacheWriteCostMicroUSD,
		ReasoningCostMicroUSD:         e.ReasoningCostMicroUSD,
		EmbeddingCostMicroUSD:         e.EmbeddingCostMicroUSD,
		TotalCostMicroUSD:             e.TotalCostMicroUSD,
		LatencyMS:                     e.LatencyMS,
		TimeToFirstTokenMS:            e.TimeToFirstTokenMS,
		TokensPerSecond:               e.TokensPerSecond,
		Status:                        e.Status,
		ErrorCode:                     e.ErrorCode,
		ErrorMessage:                  e.ErrorMessage,
		RetryCount:                    e.RetryCount,
		PromptMode:                    e.PromptMode,
		MaxOutputTokens:               e.MaxOutputTokens,
		ContextWindowK:                e.ContextWindowK,
		StreamEnabled:                 e.StreamEnabled,
		MetadataJSON:                  e.MetadataJSON,
		CreatedAt:                     e.CreatedAt,
	}
}

func PublishTokenUsageEnvelope(ctx context.Context, bus ActivityEventBus, e TokenUsageEvent) {
	if bus == nil {
		return
	}
	tu := TokenUsageEventToEnvelope(e)
	ev := ActivityEvent{
		Event: ActivityEventUpdated,
		Activity: Activity{
			ID:        e.ID,
			Kind:      ActivityKindNotice,
			Status:    ActivityStatusCompleted,
			SessionID: e.SessionID,
			TeamID:    e.TeamID,
			AgentKey:  e.AgentKey,
			Timestamp: time.Now().UTC(),
			Stage:     "token_usage",
			Meta:      map[string]any{"token_usage": tu},
		},
		Domain: ActivityDomainSystem,
	}
	bus.Publish(ctx, ev)
}
