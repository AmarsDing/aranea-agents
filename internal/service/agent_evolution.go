package service

import (
	"context"
	"strings"

	v1 "aranea-agents/api/kratos/agent/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

func toProtoSuggestion(s biz.EvolutionSuggestion) *v1.EvolutionSuggestion {
	return &v1.EvolutionSuggestion{
		Id:          s.ID,
		AgentId:     s.AgentID,
		Type:        s.Type,
		Title:       s.Title,
		Content:     s.Content,
		Status:      s.Status,
		DiffPreview: s.DiffPreview,
		CreatedAt:   s.CreatedAt,
		AppliedAt:   s.AppliedAt,
		Applicable:  s.Applicable(),
	}
}

func (s *AgentService) GetAgentEvolutionMetrics(ctx context.Context, req *v1.GetAgentEvolutionMetricsRequest) (*v1.EvolutionMetricsResponse, error) {
	if strings.TrimSpace(req.GetAgentId()) == "" {
		return nil, apierror.BadRequest(apierror.DomainAgent, "agent_id is required")
	}
	if err := s.assertAgentAccess(ctx, req.GetAgentId()); err != nil {
		return nil, err
	}
	m, err := s.evoUC.GetEvolutionMetrics(ctx, req.GetAgentId(), req.GetTimeRange())
	if err != nil {
		return nil, err
	}
	resp := &v1.EvolutionMetricsResponse{
		AgentId:          m.AgentID,
		TimeRange:        m.TimeRange,
		ToolSuccessRate:  m.ToolSuccessRate,
		RetrievalQuality: m.RetrievalQuality,
		TotalEpisodes:    int32(m.TotalEpisodes),
		NegativeFeedback: int32(m.NegativeFeedback),
		// S-05/S-08: surface partial-failure state so callers can distinguish
		// "no data" from "some sub-queries failed".
		Partial:       m.Partial,
		PartialErrors: m.PartialErrors,
	}
	for _, p := range m.ToolSuccessSeries {
		resp.ToolSuccessSeries = append(resp.ToolSuccessSeries, &v1.MetricDataPoint{Date: p.Date, Value: p.Value})
	}
	for _, p := range m.RetrievalQualitySeries {
		resp.RetrievalQualitySeries = append(resp.RetrievalQualitySeries, &v1.MetricDataPoint{Date: p.Date, Value: p.Value})
	}
	return resp, nil
}

func (s *AgentService) GetAgentEvolutionSuggestions(ctx context.Context, req *v1.GetAgentEvolutionSuggestionsRequest) (*v1.ListEvolutionSuggestionsResponse, error) {
	if err := s.assertAgentAccess(ctx, req.GetAgentId()); err != nil {
		return nil, err
	}
	items, err := s.evoUC.GetEvolutionSuggestions(ctx, req.GetAgentId(), req.GetStatus())
	if err != nil {
		return nil, err
	}
	resp := &v1.ListEvolutionSuggestionsResponse{}
	for _, item := range items {
		resp.Items = append(resp.Items, toProtoSuggestion(item))
	}
	return resp, nil
}

func (s *AgentService) ApplyEvolutionSuggestion(ctx context.Context, req *v1.ApplyEvolutionSuggestionRequest) (*v1.EvolutionSuggestion, error) {
	if err := s.assertAgentMutateAccess(ctx, req.GetAgentId()); err != nil {
		return nil, err
	}
	result, err := s.evoUC.ApplySuggestion(ctx, req.GetAgentId(), req.GetSuggestionId())
	if err != nil {
		return nil, err
	}
	// ApplySuggestion modifies agent prompt files or settings, so the cached
	// build must be invalidated to avoid serving stale agent instances.
	invalidateAgentBuildCache(req.GetAgentId())
	return toProtoSuggestion(result), nil
}

func (s *AgentService) RejectEvolutionSuggestion(ctx context.Context, req *v1.RejectEvolutionSuggestionRequest) (*v1.EvolutionSuggestion, error) {
	if err := s.assertAgentMutateAccess(ctx, req.GetAgentId()); err != nil {
		return nil, err
	}
	result, err := s.evoUC.RejectSuggestion(ctx, req.GetAgentId(), req.GetSuggestionId(), req.GetReason())
	if err != nil {
		return nil, err
	}
	return toProtoSuggestion(result), nil
}

func (s *AgentService) RollbackEvolutionSuggestion(ctx context.Context, req *v1.RollbackEvolutionSuggestionRequest) (*v1.EvolutionSuggestion, error) {
	if err := s.assertAgentMutateAccess(ctx, req.GetAgentId()); err != nil {
		return nil, err
	}
	result, err := s.evoUC.RollbackSuggestion(ctx, req.GetAgentId(), req.GetSuggestionId())
	if err != nil {
		return nil, err
	}
	// RollbackSuggestion restores agent prompt files from the pre-apply
	// snapshot; invalidate the cached build like Apply does.
	invalidateAgentBuildCache(req.GetAgentId())
	return toProtoSuggestion(result), nil
}
