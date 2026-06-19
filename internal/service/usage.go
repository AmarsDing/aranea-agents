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

func (s *UsageService) RecordTokenUsageEvent(ctx context.Context, in *v1.TokenUsageEvent) (*v1.TokenUsageEvent, error) {
	e, err := s.uc.RecordTokenUsageEvent(ctx, fromProtoTokenUsageEvent(in))
	if err != nil {
		return nil, err
	}
	return toProtoTokenUsageEvent(e), nil
}

func (s *UsageService) GetUsageOverview(ctx context.Context, in *v1.UsageQuery) (*v1.UsageOverview, error) {
	o, err := s.uc.Overview(ctx, fromProtoUsageQuery(in))
	if err != nil {
		return nil, err
	}
	return &v1.UsageOverview{
		Today:             toProtoUsageSummary(o.Today),
		Yesterday:         toProtoUsageSummary(o.Yesterday),
		Month:             toProtoUsageSummary(o.Month),
		RangeSummary:      toProtoUsageSummary(o.Range),
		Trends:            toProtoUsageTrendPoints(o.Trends),
		TopModels:         toProtoUsageBreakdownRows(o.TopModels),
		TopAgents:         toProtoUsageBreakdownRows(o.TopAgents),
		Anomalies:         toProtoTokenUsageEvents(o.Anomalies),
		QuotaDashboard:    toProtoQuotaDashboard(o.QuotaDashboard),
		InefficientModels: toProtoUsageModelInsights(o.InefficientModels),
	}, nil
}

func (s *UsageService) ListUsageTrends(ctx context.Context, in *v1.UsageQuery) (*v1.ListUsageTrendsResponse, error) {
	items, err := s.uc.Trends(ctx, fromProtoUsageQuery(in))
	if err != nil {
		return nil, err
	}
	return &v1.ListUsageTrendsResponse{Items: toProtoUsageTrendPoints(items)}, nil
}

func (s *UsageService) ListTopModels(ctx context.Context, in *v1.UsageQuery) (*v1.ListBreakdownResponse, error) {
	items, err := s.uc.TopModels(ctx, fromProtoUsageQuery(in))
	if err != nil {
		return nil, err
	}
	return &v1.ListBreakdownResponse{Items: toProtoUsageBreakdownRows(items)}, nil
}

func (s *UsageService) ListTopAgents(ctx context.Context, in *v1.UsageQuery) (*v1.ListBreakdownResponse, error) {
	items, err := s.uc.TopAgents(ctx, fromProtoUsageQuery(in))
	if err != nil {
		return nil, err
	}
	return &v1.ListBreakdownResponse{Items: toProtoUsageBreakdownRows(items)}, nil
}

func (s *UsageService) ListUsageEvents(ctx context.Context, in *v1.UsageQuery) (*v1.ListUsageEventsResponse, error) {
	items, err := s.uc.Events(ctx, fromProtoUsageQuery(in))
	if err != nil {
		return nil, err
	}
	return &v1.ListUsageEventsResponse{Items: toProtoTokenUsageEvents(items)}, nil
}

func (s *UsageService) GetUsageQuota(ctx context.Context, req *v1.GetUsageQuotaRequest) (*v1.UsageQuota, error) {
	q, err := s.uc.GetQuota(ctx, req.GetScopeType(), req.GetScopeId())
	if err != nil {
		return nil, err
	}
	return toProtoUsageQuota(q), nil
}

func (s *UsageService) SetUsageQuota(ctx context.Context, req *v1.SetUsageQuotaRequest) (*v1.UsageQuota, error) {
	q, err := s.uc.SetQuota(ctx, biz.UsageQuota{
		ScopeType:       req.GetScopeType(),
		ScopeID:         req.GetScopeId(),
		MonthlyMicroUSD: req.GetMonthlyMicroUsd(),
		PeriodStart:     req.GetPeriodStart(),
		PeriodEnd:       req.GetPeriodEnd(),
	})
	if err != nil {
		return nil, err
	}
	return toProtoUsageQuota(q), nil
}

func (s *UsageService) CheckUsageQuota(ctx context.Context, req *v1.CheckUsageQuotaRequest) (*v1.CheckUsageQuotaResponse, error) {
	check, err := s.uc.CheckQuota(ctx, req.GetScopeType(), req.GetScopeId())
	if err != nil {
		return nil, err
	}
	return &v1.CheckUsageQuotaResponse{
		Allowed:           check.Allowed,
		Quota:             toProtoUsageQuota(check.Quota),
		SpentMicroUsd:     check.SpentMicroUSD,
		RemainingMicroUsd: check.RemainingMicroUSD,
		Reason:            check.Reason,
	}, nil
}

func (s *UsageService) ListBudgetAlerts(ctx context.Context, req *v1.ListBudgetAlertsRequest) (*v1.ListBudgetAlertsResponse, error) {
	items, err := s.uc.ListBudgetAlerts(ctx, req.GetScopeType(), req.GetScopeId())
	if err != nil {
		return nil, err
	}
	return &v1.ListBudgetAlertsResponse{Items: toProtoBudgetAlerts(items)}, nil
}

func (s *UsageService) SetBudgetAlert(ctx context.Context, req *v1.SetBudgetAlertRequest) (*v1.BudgetAlert, error) {
	a, err := s.uc.SetBudgetAlert(ctx, biz.BudgetAlert{
		ScopeType:  req.GetScopeType(),
		ScopeID:    req.GetScopeId(),
		AlertRatio: req.GetAlertRatio(),
		Enabled:    req.GetEnabled(),
	})
	if err != nil {
		return nil, err
	}
	return toProtoBudgetAlert(a), nil
}

func (s *UsageService) ExportUsageEvents(ctx context.Context, in *v1.UsageQuery) (*v1.ExportUsageEventsResponse, error) {
	csv, err := s.uc.ExportUsageEventsCSV(ctx, fromProtoUsageQuery(in))
	if err != nil {
		return nil, err
	}
	return &v1.ExportUsageEventsResponse{Csv: csv}, nil
}

func (s *UsageService) PurgeUsageEvents(ctx context.Context, req *v1.PurgeUsageEventsRequest) (*v1.PurgeUsageEventsResponse, error) {
	deleted, err := s.uc.PurgeEvents(ctx, int(req.GetRetainDays()))
	if err != nil {
		return nil, err
	}
	return &v1.PurgeUsageEventsResponse{DeletedCount: deleted}, nil
}
