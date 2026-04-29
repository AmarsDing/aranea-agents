package service

import (
	"context"

	v1 "aranea-agents/api/kratos/usage/v1"
	"aranea-agents/internal/biz"
)

// UsageService implements kratos usage.v1.
type UsageService struct {
	v1.UnimplementedUsageServiceServer

	uc *biz.UsageUsecase
}

func NewUsageService(uc *biz.UsageUsecase) *UsageService {
	return &UsageService{uc: uc}
}

func protoUsageQuery(in *v1.UsageQuery) biz.UsageQuery {
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
		Status:       in.GetStatus(),
		Limit:        int(in.GetLimit()),
	}
}

func bizUsageSummaryToProto(s biz.UsageSummary) *v1.UsageSummary {
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

func bizUsageTrendPointToProto(p biz.UsageTrendPoint) *v1.UsageTrendPoint {
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

func bizBreakdownRowToProto(r biz.UsageBreakdownRow) *v1.UsageBreakdownRow {
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

func bizTokenUsageEventToProto(e biz.TokenUsageEvent) *v1.TokenUsageEvent {
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

func mapTrendPoints(items []biz.UsageTrendPoint) []*v1.UsageTrendPoint {
	out := make([]*v1.UsageTrendPoint, 0, len(items))
	for _, p := range items {
		out = append(out, bizUsageTrendPointToProto(p))
	}
	return out
}

func mapBreakdownRows(rows []biz.UsageBreakdownRow) []*v1.UsageBreakdownRow {
	out := make([]*v1.UsageBreakdownRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, bizBreakdownRowToProto(r))
	}
	return out
}

func mapTokenEvents(events []biz.TokenUsageEvent) []*v1.TokenUsageEvent {
	out := make([]*v1.TokenUsageEvent, 0, len(events))
	for _, e := range events {
		out = append(out, bizTokenUsageEventToProto(e))
	}
	return out
}

func (s *UsageService) GetUsageOverview(ctx context.Context, in *v1.UsageQuery) (*v1.UsageOverview, error) {
	o, err := s.uc.Overview(ctx, protoUsageQuery(in))
	if err != nil {
		return nil, err
	}
	return &v1.UsageOverview{
		Today:        bizUsageSummaryToProto(o.Today),
		Yesterday:    bizUsageSummaryToProto(o.Yesterday),
		Month:        bizUsageSummaryToProto(o.Month),
		RangeSummary: bizUsageSummaryToProto(o.Range),
		Trends:       mapTrendPoints(o.Trends),
		TopModels:    mapBreakdownRows(o.TopModels),
		TopAgents:    mapBreakdownRows(o.TopAgents),
		Anomalies:    mapTokenEvents(o.Anomalies),
	}, nil
}

func (s *UsageService) ListUsageTrends(ctx context.Context, in *v1.UsageQuery) (*v1.ListUsageTrendsResponse, error) {
	items, err := s.uc.Trends(ctx, protoUsageQuery(in))
	if err != nil {
		return nil, err
	}
	return &v1.ListUsageTrendsResponse{Items: mapTrendPoints(items)}, nil
}

func (s *UsageService) ListTopModels(ctx context.Context, in *v1.UsageQuery) (*v1.ListBreakdownResponse, error) {
	items, err := s.uc.TopModels(ctx, protoUsageQuery(in))
	if err != nil {
		return nil, err
	}
	return &v1.ListBreakdownResponse{Items: mapBreakdownRows(items)}, nil
}

func (s *UsageService) ListTopAgents(ctx context.Context, in *v1.UsageQuery) (*v1.ListBreakdownResponse, error) {
	items, err := s.uc.TopAgents(ctx, protoUsageQuery(in))
	if err != nil {
		return nil, err
	}
	return &v1.ListBreakdownResponse{Items: mapBreakdownRows(items)}, nil
}

func (s *UsageService) ListUsageEvents(ctx context.Context, in *v1.UsageQuery) (*v1.ListUsageEventsResponse, error) {
	items, err := s.uc.Events(ctx, protoUsageQuery(in))
	if err != nil {
		return nil, err
	}
	return &v1.ListUsageEventsResponse{Items: mapTokenEvents(items)}, nil
}
