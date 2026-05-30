package usage

import (
	"context"
	"errors"
	"testing"
	"time"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	"aranea-agents/internal/biz/shared"
)

type mockUsageRepo struct {
	getModelUsageSummaryFn          func(context.Context, Query) (Summary, error)
	listModelUsageTrendsFn          func(context.Context, Query) ([]TrendPoint, error)
	listTopModelUsageFn             func(context.Context, Query) ([]BreakdownRow, error)
	listTopAgentUsageFn             func(context.Context, Query) ([]BreakdownRow, error)
	listModelUsageEventsFn          func(context.Context, Query) ([]TokenUsageEvent, error)
	listModelUsageHourlyTrendsFn    func(context.Context, Query) ([]TrendPoint, error)
	getModelUsageSummaryFromDailyFn func(context.Context, Query) (Summary, error)
	listModelUsageDailyTrendsFn     func(context.Context, Query) ([]TrendPoint, error)
	listTopModelUsageFromDailyFn    func(context.Context, Query) ([]BreakdownRow, error)
	listTopAgentUsageFromDailyFn    func(context.Context, Query) ([]BreakdownRow, error)
	recordTokenUsageEventFn         func(context.Context, TokenUsageEvent) (TokenUsageEvent, error)
	getActiveModelPricingFn         func(context.Context, string, string) (ModelPricingSnapshot, bool, error)
	getQuotaFn                      func(context.Context, string, string) (Quota, error)
	setQuotaFn                      func(context.Context, Quota) (Quota, error)
	sumScopeCostInPeriodFn          func(context.Context, string, string, string, string) (int64, error)
	listActiveQuotasFn              func(context.Context) ([]Quota, error)
	batchSumScopeCostFn             func(context.Context, []Quota) (map[string]int64, error)
	listBudgetAlertsFn              func(context.Context, string, string) ([]BudgetAlert, error)
	setBudgetAlertFn                func(context.Context, BudgetAlert) (BudgetAlert, error)
	updateBudgetAlertLastFiredFn    func(context.Context, string, string) error
	purgeUsageEventsOlderThanFn    func(context.Context, int) (int64, error)
}

func (m *mockUsageRepo) GetModelUsageSummary(ctx context.Context, q Query) (Summary, error) {
	if m.getModelUsageSummaryFn != nil {
		return m.getModelUsageSummaryFn(ctx, q)
	}
	return Summary{}, nil
}

func (m *mockUsageRepo) ListModelUsageTrends(ctx context.Context, q Query) ([]TrendPoint, error) {
	if m.listModelUsageTrendsFn != nil {
		return m.listModelUsageTrendsFn(ctx, q)
	}
	return nil, nil
}

func (m *mockUsageRepo) ListTopModelUsage(ctx context.Context, q Query) ([]BreakdownRow, error) {
	if m.listTopModelUsageFn != nil {
		return m.listTopModelUsageFn(ctx, q)
	}
	return nil, nil
}

func (m *mockUsageRepo) ListTopAgentUsage(ctx context.Context, q Query) ([]BreakdownRow, error) {
	if m.listTopAgentUsageFn != nil {
		return m.listTopAgentUsageFn(ctx, q)
	}
	return nil, nil
}

func (m *mockUsageRepo) ListModelUsageEvents(ctx context.Context, q Query) ([]TokenUsageEvent, error) {
	if m.listModelUsageEventsFn != nil {
		return m.listModelUsageEventsFn(ctx, q)
	}
	return nil, nil
}

func (m *mockUsageRepo) ListModelUsageHourlyTrends(ctx context.Context, q Query) ([]TrendPoint, error) {
	if m.listModelUsageHourlyTrendsFn != nil {
		return m.listModelUsageHourlyTrendsFn(ctx, q)
	}
	return nil, nil
}

func (m *mockUsageRepo) GetModelUsageSummaryFromDaily(ctx context.Context, q Query) (Summary, error) {
	if m.getModelUsageSummaryFromDailyFn != nil {
		return m.getModelUsageSummaryFromDailyFn(ctx, q)
	}
	return Summary{}, nil
}

