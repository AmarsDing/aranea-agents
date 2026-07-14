package service

import (
	v1 "aranea-agents/api/kratos/usage/v1"
	"aranea-agents/internal/biz"
)

func fromProtoUsageQuery(in *v1.UsageQuery) biz.UsageQuery {
	if in == nil {
		return biz.UsageQuery{}
	}
	return biz.UsageQuery{
		Range:        in.GetRange(),
		StartDate:    in.GetStartDate(),
		EndDate:      in.GetEndDate(),
		ProviderCode: in.GetProviderCode(),
		ModelAPIID:   in.GetModelApiId(),
		AgentID:      in.GetAgentId(),
		TeamID:       in.GetTeamId(),
		UsageKind:    in.GetUsageKind(),
		Status:       in.GetStatus(),
		Limit:        int(in.GetLimit()),
		Granularity:  in.GetGranularity(),
	}
}

// fromProtoBreakdownQuery converts the proto ListAllModelsBreakdown request to the biz query.
// Range/StartDate/EndDate are passed through; biz.Usecase.AllModelsBreakdown resolves the
// date range if StartDate/EndDate are empty (via normalizeBreakdownQuery).
func fromProtoBreakdownQuery(in *v1.ListAllModelsBreakdownRequest) biz.UsageBreakdownQuery {
	if in == nil {
		return biz.UsageBreakdownQuery{}
	}
	return biz.UsageBreakdownQuery{
		Range:        in.GetRange(),
		StartDate:    in.GetStartDate(),
		EndDate:      in.GetEndDate(),
		ProviderCode: in.GetProviderCode(),
		Search:       in.GetSearch(),
		SortField:    in.GetSortField(),
		SortDir:      in.GetSortDir(),
		Page:         in.GetPage(),
		PageSize:     in.GetPageSize(),
	}
}

func fromProtoTokenUsageEvent(p *v1.TokenUsageEvent) biz.TokenUsageEvent {
	if p == nil {
		return biz.TokenUsageEvent{}
	}
	return biz.TokenUsageEvent{
		ID:                            p.GetId(),
		OccurredAt:                    p.GetOccurredAt(),
		DateKey:                       p.GetDateKey(),
		HourKey:                       p.GetHourKey(),
		WorkspaceID:                   p.GetWorkspaceId(),
		UserID:                        p.GetUserId(),
		TeamID:                        p.GetTeamId(),
		AgentID:                       p.GetAgentId(),
		AgentKey:                      p.GetAgentKey(),
		SessionID:                     p.GetSessionId(),
		MessageID:                     p.GetMessageId(),
		RequestID:                     p.GetRequestId(),
		ProviderCode:                  p.GetProviderCode(),
		ProviderType:                  p.GetProviderType(),
		ProviderDisplayName:           p.GetProviderDisplayName(),
		ModelAPIID:                    p.GetModelApiId(),
		ModelDisplayName:              p.GetModelDisplayName(),
		ModelCategoryJSON:             p.GetModelCategoryJson(),
		UsageKind:                     p.GetUsageKind(),
		CallCount:                     int(p.GetCallCount()),
		InputTokens:                   int(p.GetInputTokens()),
		OutputTokens:                  int(p.GetOutputTokens()),
		CachedInputTokens:             int(p.GetCachedInputTokens()),
		ReasoningTokens:               int(p.GetReasoningTokens()),
		EmbeddingTokens:               int(p.GetEmbeddingTokens()),
		TotalTokens:                   int(p.GetTotalTokens()),
		InputPriceMicroUSDPer1K:       p.GetInputPriceMicroUsdPer_1K(),
		OutputPriceMicroUSDPer1K:      p.GetOutputPriceMicroUsdPer_1K(),
		CachedInputPriceMicroUSDPer1K: p.GetCachedInputPriceMicroUsdPer_1K(),
		ReasoningPriceMicroUSDPer1K:   p.GetReasoningPriceMicroUsdPer_1K(),
		EmbeddingPriceMicroUSDPer1K:   p.GetEmbeddingPriceMicroUsdPer_1K(),
		InputCostMicroUSD:             p.GetInputCostMicroUsd(),
		OutputCostMicroUSD:            p.GetOutputCostMicroUsd(),
		CachedInputCostMicroUSD:       p.GetCachedInputCostMicroUsd(),
		ReasoningCostMicroUSD:         p.GetReasoningCostMicroUsd(),
		EmbeddingCostMicroUSD:         p.GetEmbeddingCostMicroUsd(),
		TotalCostMicroUSD:             p.GetTotalCostMicroUsd(),
		LatencyMS:                     int(p.GetLatencyMs()),
		TimeToFirstTokenMS:            int(p.GetTimeToFirstTokenMs()),
		TokensPerSecond:               p.GetTokensPerSecond(),
		Status:                        p.GetStatus(),
		ErrorCode:                     p.GetErrorCode(),
		ErrorMessage:                  p.GetErrorMessage(),
		RetryCount:                    int(p.GetRetryCount()),
		PromptMode:                    p.GetPromptMode(),
		MaxOutputTokens:               int(p.GetMaxOutputTokens()),
		ContextWindowK:                int(p.GetContextWindowK()),
		StreamEnabled:                 p.GetStreamEnabled(),
		MetadataJSON:                  p.GetMetadataJson(),
		CreatedAt:                     p.GetCreatedAt(),
	}
}

