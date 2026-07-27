package service

import (
	"context"

	v1 "aranea-agents/api/kratos/usage/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
)

// UsageService implements kratos usage.v1.
type UsageService struct {
	v1.UnimplementedUsageServiceServer

	uc *biz.UsageUsecase
}

func NewUsageService(uc *biz.UsageUsecase) *UsageService {
	return &UsageService{uc: uc}
}

// assertSystemCaller returns Forbidden if the caller is not a system
// caller. Quota/BudgetAlert/Purge operations are platform-level configurations
// that require system privileges.
//
// TECH-DEBT: UsageService has no lg field, so we cannot log IDOR attempts.
// Future refactor should inject loggateway.Logger via constructor.
func (s *UsageService) assertSystemCaller(ctx context.Context) error {
	if workspace.IsSystem(ctx) {
		return nil
	}
	if a, ok := auth.FromContext(ctx); ok && a.HasAdminAccess() {
		return nil
	}
	return apierror.Forbidden("USAGE", "system or admin privileges required")
}

// resolveWorkspaceID returns the workspace ID to filter by. System caller
// returns "" (no filter, see all workspaces); non-system caller returns
// the ctx workspace ID.
func (s *UsageService) resolveWorkspaceID(ctx context.Context) string {
	if workspace.IsSystem(ctx) {
		return ""
	}
	return workspace.IDFromContext(ctx)
}

func (s *UsageService) RecordTokenUsageEvent(ctx context.Context, in *v1.TokenUsageEvent) (*v1.TokenUsageEvent, error) {
	e := fromProtoTokenUsageEvent(in)
	// P2-C: workspace forgery guard. Non-system callers cannot set
	// WorkspaceID to anything other than their ctx workspace.
	if !workspace.IsSystem(ctx) {
		callerWS := workspace.IDFromContext(ctx)
		if e.WorkspaceID != "" && e.WorkspaceID != callerWS {
			// TECH-DEBT: no lg field, cannot log forgery attempt.
			e.WorkspaceID = callerWS // force-override
		} else if e.WorkspaceID == "" {
			e.WorkspaceID = callerWS
		}
	}
	recorded, err := s.uc.RecordTokenUsageEvent(ctx, e)
	if err != nil {
		return nil, err
	}
	return toProtoTokenUsageEvent(recorded), nil
}

func (s *UsageService) GetUsageOverview(ctx context.Context, in *v1.UsageQuery) (*v1.UsageOverview, error) {
	q := fromProtoUsageQuery(in)
	q.WorkspaceID = s.resolveWorkspaceID(ctx)
	o, err := s.uc.Overview(ctx, q)
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
	q := fromProtoUsageQuery(in)
	q.WorkspaceID = s.resolveWorkspaceID(ctx)
	items, err := s.uc.Trends(ctx, q)
	if err != nil {
		return nil, err
	}
	return &v1.ListUsageTrendsResponse{Items: toProtoUsageTrendPoints(items)}, nil
}

func (s *UsageService) ListTopModels(ctx context.Context, in *v1.UsageQuery) (*v1.ListBreakdownResponse, error) {
	q := fromProtoUsageQuery(in)
	q.WorkspaceID = s.resolveWorkspaceID(ctx)
	items, err := s.uc.TopModels(ctx, q)
	if err != nil {
		return nil, err
	}
	return &v1.ListBreakdownResponse{Items: toProtoUsageBreakdownRows(items)}, nil
}

func (s *UsageService) ListTopAgents(ctx context.Context, in *v1.UsageQuery) (*v1.ListBreakdownResponse, error) {
	q := fromProtoUsageQuery(in)
	q.WorkspaceID = s.resolveWorkspaceID(ctx)
	items, err := s.uc.TopAgents(ctx, q)
	if err != nil {
		return nil, err
	}
	return &v1.ListBreakdownResponse{Items: toProtoUsageBreakdownRows(items)}, nil
}

func (s *UsageService) ListUsageEvents(ctx context.Context, in *v1.UsageQuery) (*v1.ListUsageEventsResponse, error) {
	q := fromProtoUsageQuery(in)
	q.WorkspaceID = s.resolveWorkspaceID(ctx)
	page, err := s.uc.EventsPage(ctx, q)
	if err != nil {
		return nil, err
	}
	return &v1.ListUsageEventsResponse{
		Items: toProtoTokenUsageEvents(page.Items),
		Total: int32(page.Total),
	}, nil
}

func (s *UsageService) GetUsageQuota(ctx context.Context, req *v1.GetUsageQuotaRequest) (*v1.UsageQuota, error) {
	if err := s.assertSystemCaller(ctx); err != nil {
		return nil, err
	}
	q, err := s.uc.GetQuota(ctx, req.GetScopeType(), req.GetScopeId())
	if err != nil {
		return nil, err
	}
	return toProtoUsageQuota(q), nil
}

func (s *UsageService) SetUsageQuota(ctx context.Context, req *v1.SetUsageQuotaRequest) (*v1.UsageQuota, error) {
	if err := s.assertSystemCaller(ctx); err != nil {
		return nil, err
	}
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
	if err := s.assertSystemCaller(ctx); err != nil {
		return nil, err
	}
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
	if err := s.assertSystemCaller(ctx); err != nil {
		return nil, err
	}
	items, err := s.uc.ListBudgetAlerts(ctx, req.GetScopeType(), req.GetScopeId())
	if err != nil {
		return nil, err
	}
	return &v1.ListBudgetAlertsResponse{Items: toProtoBudgetAlerts(items)}, nil
}

func (s *UsageService) SetBudgetAlert(ctx context.Context, req *v1.SetBudgetAlertRequest) (*v1.BudgetAlert, error) {
	if err := s.assertSystemCaller(ctx); err != nil {
		return nil, err
	}
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
	q := fromProtoUsageQuery(in)
	q.WorkspaceID = s.resolveWorkspaceID(ctx)
	csv, err := s.uc.ExportUsageEventsCSV(ctx, q)
	if err != nil {
		return nil, err
	}
	return &v1.ExportUsageEventsResponse{Csv: csv}, nil
}

func (s *UsageService) PurgeUsageEvents(ctx context.Context, req *v1.PurgeUsageEventsRequest) (*v1.PurgeUsageEventsResponse, error) {
	if err := s.assertSystemCaller(ctx); err != nil {
		return nil, err
	}
	deleted, err := s.uc.PurgeEvents(ctx, int(req.GetRetainDays()))
	if err != nil {
		return nil, err
	}
	return &v1.PurgeUsageEventsResponse{DeletedCount: deleted}, nil
}

// ListAllModelsBreakdown returns a paginated, searchable, sortable breakdown of all models
// for the full-model consumption overview table. Server-side pagination/sort/search.
func (s *UsageService) ListAllModelsBreakdown(ctx context.Context, req *v1.ListAllModelsBreakdownRequest) (*v1.ListAllModelsBreakdownResponse, error) {
	q := fromProtoBreakdownQuery(req)
	q.WorkspaceID = s.resolveWorkspaceID(ctx)
	result, err := s.uc.AllModelsBreakdown(ctx, q)
	if err != nil {
		return nil, err
	}
	return &v1.ListAllModelsBreakdownResponse{
		Items:    toProtoUsageBreakdownRows(result.Items),
		Total:    result.Total,
		Page:     result.Page,
		PageSize: result.PageSize,
	}, nil
}