func (m *mockUsageRepo) ListModelUsageDailyTrends(ctx context.Context, q Query) ([]TrendPoint, error) {
	if m.listModelUsageDailyTrendsFn != nil {
		return m.listModelUsageDailyTrendsFn(ctx, q)
	}
	return nil, nil
}

func (m *mockUsageRepo) ListTopModelUsageFromDaily(ctx context.Context, q Query) ([]BreakdownRow, error) {
	if m.listTopModelUsageFromDailyFn != nil {
		return m.listTopModelUsageFromDailyFn(ctx, q)
	}
	return nil, nil
}

func (m *mockUsageRepo) ListTopAgentUsageFromDaily(ctx context.Context, q Query) ([]BreakdownRow, error) {
	if m.listTopAgentUsageFromDailyFn != nil {
		return m.listTopAgentUsageFromDailyFn(ctx, q)
	}
	return nil, nil
}

func (m *mockUsageRepo) RecordTokenUsageEvent(ctx context.Context, e TokenUsageEvent) (TokenUsageEvent, error) {
	if m.recordTokenUsageEventFn != nil {
		return m.recordTokenUsageEventFn(ctx, e)
	}
	return TokenUsageEvent{}, nil
}

func (m *mockUsageRepo) GetActiveModelPricing(ctx context.Context, providerCode, modelAPIID string) (ModelPricingSnapshot, bool, error) {
	if m.getActiveModelPricingFn != nil {
		return m.getActiveModelPricingFn(ctx, providerCode, modelAPIID)
	}
	return ModelPricingSnapshot{}, false, nil
}

func (m *mockUsageRepo) GetQuota(ctx context.Context, scopeType, scopeID string) (Quota, error) {
	if m.getQuotaFn != nil {
		return m.getQuotaFn(ctx, scopeType, scopeID)
	}
	return Quota{}, nil
}

func (m *mockUsageRepo) SetQuota(ctx context.Context, q Quota) (Quota, error) {
	if m.setQuotaFn != nil {
		return m.setQuotaFn(ctx, q)
	}
	return Quota{}, nil
}

func (m *mockUsageRepo) SumScopeCostInPeriod(ctx context.Context, scopeType, scopeID, periodStart, periodEnd string) (int64, error) {
	if m.sumScopeCostInPeriodFn != nil {
		return m.sumScopeCostInPeriodFn(ctx, scopeType, scopeID, periodStart, periodEnd)
	}
	return 0, nil
}

func (m *mockUsageRepo) ListActiveQuotas(ctx context.Context) ([]Quota, error) {
	if m.listActiveQuotasFn != nil {
		return m.listActiveQuotasFn(ctx)
	}
	return nil, nil
}

func (m *mockUsageRepo) BatchSumScopeCost(ctx context.Context, quotas []Quota) (map[string]int64, error) {
	if m.batchSumScopeCostFn != nil {
		return m.batchSumScopeCostFn(ctx, quotas)
	}
	return map[string]int64{}, nil
}

func (m *mockUsageRepo) ListBudgetAlerts(ctx context.Context, scopeType, scopeID string) ([]BudgetAlert, error) {
	if m.listBudgetAlertsFn != nil {
		return m.listBudgetAlertsFn(ctx, scopeType, scopeID)
	}
	return nil, nil
}

func (m *mockUsageRepo) SetBudgetAlert(ctx context.Context, a BudgetAlert) (BudgetAlert, error) {
	if m.setBudgetAlertFn != nil {
		return m.setBudgetAlertFn(ctx, a)
	}
	return BudgetAlert{}, nil
}

func (m *mockUsageRepo) UpdateBudgetAlertLastFired(ctx context.Context, id, firedAt string) error {
	if m.updateBudgetAlertLastFiredFn != nil {
		return m.updateBudgetAlertLastFiredFn(ctx, id, firedAt)
	}
	return nil
}

func (m *mockUsageRepo) PurgeUsageEventsOlderThan(ctx context.Context, retainDays int) (int64, error) {
	if m.purgeUsageEventsOlderThanFn != nil {
		return m.purgeUsageEventsOlderThanFn(ctx, retainDays)
	}
	return 0, nil
}

var fixedNow = time.Date(2025, 3, 15, 12, 0, 0, 0, time.UTC)