func toProtoUsageQuota(q biz.UsageQuota) *v1.UsageQuota {
	return &v1.UsageQuota{
		Id:              q.ID,
		ScopeType:       q.ScopeType,
		ScopeId:         q.ScopeID,
		MonthlyMicroUsd: q.MonthlyMicroUSD,
		PeriodStart:     q.PeriodStart,
		PeriodEnd:       q.PeriodEnd,
		CreatedAt:       q.CreatedAt,
		UpdatedAt:       q.UpdatedAt,
	}
}

func toProtoBudgetAlert(a biz.BudgetAlert) *v1.BudgetAlert {
	return &v1.BudgetAlert{
		Id:          a.ID,
		ScopeType:   a.ScopeType,
		ScopeId:     a.ScopeID,
		AlertRatio:  a.AlertRatio,
		Enabled:     a.Enabled,
		LastFiredAt: a.LastFiredAt,
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
	}
}

func toProtoUsageSummary(s biz.UsageSummary) *v1.UsageSummary {
	return &v1.UsageSummary{
		CallCount:          int32(s.CallCount),
		RequestCount:       int32(s.RequestCount),
		SuccessCount:       int32(s.SuccessCount),
		FailedCount:        int32(s.FailedCount),
		CancelledCount:     int32(s.CancelledCount),
		InputTokens:        int32(s.InputTokens),
		OutputTokens:       int32(s.OutputTokens),
		TotalTokens:        int32(s.TotalTokens),
		TotalCostMicroUsd:  s.TotalCostMicroUSD,
		AvgLatencyMs:       s.AvgLatencyMS,
		AvgTokensPerSecond: s.AvgTokensPerSecond,
		SuccessRate:        s.SuccessRate,
	}
}

func toProtoUsageTrendPoint(p biz.UsageTrendPoint) *v1.UsageTrendPoint {
	return &v1.UsageTrendPoint{
		DateKey:            p.DateKey,
		CallCount:          int32(p.CallCount),
		InputTokens:        int32(p.InputTokens),
		OutputTokens:       int32(p.OutputTokens),
		TotalTokens:        int32(p.TotalTokens),
		TotalCostMicroUsd:  p.TotalCostMicroUSD,
		SuccessCount:       int32(p.SuccessCount),
		FailedCount:        int32(p.FailedCount),
		CancelledCount:     int32(p.CancelledCount),
		AvgLatencyMs:       p.AvgLatencyMS,
		AvgTokensPerSecond: p.AvgTokensPerSecond,
	}
}

func toProtoUsageBreakdownRow(r biz.UsageBreakdownRow) *v1.UsageBreakdownRow {
	return &v1.UsageBreakdownRow{
		ProviderCode:       r.ProviderCode,
		ModelApiId:         r.ModelAPIID,
		ModelDisplayName:   r.ModelDisplayName,
		AgentId:            r.AgentID,
		AgentKey:           r.AgentKey,
		CallCount:          int32(r.CallCount),
		InputTokens:        int32(r.InputTokens),
		OutputTokens:       int32(r.OutputTokens),
		TotalTokens:        int32(r.TotalTokens),
		TotalCostMicroUsd:  r.TotalCostMicroUSD,
		AvgLatencyMs:       r.AvgLatencyMS,
		AvgTokensPerSecond: r.AvgTokensPerSecond,
		SuccessRate:        r.SuccessRate,
	}
}

