package biz

import (
	"context"

	"aranea-agents/internal/event/contract"
)

type usageRollupConsumer struct {
	bus    contract.Bus
	usage  *UsageUsecase
	logger SessionLogWriter
}

func newUsageRollupConsumer(bus contract.Bus, usage *UsageUsecase, logger SessionLogWriter) *usageRollupConsumer {
	if usage == nil || bus == nil {
		return nil
	}
	return &usageRollupConsumer{bus: bus, usage: usage, logger: logger}
}

func (c *usageRollupConsumer) Start(ctx context.Context) {
	if c == nil {
		return
	}
	runTypedConsumer(ctx, "event-bus-usage-rollup", c.bus, contract.SubscribeOptions{
		EventTypes: []contract.EnvelopeType{contract.EnvelopeTypeTokenUsage},
		BufferSize: 256,
	}, c.handle, c.logger)
}

func (c *usageRollupConsumer) handle(ctx context.Context, env contract.Envelope) {
	if c == nil || c.usage == nil || env.TokenUsage == nil {
		return
	}
	e := tokenUsageFromEnvelope(env.TokenUsage)
	if err := c.usage.RollupDailyHourly(ctx, e); err != nil && c.logger != nil {
		c.logger.LogSessionWarn(ctx, env.SessionID, "usage.rollup_failed", "用量汇总写入失败",
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

func PublishTokenUsageEnvelope(ctx context.Context, bus contract.Bus, e TokenUsageEvent) {
	if bus == nil {
		return
	}
	tu := TokenUsageEventToEnvelope(e)
	env := contract.NewEnvelope(contract.EnvelopeTypeTokenUsage, "usage", e.SessionID)
	env.TokenUsage = &tu
	bus.Publish(ctx, env)
}