func newTestUsecase(repo Repo) *Usecase {
	u := NewUsecase(repo)
	u.now = func() time.Time { return fixedNow }
	return u
}

func TestOverview(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*mockUsageRepo)
		query   Query
		wantErr bool
		check   func(t *testing.T, ov Overview)
	}{
		{
			name: "valid_query_returns_data",
			setup: func(r *mockUsageRepo) {
				r.getModelUsageSummaryFn = func(_ context.Context, _ Query) (Summary, error) {
					return Summary{CallCount: 10, TotalTokens: 1000}, nil
				}
				r.getModelUsageSummaryFromDailyFn = func(_ context.Context, _ Query) (Summary, error) {
					return Summary{CallCount: 5, TotalTokens: 500}, nil
				}
				r.listModelUsageTrendsFn = func(_ context.Context, _ Query) ([]TrendPoint, error) {
					return []TrendPoint{{DateKey: "2025-03-15", CallCount: 3}}, nil
				}
				r.listTopModelUsageFn = func(_ context.Context, _ Query) ([]BreakdownRow, error) {
					return []BreakdownRow{{ProviderCode: "openai", ModelAPIID: "gpt-4", CallCount: 8}}, nil
				}
				r.listTopAgentUsageFn = func(_ context.Context, _ Query) ([]BreakdownRow, error) {
					return []BreakdownRow{{AgentID: "agent-1", CallCount: 5}}, nil
				}
				r.listModelUsageEventsFn = func(_ context.Context, _ Query) ([]TokenUsageEvent, error) {
					return nil, nil
				}
				r.listActiveQuotasFn = func(_ context.Context) ([]Quota, error) {
					return nil, nil
				}
			},
			query: Query{Range: "7d"},
			check: func(t *testing.T, ov Overview) {
				if ov.Today.CallCount != 10 {
					t.Errorf("Today.CallCount = %d, want 10", ov.Today.CallCount)
				}
				if ov.Today.TotalTokens != 1000 {
					t.Errorf("Today.TotalTokens = %d, want 1000", ov.Today.TotalTokens)
				}
				if ov.Yesterday.CallCount != 5 {
					t.Errorf("Yesterday.CallCount = %d, want 5", ov.Yesterday.CallCount)
				}
				if len(ov.Trends) != 1 || ov.Trends[0].DateKey != "2025-03-15" {
					t.Errorf("Trends = %v, want 1 point with date 2025-03-15", ov.Trends)
				}
				if len(ov.TopModels) != 1 || ov.TopModels[0].ProviderCode != "openai" {
					t.Errorf("TopModels = %v, want 1 row with openai", ov.TopModels)
				}
				if len(ov.TopAgents) != 1 || ov.TopAgents[0].AgentID != "agent-1" {
					t.Errorf("TopAgents = %v, want 1 row with agent-1", ov.TopAgents)
				}
			},
		},
		{
			name: "repo_error_propagation",
			setup: func(r *mockUsageRepo) {
				r.getModelUsageSummaryFn = func(_ context.Context, _ Query) (Summary, error) {
					return Summary{}, errors.New("db connection lost")
				}
			},
			query:   Query{Range: "7d"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUsageRepo{}
			tt.setup(repo)
			uc := newTestUsecase(repo)
			ov, err := uc.Overview(context.Background(), tt.query)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Overview() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Overview() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, ov)
			}
		})
	}
}