func toProtoTokenUsageEvent(e biz.TokenUsageEvent) *v1.TokenUsageEvent {
	return &v1.TokenUsageEvent{
		Id:                             e.ID,
		OccurredAt:                     e.OccurredAt,
		DateKey:                        e.DateKey,
		HourKey:                        e.HourKey,
		WorkspaceId:                    e.WorkspaceID,
		UserId:                         e.UserID,
		TeamId:                         e.TeamID,
		AgentId:                        e.AgentID,
		AgentKey:                       e.AgentKey,
		SessionId:                      e.SessionID,
		MessageId:                      e.MessageID,
		RequestId:                      e.RequestID,
		ProviderCode:                   e.ProviderCode,
		ProviderType:                   e.ProviderType,
		ProviderDisplayName:            e.ProviderDisplayName,
		ModelApiId:                     e.ModelAPIID,
		ModelDisplayName:               e.ModelDisplayName,
		ModelCategoryJson:              e.ModelCategoryJSON,
		UsageKind:                      e.UsageKind,
		CallCount:                      int32(e.CallCount),
		InputTokens:                    int32(e.InputTokens),
		OutputTokens:                   int32(e.OutputTokens),
		CachedInputTokens:              int32(e.CachedInputTokens),
		ReasoningTokens:                int32(e.ReasoningTokens),
		EmbeddingTokens:                int32(e.EmbeddingTokens),
		TotalTokens:                    int32(e.TotalTokens),
		InputPriceMicroUsdPer_1K:       e.InputPriceMicroUSDPer1K,
		OutputPriceMicroUsdPer_1K:      e.OutputPriceMicroUSDPer1K,
		CachedInputPriceMicroUsdPer_1K: e.CachedInputPriceMicroUSDPer1K,
		ReasoningPriceMicroUsdPer_1K:   e.ReasoningPriceMicroUSDPer1K,
		EmbeddingPriceMicroUsdPer_1K:   e.EmbeddingPriceMicroUSDPer1K,
		InputCostMicroUsd:              e.InputCostMicroUSD,
		OutputCostMicroUsd:             e.OutputCostMicroUSD,
		CachedInputCostMicroUsd:        e.CachedInputCostMicroUSD,
		ReasoningCostMicroUsd:          e.ReasoningCostMicroUSD,
		EmbeddingCostMicroUsd:          e.EmbeddingCostMicroUSD,
		TotalCostMicroUsd:              e.TotalCostMicroUSD,
		LatencyMs:                      int32(e.LatencyMS),
		TimeToFirstTokenMs:             int32(e.TimeToFirstTokenMS),
		TokensPerSecond:                e.TokensPerSecond,
		Status:                         e.Status,
		ErrorCode:                      e.ErrorCode,
		ErrorMessage:                   e.ErrorMessage,
		RetryCount:                     int32(e.RetryCount),
		PromptMode:                     e.PromptMode,
		MaxOutputTokens:                int32(e.MaxOutputTokens),
		ContextWindowK:                 int32(e.ContextWindowK),
		StreamEnabled:                  e.StreamEnabled,
		MetadataJson:                   e.MetadataJSON,
		CreatedAt:                      e.CreatedAt,
	}
}

func toProtoQuotaDashboard(d biz.QuotaDashboard) *v1.UsageQuotaDashboard {
	return &v1.UsageQuotaDashboard{
		ConfiguredCount:     int32(d.ConfiguredCount),
		TotalCapMicroUsd:    d.TotalCapMicroUSD,
		TotalSpentMicroUsd:  d.TotalSpentMicroUSD,
		MaxUtilizationRatio: d.MaxUtilization,
	}
}

func toProtoUsageModelInsights(items []biz.UsageModelInsight) []*v1.UsageModelInsight {
	out := make([]*v1.UsageModelInsight, 0, len(items))
	for _, m := range items {
		out = append(out, &v1.UsageModelInsight{
			ProviderCode:       m.ProviderCode,
			ModelApiId:         m.ModelAPIID,
			ModelDisplayName:   m.ModelDisplayName,
			CallCount:          int32(m.CallCount),
			TotalTokens:        int32(m.TotalTokens),
			TotalCostMicroUsd:  m.TotalCostMicroUSD,
			AvgLatencyMs:       m.AvgLatencyMS,
			AvgTokensPerSecond: m.AvgTokensPerSecond,
			SuccessRate:        m.SuccessRate,
			Flags:              m.Flags,
		})
	}
	return out
}

func toProtoUsageTrendPoints(items []biz.UsageTrendPoint) []*v1.UsageTrendPoint {
	out := make([]*v1.UsageTrendPoint, 0, len(items))
	for _, p := range items {
		out = append(out, toProtoUsageTrendPoint(p))
	}
	return out
}

func toProtoUsageBreakdownRows(rows []biz.UsageBreakdownRow) []*v1.UsageBreakdownRow {
	out := make([]*v1.UsageBreakdownRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, toProtoUsageBreakdownRow(r))
	}
	return out
}

func toProtoTokenUsageEvents(events []biz.TokenUsageEvent) []*v1.TokenUsageEvent {
	out := make([]*v1.TokenUsageEvent, 0, len(events))
	for _, e := range events {
		out = append(out, toProtoTokenUsageEvent(e))
	}
	return out
}

func toProtoBudgetAlerts(items []biz.BudgetAlert) []*v1.BudgetAlert {
	out := make([]*v1.BudgetAlert, 0, len(items))
	for _, a := range items {
		out = append(out, toProtoBudgetAlert(a))
	}
	return out
}
