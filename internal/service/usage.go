package service

import (
	"context"
	"time"

	v1 "aranea-agents/api/kratos/usage/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

// UsageService implements kratos usage.v1.
type UsageService struct {
	v1.UnimplementedUsageServiceServer

	uc *biz.UsageUsecase
	lg loggateway.Logger
}

func NewUsageService(uc *biz.UsageUsecase, lg loggateway.Logger) *UsageService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &UsageService{uc: uc, lg: lg}
}

// assertSystemCaller returns Forbidden if the caller is not a system
// caller. Quota/BudgetAlert/Purge operations are platform-level configurations
// that require system privileges.
//
func (s *UsageService) assertSystemCaller(ctx context.Context) error {
	if workspace.IsSystem(ctx) {
		return nil
	}
	if a, ok := auth.FromContext(ctx); ok && a.HasAdminAccess() {
		return nil
	}
	s.lg.Warn("usage privileged op denied for non-system caller",
		loggateway.StepID("usage.assert_system_caller"),
		loggateway.Str("workspace_id", workspace.IDFromContext(ctx)))
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
			s.lg.Warn("usage workspace_id forgery overridden",
				loggateway.StepID("usage.workspace_forgery"),
				loggateway.Str("caller_workspace", callerWS),
				loggateway.Str("requested_workspace", e.WorkspaceID))
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

// GetContextBudgetStats serves the P2-1 cross-turn context budget aggregation
// (29-token.development.md §18). Reuses UsageQuery filters; workspace scoping
// follows the same rule as other analytics RPCs.
func (s *UsageService) GetContextBudgetStats(ctx context.Context, req *v1.UsageQuery) (*v1.GetContextBudgetStatsResponse, error) {
	q := fromProtoUsageQuery(req)
	q.WorkspaceID = s.resolveWorkspaceID(ctx)
	stats, err := s.uc.ContextBudgetStats(ctx, q)
	if err != nil {
		return nil, err
	}
	return toProtoContextBudgetStats(stats), nil
}

// GetCacheHitRatioStats serves the 29-token §9.3 prompt-cache observability
// read path. Global (provider, model) aggregation with no workspace dimension,
// so it uses the same platform-level caller rule as budget alerts.
func (s *UsageService) GetCacheHitRatioStats(ctx context.Context, req *v1.GetCacheHitRatioStatsRequest) (*v1.GetCacheHitRatioStatsResponse, error) {
	if err := s.assertSystemCaller(ctx); err != nil {
		return nil, err
	}
	stats, err := s.uc.CacheHitRatioStats(ctx, clampCacheHitWindow(req.GetWindowHours()))
	if err != nil {
		return nil, err
	}
	return toProtoCacheHitRatioStats(stats), nil
}

// clampCacheHitWindow normalizes the trailing aggregation window: 0/negative
// falls back to the 1h default (matching the alert window), capped at 7 days.
func clampCacheHitWindow(hours int32) time.Duration {
	if hours < 1 {
		hours = 1
	}
	if hours > 168 {
		hours = 168
	}
	return time.Duration(hours) * time.Hour
}