func TestTrends(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*mockUsageRepo)
		query   Query
		wantErr bool
		check   func(t *testing.T, points []TrendPoint)
	}{
		{
			name: "valid_query_returns_trend_points",
			setup: func(r *mockUsageRepo) {
				r.listModelUsageTrendsFn = func(_ context.Context, _ Query) ([]TrendPoint, error) {
					return []TrendPoint{
						{DateKey: "2025-03-14", CallCount: 5, TotalTokens: 500},
						{DateKey: "2025-03-15", CallCount: 8, TotalTokens: 800},
					}, nil
				}
			},
			query: Query{StartDate: "2025-03-14", EndDate: "2025-03-15"},
			check: func(t *testing.T, points []TrendPoint) {
				if len(points) != 2 {
					t.Fatalf("len(points) = %d, want 2", len(points))
				}
				if points[0].CallCount != 5 {
					t.Errorf("points[0].CallCount = %d, want 5", points[0].CallCount)
				}
				if points[1].TotalTokens != 800 {
					t.Errorf("points[1].TotalTokens = %d, want 800", points[1].TotalTokens)
				}
			},
		},
		{
			name: "empty_result",
			setup: func(r *mockUsageRepo) {
				r.listModelUsageTrendsFn = func(_ context.Context, _ Query) ([]TrendPoint, error) {
					return nil, nil
				}
			},
			query: Query{StartDate: "2025-03-14", EndDate: "2025-03-15"},
			check: func(t *testing.T, points []TrendPoint) {
				if len(points) != 0 {
					t.Errorf("len(points) = %d, want 0", len(points))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUsageRepo{}
			tt.setup(repo)
			uc := newTestUsecase(repo)
			points, err := uc.Trends(context.Background(), tt.query)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Trends() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Trends() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, points)
			}
		})
	}
}

func TestTopModels(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*mockUsageRepo)
		query Query
		check func(t *testing.T, rows []BreakdownRow)
	}{
		{
			name: "returns_breakdown_rows",
			setup: func(r *mockUsageRepo) {
				r.listTopModelUsageFn = func(_ context.Context, _ Query) ([]BreakdownRow, error) {
					return []BreakdownRow{
						{ProviderCode: "openai", ModelAPIID: "gpt-4", CallCount: 100},
						{ProviderCode: "anthropic", ModelAPIID: "claude-3", CallCount: 50},
					}, nil
				}
			},
			query: Query{Range: "30d"},
			check: func(t *testing.T, rows []BreakdownRow) {
				if len(rows) != 2 {
					t.Fatalf("len(rows) = %d, want 2", len(rows))
				}
				if rows[0].ProviderCode != "openai" {
					t.Errorf("rows[0].ProviderCode = %q, want %q", rows[0].ProviderCode, "openai")
				}
				if rows[1].CallCount != 50 {
					t.Errorf("rows[1].CallCount = %d, want 50", rows[1].CallCount)
				}
			},
		},
		{
			name: "empty_result",
			setup: func(r *mockUsageRepo) {
				r.listTopModelUsageFn = func(_ context.Context, _ Query) ([]BreakdownRow, error) {
					return nil, nil
				}
			},
			query: Query{Range: "30d"},
			check: func(t *testing.T, rows []BreakdownRow) {
				if len(rows) != 0 {
					t.Errorf("len(rows) = %d, want 0", len(rows))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUsageRepo{}
			tt.setup(repo)
			uc := newTestUsecase(repo)
			rows, err := uc.TopModels(context.Background(), tt.query)
			if err != nil {
				t.Fatalf("TopModels() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, rows)
			}
		})
	}
}

func TestCheckQuota(t *testing.T) {
	tests := []struct {
		name      string
		scopeType string
		scopeID   string
		setup     func(*mockUsageRepo)
		wantErr   bool
		check     func(t *testing.T, qc QuotaCheck)
	}{
		{
			name:      "quota_not_exceeded",
			scopeType: "agent",
			scopeID:   "agent-1",
			setup: func(r *mockUsageRepo) {
				r.getQuotaFn = func(_ context.Context, _, _ string) (Quota, error) {
					return Quota{
						ScopeType:       "agent",
						ScopeID:         "agent-1",
						MonthlyMicroUSD: 10000,
						PeriodStart:     "2025-03-01",
						PeriodEnd:       "2025-03-31",
					}, nil
				}
				r.sumScopeCostInPeriodFn = func(_ context.Context, _, _, _, _ string) (int64, error) {
					return 5000, nil
				}
			},
			check: func(t *testing.T, qc QuotaCheck) {
				if !qc.Allowed {
					t.Error("Allowed = false, want true")
				}
				if qc.SpentMicroUSD != 5000 {
					t.Errorf("SpentMicroUSD = %d, want 5000", qc.SpentMicroUSD)
				}
				if qc.RemainingMicroUSD != 5000 {
					t.Errorf("RemainingMicroUSD = %d, want 5000", qc.RemainingMicroUSD)
				}
				if qc.Reason != "within quota" {
					t.Errorf("Reason = %q, want %q", qc.Reason, "within quota")
				}
			},
		},
		{
			name:      "quota_exceeded",
			scopeType: "agent",
			scopeID:   "agent-2",
			setup: func(r *mockUsageRepo) {
				r.getQuotaFn = func(_ context.Context, _, _ string) (Quota, error) {
					return Quota{
						ScopeType:       "agent",
						ScopeID:         "agent-2",
						MonthlyMicroUSD: 10000,
						PeriodStart:     "2025-03-01",
						PeriodEnd:       "2025-03-31",
					}, nil
				}
				r.sumScopeCostInPeriodFn = func(_ context.Context, _, _, _, _ string) (int64, error) {
					return 12000, nil
				}
			},
			check: func(t *testing.T, qc QuotaCheck) {
				if qc.Allowed {
					t.Error("Allowed = true, want false")
				}
				if qc.SpentMicroUSD != 12000 {
					t.Errorf("SpentMicroUSD = %d, want 12000", qc.SpentMicroUSD)
				}
				if qc.RemainingMicroUSD != 0 {
					t.Errorf("RemainingMicroUSD = %d, want 0", qc.RemainingMicroUSD)
				}
			},
		},
		{
			name:      "repo_error",
			scopeType: "agent",
			scopeID:   "agent-3",
			setup: func(r *mockUsageRepo) {
				r.getQuotaFn = func(_ context.Context, _, _ string) (Quota, error) {
					return Quota{}, errors.New("db timeout")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUsageRepo{}
			tt.setup(repo)
			uc := newTestUsecase(repo)
			qc, err := uc.CheckQuota(context.Background(), tt.scopeType, tt.scopeID)
			if tt.wantErr {
				if err == nil {
					t.Fatal("CheckQuota() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("CheckQuota() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, qc)
			}
		})
	}
}

func TestGetQuota(t *testing.T) {
	tests := []struct {
		name       string
		scopeType  string
		scopeID    string
		setup      func(*mockUsageRepo)
		wantErr    bool
		wantReason string
		wantCode   int32
		check      func(t *testing.T, q Quota)
	}{
		{
			name:      "existing_quota",
			scopeType: "agent",
			scopeID:   "agent-1",
			setup: func(r *mockUsageRepo) {
				r.getQuotaFn = func(_ context.Context, _, _ string) (Quota, error) {
					return Quota{
						ScopeType:       "agent",
						ScopeID:         "agent-1",
						MonthlyMicroUSD: 50000,
						PeriodStart:     "2025-03-01",
						PeriodEnd:       "2025-03-31",
					}, nil
				}
			},
			check: func(t *testing.T, q Quota) {
				if q.MonthlyMicroUSD != 50000 {
					t.Errorf("MonthlyMicroUSD = %d, want 50000", q.MonthlyMicroUSD)
				}
				if q.ScopeType != "agent" {
					t.Errorf("ScopeType = %q, want %q", q.ScopeType, "agent")
				}
			},
		},
		{
			name:      "not_found",
			scopeType: "agent",
			scopeID:   "agent-unknown",
			setup: func(r *mockUsageRepo) {
				r.getQuotaFn = func(_ context.Context, _, _ string) (Quota, error) {
					return Quota{}, shared.ErrQuotaNotFound
				}
			},
			wantErr:    true,
			wantReason: "USAGE_QUOTA",
			wantCode:   404,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUsageRepo{}
			tt.setup(repo)
			uc := newTestUsecase(repo)
			q, err := uc.GetQuota(context.Background(), tt.scopeType, tt.scopeID)
			if tt.wantErr {
				if err == nil {
					t.Fatal("GetQuota() expected error, got nil")
				}
				se := kerrors.FromError(err)
				if se == nil {
					t.Fatalf("expected kratos error, got %T", err)
				}
				if se.Reason != tt.wantReason {
					t.Errorf("reason = %q, want %q", se.Reason, tt.wantReason)
				}
				if se.Code != tt.wantCode {
					t.Errorf("code = %d, want %d", se.Code, tt.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetQuota() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, q)
			}
		})
	}
}
