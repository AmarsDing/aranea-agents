package usage_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	usage "aranea-agents/internal/biz/usage"
	"aranea-agents/internal/biz/shared"
)

type mockRepo struct {
	getModelUsageSummaryFn          func(context.Context, usage.Query) (usage.Summary, error)
	listModelUsageTrendsFn          func(context.Context, usage.Query) ([]usage.TrendPoint, error)
	listTopModelUsageFn             func(context.Context, usage.Query) ([]usage.BreakdownRow, error)
	listTopAgentUsageFn             func(context.Context, usage.Query) ([]usage.BreakdownRow, error)
	listModelUsageEventsFn          func(context.Context, usage.Query) ([]usage.TokenUsageEvent, error)
	listModelUsageHourlyTrendsFn    func(context.Context, usage.Query) ([]usage.TrendPoint, error)
	getModelUsageSummaryFromDailyFn func(context.Context, usage.Query) (usage.Summary, error)
	listModelUsageDailyTrendsFn     func(context.Context, usage.Query) ([]usage.TrendPoint, error)
	listTopModelUsageFromDailyFn    func(context.Context, usage.Query) ([]usage.BreakdownRow, error)
	listTopAgentUsageFromDailyFn    func(context.Context, usage.Query) ([]usage.BreakdownRow, error)
	recordTokenUsageEventFn         func(context.Context, usage.TokenUsageEvent) (usage.TokenUsageEvent, error)
	getActiveModelPricingFn         func(context.Context, string, string) (usage.ModelPricingSnapshot, bool, error)
	getQuotaFn                      func(context.Context, string, string) (usage.Quota, error)
	setQuotaFn                      func(context.Context, usage.Quota) (usage.Quota, error)
	sumScopeCostInPeriodFn          func(context.Context, string, string, string, string) (int64, error)
	listActiveQuotasFn              func(context.Context) ([]usage.Quota, error)
	batchSumScopeCostFn             func(context.Context, []usage.Quota) (map[string]int64, error)
	listBudgetAlertsFn              func(context.Context, string, string) ([]usage.BudgetAlert, error)
	setBudgetAlertFn                func(context.Context, usage.BudgetAlert) (usage.BudgetAlert, error)
	updateBudgetAlertLastFiredFn    func(context.Context, string, string) error
	purgeUsageEventsOlderThanFn     func(context.Context, int) (int64, error)
}

func (m *mockRepo) GetModelUsageSummary(ctx context.Context, q usage.Query) (usage.Summary, error) {
	if m.getModelUsageSummaryFn != nil {
		return m.getModelUsageSummaryFn(ctx, q)
	}
	return usage.Summary{}, nil
}

func (m *mockRepo) ListModelUsageTrends(ctx context.Context, q usage.Query) ([]usage.TrendPoint, error) {
	if m.listModelUsageTrendsFn != nil {
		return m.listModelUsageTrendsFn(ctx, q)
	}
	return nil, nil
}

func (m *mockRepo) ListTopModelUsage(ctx context.Context, q usage.Query) ([]usage.BreakdownRow, error) {
	if m.listTopModelUsageFn != nil {
		return m.listTopModelUsageFn(ctx, q)
	}
	return nil, nil
}

func (m *mockRepo) ListTopAgentUsage(ctx context.Context, q usage.Query) ([]usage.BreakdownRow, error) {
	if m.listTopAgentUsageFn != nil {
		return m.listTopAgentUsageFn(ctx, q)
	}
	return nil, nil
}

func (m *mockRepo) ListModelUsageEvents(ctx context.Context, q usage.Query) ([]usage.TokenUsageEvent, error) {
	if m.listModelUsageEventsFn != nil {
		return m.listModelUsageEventsFn(ctx, q)
	}
	return nil, nil
}

func (m *mockRepo) ListModelUsageHourlyTrends(ctx context.Context, q usage.Query) ([]usage.TrendPoint, error) {
	if m.listModelUsageHourlyTrendsFn != nil {
		return m.listModelUsageHourlyTrendsFn(ctx, q)
	}
	return nil, nil
}

func (m *mockRepo) GetModelUsageSummaryFromDaily(ctx context.Context, q usage.Query) (usage.Summary, error) {
	if m.getModelUsageSummaryFromDailyFn != nil {
		return m.getModelUsageSummaryFromDailyFn(ctx, q)
	}
	return usage.Summary{}, nil
}

func (m *mockRepo) ListModelUsageDailyTrends(ctx context.Context, q usage.Query) ([]usage.TrendPoint, error) {
	if m.listModelUsageDailyTrendsFn != nil {
		return m.listModelUsageDailyTrendsFn(ctx, q)
	}
	return nil, nil
}

func (m *mockRepo) ListTopModelUsageFromDaily(ctx context.Context, q usage.Query) ([]usage.BreakdownRow, error) {
	if m.listTopModelUsageFromDailyFn != nil {
		return m.listTopModelUsageFromDailyFn(ctx, q)
	}
	return nil, nil
}

func (m *mockRepo) ListTopAgentUsageFromDaily(ctx context.Context, q usage.Query) ([]usage.BreakdownRow, error) {
	if m.listTopAgentUsageFromDailyFn != nil {
		return m.listTopAgentUsageFromDailyFn(ctx, q)
	}
	return nil, nil
}

func (m *mockRepo) RecordTokenUsageEvent(ctx context.Context, e usage.TokenUsageEvent) (usage.TokenUsageEvent, error) {
	if m.recordTokenUsageEventFn != nil {
		return m.recordTokenUsageEventFn(ctx, e)
	}
	return usage.TokenUsageEvent{}, nil
}

func (m *mockRepo) GetActiveModelPricing(ctx context.Context, providerCode, modelAPIID string) (usage.ModelPricingSnapshot, bool, error) {
	if m.getActiveModelPricingFn != nil {
		return m.getActiveModelPricingFn(ctx, providerCode, modelAPIID)
	}
	return usage.ModelPricingSnapshot{}, false, nil
}

func (m *mockRepo) GetQuota(ctx context.Context, scopeType, scopeID string) (usage.Quota, error) {
	if m.getQuotaFn != nil {
		return m.getQuotaFn(ctx, scopeType, scopeID)
	}
	return usage.Quota{}, nil
}

func (m *mockRepo) SetQuota(ctx context.Context, q usage.Quota) (usage.Quota, error) {
	if m.setQuotaFn != nil {
		return m.setQuotaFn(ctx, q)
	}
	return usage.Quota{}, nil
}

func (m *mockRepo) SumScopeCostInPeriod(ctx context.Context, scopeType, scopeID, periodStart, periodEnd string) (int64, error) {
	if m.sumScopeCostInPeriodFn != nil {
		return m.sumScopeCostInPeriodFn(ctx, scopeType, scopeID, periodStart, periodEnd)
	}
	return 0, nil
}

func (m *mockRepo) ListActiveQuotas(ctx context.Context) ([]usage.Quota, error) {
	if m.listActiveQuotasFn != nil {
		return m.listActiveQuotasFn(ctx)
	}
	return nil, nil
}

func (m *mockRepo) BatchSumScopeCost(ctx context.Context, quotas []usage.Quota) (map[string]int64, error) {
	if m.batchSumScopeCostFn != nil {
		return m.batchSumScopeCostFn(ctx, quotas)
	}
	return map[string]int64{}, nil
}

func (m *mockRepo) ListBudgetAlerts(ctx context.Context, scopeType, scopeID string) ([]usage.BudgetAlert, error) {
	if m.listBudgetAlertsFn != nil {
		return m.listBudgetAlertsFn(ctx, scopeType, scopeID)
	}
	return nil, nil
}

func (m *mockRepo) SetBudgetAlert(ctx context.Context, a usage.BudgetAlert) (usage.BudgetAlert, error) {
	if m.setBudgetAlertFn != nil {
		return m.setBudgetAlertFn(ctx, a)
	}
	return usage.BudgetAlert{}, nil
}

func (m *mockRepo) UpdateBudgetAlertLastFired(ctx context.Context, id, firedAt string) error {
	if m.updateBudgetAlertLastFiredFn != nil {
		return m.updateBudgetAlertLastFiredFn(ctx, id, firedAt)
	}
	return nil
}

func (m *mockRepo) PurgeUsageEventsOlderThan(ctx context.Context, retainDays int) (int64, error) {
	if m.purgeUsageEventsOlderThanFn != nil {
		return m.purgeUsageEventsOlderThanFn(ctx, retainDays)
	}
	return 0, nil
}

var fixedNow = time.Date(2025, 3, 15, 12, 0, 0, 0, time.UTC)

func newTestUsecase(repo usage.Repo) *usage.Usecase {
	u := usage.NewUsecase(repo)
	usage.SetUsecaseNow(u, func() time.Time { return fixedNow })
	return u
}

func TestTopAgents(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*mockRepo)
		query   usage.Query
		wantErr bool
		check   func(t *testing.T, rows []usage.BreakdownRow)
	}{
		{
			name: "returns_breakdown_rows",
			setup: func(r *mockRepo) {
				r.listTopAgentUsageFn = func(_ context.Context, _ usage.Query) ([]usage.BreakdownRow, error) {
					return []usage.BreakdownRow{
						{AgentID: "agent-1", CallCount: 100},
						{AgentID: "agent-2", CallCount: 50},
					}, nil
				}
			},
			query: usage.Query{Range: "30d"},
			check: func(t *testing.T, rows []usage.BreakdownRow) {
				if len(rows) != 2 {
					t.Fatalf("len(rows) = %d, want 2", len(rows))
				}
				if rows[0].AgentID != "agent-1" {
					t.Errorf("rows[0].AgentID = %q, want %q", rows[0].AgentID, "agent-1")
				}
				if rows[1].CallCount != 50 {
					t.Errorf("rows[1].CallCount = %d, want 50", rows[1].CallCount)
				}
			},
		},
		{
			name: "empty_result",
			setup: func(r *mockRepo) {
				r.listTopAgentUsageFn = func(_ context.Context, _ usage.Query) ([]usage.BreakdownRow, error) {
					return nil, nil
				}
			},
			query: usage.Query{Range: "30d"},
			check: func(t *testing.T, rows []usage.BreakdownRow) {
				if len(rows) != 0 {
					t.Errorf("len(rows) = %d, want 0", len(rows))
				}
			},
		},
		{
			name: "repo_error",
			setup: func(r *mockRepo) {
				r.listTopAgentUsageFn = func(_ context.Context, _ usage.Query) ([]usage.BreakdownRow, error) {
					return nil, errors.New("db timeout")
				}
			},
			query:   usage.Query{Range: "30d"},
			wantErr: true,
		},
		{
			name: "query_normalization_applied",
			setup: func(r *mockRepo) {
				r.listTopAgentUsageFn = func(_ context.Context, q usage.Query) ([]usage.BreakdownRow, error) {
					if q.StartDate == "" {
						t.Error("StartDate should be normalized")
					}
					if q.EndDate == "" {
						t.Error("EndDate should be normalized")
					}
					return nil, nil
				}
			},
			query: usage.Query{Range: "7d"},
			check: func(t *testing.T, rows []usage.BreakdownRow) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepo{}
			tt.setup(repo)
			uc := newTestUsecase(repo)
			rows, err := uc.TopAgents(context.Background(), tt.query)
			if tt.wantErr {
				if err == nil {
					t.Fatal("TopAgents() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("TopAgents() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, rows)
			}
		})
	}
}

func TestEvents(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*mockRepo)
		query   usage.Query
		wantErr bool
		check   func(t *testing.T, events []usage.TokenUsageEvent)
	}{
		{
			name: "returns_events",
			setup: func(r *mockRepo) {
				r.listModelUsageEventsFn = func(_ context.Context, _ usage.Query) ([]usage.TokenUsageEvent, error) {
					return []usage.TokenUsageEvent{
						{ID: "evt-1", ProviderCode: "openai", ModelAPIID: "gpt-4"},
						{ID: "evt-2", ProviderCode: "anthropic", ModelAPIID: "claude-3"},
					}, nil
				}
			},
			query: usage.Query{Range: "7d"},
			check: func(t *testing.T, events []usage.TokenUsageEvent) {
				if len(events) != 2 {
					t.Fatalf("len(events) = %d, want 2", len(events))
				}
				if events[0].ID != "evt-1" {
					t.Errorf("events[0].ID = %q, want %q", events[0].ID, "evt-1")
				}
				if events[1].ProviderCode != "anthropic" {
					t.Errorf("events[1].ProviderCode = %q, want %q", events[1].ProviderCode, "anthropic")
				}
			},
		},
		{
			name: "empty_result",
			setup: func(r *mockRepo) {
				r.listModelUsageEventsFn = func(_ context.Context, _ usage.Query) ([]usage.TokenUsageEvent, error) {
					return nil, nil
				}
			},
			query: usage.Query{Range: "30d"},
			check: func(t *testing.T, events []usage.TokenUsageEvent) {
				if len(events) != 0 {
					t.Errorf("len(events) = %d, want 0", len(events))
				}
			},
		},
		{
			name: "repo_error",
			setup: func(r *mockRepo) {
				r.listModelUsageEventsFn = func(_ context.Context, _ usage.Query) ([]usage.TokenUsageEvent, error) {
					return nil, errors.New("connection refused")
				}
			},
			query:   usage.Query{Range: "30d"},
			wantErr: true,
		},
		{
			name: "query_normalization_applied",
			setup: func(r *mockRepo) {
				r.listModelUsageEventsFn = func(_ context.Context, q usage.Query) ([]usage.TokenUsageEvent, error) {
					if q.StartDate == "" {
						t.Error("StartDate should be normalized")
					}
					if q.EndDate == "" {
						t.Error("EndDate should be normalized")
					}
					return nil, nil
				}
			},
			query: usage.Query{Range: "today"},
			check: func(t *testing.T, events []usage.TokenUsageEvent) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepo{}
			tt.setup(repo)
			uc := newTestUsecase(repo)
			events, err := uc.Events(context.Background(), tt.query)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Events() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Events() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, events)
			}
		})
	}
}

func TestQuotaDashboard(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*mockRepo)
		wantErr bool
		check   func(t *testing.T, dash usage.QuotaDashboard)
	}{
		{
			name: "no_quotas_returns_zero_dashboard",
			setup: func(r *mockRepo) {
				r.listActiveQuotasFn = func(_ context.Context) ([]usage.Quota, error) {
					return nil, nil
				}
			},
			check: func(t *testing.T, dash usage.QuotaDashboard) {
				if dash.ConfiguredCount != 0 {
					t.Errorf("ConfiguredCount = %d, want 0", dash.ConfiguredCount)
				}
				if dash.TotalCapMicroUSD != 0 {
					t.Errorf("TotalCapMicroUSD = %d, want 0", dash.TotalCapMicroUSD)
				}
				if dash.TotalSpentMicroUSD != 0 {
					t.Errorf("TotalSpentMicroUSD = %d, want 0", dash.TotalSpentMicroUSD)
				}
				if dash.MaxUtilization != 0 {
					t.Errorf("MaxUtilization = %f, want 0", dash.MaxUtilization)
				}
			},
		},
		{
			name: "single_quota_with_spent",
			setup: func(r *mockRepo) {
				r.listActiveQuotasFn = func(_ context.Context) ([]usage.Quota, error) {
					return []usage.Quota{
						{ScopeType: "agent", ScopeID: "agent-1", MonthlyMicroUSD: 100000, PeriodStart: "2025-03-01", PeriodEnd: "2025-03-31"},
					}, nil
				}
				r.batchSumScopeCostFn = func(_ context.Context, _ []usage.Quota) (map[string]int64, error) {
					return map[string]int64{"agent:agent-1": 50000}, nil
				}
			},
			check: func(t *testing.T, dash usage.QuotaDashboard) {
				if dash.ConfiguredCount != 1 {
					t.Errorf("ConfiguredCount = %d, want 1", dash.ConfiguredCount)
				}
				if dash.TotalCapMicroUSD != 100000 {
					t.Errorf("TotalCapMicroUSD = %d, want 100000", dash.TotalCapMicroUSD)
				}
				if dash.TotalSpentMicroUSD != 50000 {
					t.Errorf("TotalSpentMicroUSD = %d, want 50000", dash.TotalSpentMicroUSD)
				}
				if dash.MaxUtilization != 0.5 {
					t.Errorf("MaxUtilization = %f, want 0.5", dash.MaxUtilization)
				}
			},
		},
		{
			name: "multiple_quotas_max_utilization",
			setup: func(r *mockRepo) {
				r.listActiveQuotasFn = func(_ context.Context) ([]usage.Quota, error) {
					return []usage.Quota{
						{ScopeType: "agent", ScopeID: "agent-1", MonthlyMicroUSD: 100000, PeriodStart: "2025-03-01", PeriodEnd: "2025-03-31"},
						{ScopeType: "agent", ScopeID: "agent-2", MonthlyMicroUSD: 200000, PeriodStart: "2025-03-01", PeriodEnd: "2025-03-31"},
					}, nil
				}
				r.batchSumScopeCostFn = func(_ context.Context, _ []usage.Quota) (map[string]int64, error) {
					return map[string]int64{
						"agent:agent-1": 80000,
						"agent:agent-2": 60000,
					}, nil
				}
			},
			check: func(t *testing.T, dash usage.QuotaDashboard) {
				if dash.ConfiguredCount != 2 {
					t.Errorf("ConfiguredCount = %d, want 2", dash.ConfiguredCount)
				}
				if dash.TotalCapMicroUSD != 300000 {
					t.Errorf("TotalCapMicroUSD = %d, want 300000", dash.TotalCapMicroUSD)
				}
				if dash.TotalSpentMicroUSD != 140000 {
					t.Errorf("TotalSpentMicroUSD = %d, want 140000", dash.TotalSpentMicroUSD)
				}
				if dash.MaxUtilization != 0.8 {
					t.Errorf("MaxUtilization = %f, want 0.8", dash.MaxUtilization)
				}
			},
		},
		{
			name: "quota_zero_monthly_skipped",
			setup: func(r *mockRepo) {
				r.listActiveQuotasFn = func(_ context.Context) ([]usage.Quota, error) {
					return []usage.Quota{
						{ScopeType: "agent", ScopeID: "agent-1", MonthlyMicroUSD: 0, PeriodStart: "2025-03-01", PeriodEnd: "2025-03-31"},
					}, nil
				}
				r.batchSumScopeCostFn = func(_ context.Context, _ []usage.Quota) (map[string]int64, error) {
					return map[string]int64{}, nil
				}
			},
			check: func(t *testing.T, dash usage.QuotaDashboard) {
				if dash.ConfiguredCount != 0 {
					t.Errorf("ConfiguredCount = %d, want 0 (zero MonthlyMicroUSD skipped)", dash.ConfiguredCount)
				}
				if dash.TotalCapMicroUSD != 0 {
					t.Errorf("TotalCapMicroUSD = %d, want 0", dash.TotalCapMicroUSD)
				}
			},
		},
		{
			name: "batch_error_with_spent_in_map_still_counted",
			setup: func(r *mockRepo) {
				r.listActiveQuotasFn = func(_ context.Context) ([]usage.Quota, error) {
					return []usage.Quota{
						{ScopeType: "agent", ScopeID: "agent-1", MonthlyMicroUSD: 100000, PeriodStart: "2025-03-01", PeriodEnd: "2025-03-31"},
					}, nil
				}
				r.batchSumScopeCostFn = func(_ context.Context, _ []usage.Quota) (map[string]int64, error) {
					return map[string]int64{"agent:agent-1": 50000}, errors.New("partial failure")
				}
			},
			check: func(t *testing.T, dash usage.QuotaDashboard) {
				if dash.ConfiguredCount != 1 {
					t.Errorf("ConfiguredCount = %d, want 1 (spent in map despite batch error)", dash.ConfiguredCount)
				}
				if dash.TotalSpentMicroUSD != 50000 {
					t.Errorf("TotalSpentMicroUSD = %d, want 50000", dash.TotalSpentMicroUSD)
				}
			},
		},
		{
			name: "batch_error_without_spent_in_map_skipped",
			setup: func(r *mockRepo) {
				r.listActiveQuotasFn = func(_ context.Context) ([]usage.Quota, error) {
					return []usage.Quota{
						{ScopeType: "agent", ScopeID: "agent-1", MonthlyMicroUSD: 100000, PeriodStart: "2025-03-01", PeriodEnd: "2025-03-31"},
					}, nil
				}
				r.batchSumScopeCostFn = func(_ context.Context, _ []usage.Quota) (map[string]int64, error) {
					return map[string]int64{}, errors.New("total failure")
				}
			},
			check: func(t *testing.T, dash usage.QuotaDashboard) {
				if dash.ConfiguredCount != 0 {
					t.Errorf("ConfiguredCount = %d, want 0 (spent not in map and batch error)", dash.ConfiguredCount)
				}
			},
		},
		{
			name: "list_active_quotas_error",
			setup: func(r *mockRepo) {
				r.listActiveQuotasFn = func(_ context.Context) ([]usage.Quota, error) {
					return nil, errors.New("db down")
				}
			},
			wantErr: true,
		},
		{
			name: "negative_monthly_micro_usd_skipped",
			setup: func(r *mockRepo) {
				r.listActiveQuotasFn = func(_ context.Context) ([]usage.Quota, error) {
					return []usage.Quota{
						{ScopeType: "agent", ScopeID: "agent-1", MonthlyMicroUSD: -500, PeriodStart: "2025-03-01", PeriodEnd: "2025-03-31"},
					}, nil
				}
				r.batchSumScopeCostFn = func(_ context.Context, _ []usage.Quota) (map[string]int64, error) {
					return map[string]int64{}, nil
				}
			},
			check: func(t *testing.T, dash usage.QuotaDashboard) {
				if dash.ConfiguredCount != 0 {
					t.Errorf("ConfiguredCount = %d, want 0 (negative MonthlyMicroUSD skipped)", dash.ConfiguredCount)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepo{}
			tt.setup(repo)
			uc := newTestUsecase(repo)
			dash, err := uc.QuotaDashboard(context.Background())
			if tt.wantErr {
				if err == nil {
					t.Fatal("QuotaDashboard() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("QuotaDashboard() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, dash)
			}
		})
	}
}

func TestInefficientModels(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*mockRepo)
		query   usage.Query
		wantErr bool
		check   func(t *testing.T, insights []usage.ModelInsight)
	}{
		{
			name: "low_tps_flag",
			setup: func(r *mockRepo) {
				r.listTopModelUsageFn = func(_ context.Context, _ usage.Query) ([]usage.BreakdownRow, error) {
					return []usage.BreakdownRow{
						{ProviderCode: "openai", ModelAPIID: "gpt-4", ModelDisplayName: "GPT-4", CallCount: 5, TotalCostMicroUSD: 200000, AvgTokensPerSecond: 2.0, SuccessRate: 0.95},
					}, nil
				}
			},
			query: usage.Query{Range: "30d"},
			check: func(t *testing.T, insights []usage.ModelInsight) {
				if len(insights) != 1 {
					t.Fatalf("len(insights) = %d, want 1", len(insights))
				}
				if len(insights[0].Flags) != 1 || insights[0].Flags[0] != "low_tps" {
					t.Errorf("Flags = %v, want [low_tps]", insights[0].Flags)
				}
			},
		},
		{
			name: "high_failure_flag",
			setup: func(r *mockRepo) {
				r.listTopModelUsageFn = func(_ context.Context, _ usage.Query) ([]usage.BreakdownRow, error) {
					return []usage.BreakdownRow{
						{ProviderCode: "anthropic", ModelAPIID: "claude-3", ModelDisplayName: "Claude 3", CallCount: 10, TotalCostMicroUSD: 200000, AvgTokensPerSecond: 20.0, SuccessRate: 0.7},
					}, nil
				}
			},
			query: usage.Query{Range: "30d"},
			check: func(t *testing.T, insights []usage.ModelInsight) {
				if len(insights) != 1 {
					t.Fatalf("len(insights) = %d, want 1", len(insights))
				}
				if len(insights[0].Flags) != 1 || insights[0].Flags[0] != "high_failure" {
					t.Errorf("Flags = %v, want [high_failure]", insights[0].Flags)
				}
			},
		},
		{
			name: "high_cost_flag",
			setup: func(r *mockRepo) {
				r.listTopModelUsageFn = func(_ context.Context, _ usage.Query) ([]usage.BreakdownRow, error) {
					return []usage.BreakdownRow{
						{ProviderCode: "openai", ModelAPIID: "gpt-4", ModelDisplayName: "GPT-4", CallCount: 50, TotalCostMicroUSD: 1500000, AvgTokensPerSecond: 20.0, SuccessRate: 0.99},
					}, nil
				}
			},
			query: usage.Query{Range: "30d"},
			check: func(t *testing.T, insights []usage.ModelInsight) {
				if len(insights) != 1 {
					t.Fatalf("len(insights) = %d, want 1", len(insights))
				}
				found := false
				for _, f := range insights[0].Flags {
					if f == "high_cost" {
						found = true
					}
				}
				if !found {
					t.Errorf("Flags = %v, want high_cost included", insights[0].Flags)
				}
			},
		},
		{
			name: "multiple_flags",
			setup: func(r *mockRepo) {
				r.listTopModelUsageFn = func(_ context.Context, _ usage.Query) ([]usage.BreakdownRow, error) {
					return []usage.BreakdownRow{
						{ProviderCode: "openai", ModelAPIID: "gpt-4", ModelDisplayName: "GPT-4", CallCount: 10, TotalCostMicroUSD: 1500000, AvgTokensPerSecond: 2.0, SuccessRate: 0.7},
					}, nil
				}
			},
			query: usage.Query{Range: "30d"},
			check: func(t *testing.T, insights []usage.ModelInsight) {
				if len(insights) != 1 {
					t.Fatalf("len(insights) = %d, want 1", len(insights))
				}
				flagSet := map[string]bool{}
				for _, f := range insights[0].Flags {
					flagSet[f] = true
				}
				for _, want := range []string{"low_tps", "high_failure", "high_cost"} {
					if !flagSet[want] {
						t.Errorf("missing flag %q in %v", want, insights[0].Flags)
					}
				}
			},
		},
		{
			name: "below_min_calls_filtered",
			setup: func(r *mockRepo) {
				r.listTopModelUsageFn = func(_ context.Context, _ usage.Query) ([]usage.BreakdownRow, error) {
					return []usage.BreakdownRow{
						{ProviderCode: "openai", ModelAPIID: "gpt-4", CallCount: 2, TotalCostMicroUSD: 200000, AvgTokensPerSecond: 1.0, SuccessRate: 0.5},
					}, nil
				}
			},
			query: usage.Query{Range: "30d"},
			check: func(t *testing.T, insights []usage.ModelInsight) {
				if len(insights) != 0 {
					t.Errorf("len(insights) = %d, want 0 (below min calls)", len(insights))
				}
			},
		},
		{
			name: "below_cost_floor_filtered",
			setup: func(r *mockRepo) {
				r.listTopModelUsageFn = func(_ context.Context, _ usage.Query) ([]usage.BreakdownRow, error) {
					return []usage.BreakdownRow{
						{ProviderCode: "openai", ModelAPIID: "gpt-4", CallCount: 10, TotalCostMicroUSD: 50000, AvgTokensPerSecond: 1.0, SuccessRate: 0.5},
					}, nil
				}
			},
			query: usage.Query{Range: "30d"},
			check: func(t *testing.T, insights []usage.ModelInsight) {
				if len(insights) != 0 {
					t.Errorf("len(insights) = %d, want 0 (below cost floor)", len(insights))
				}
			},
		},
		{
			name: "no_flags_filtered",
			setup: func(r *mockRepo) {
				r.listTopModelUsageFn = func(_ context.Context, _ usage.Query) ([]usage.BreakdownRow, error) {
					return []usage.BreakdownRow{
						{ProviderCode: "openai", ModelAPIID: "gpt-4", CallCount: 10, TotalCostMicroUSD: 200000, AvgTokensPerSecond: 20.0, SuccessRate: 0.95},
					}, nil
				}
			},
			query: usage.Query{Range: "30d"},
			check: func(t *testing.T, insights []usage.ModelInsight) {
				if len(insights) != 0 {
					t.Errorf("len(insights) = %d, want 0 (no inefficient flags)", len(insights))
				}
			},
		},
		{
			name: "zero_tps_not_flagged",
			setup: func(r *mockRepo) {
				r.listTopModelUsageFn = func(_ context.Context, _ usage.Query) ([]usage.BreakdownRow, error) {
					return []usage.BreakdownRow{
						{ProviderCode: "openai", ModelAPIID: "gpt-4", CallCount: 5, TotalCostMicroUSD: 1500000, AvgTokensPerSecond: 0, SuccessRate: 0.95},
					}, nil
				}
			},
			query: usage.Query{Range: "30d"},
			check: func(t *testing.T, insights []usage.ModelInsight) {
				if len(insights) != 1 {
					t.Fatalf("len(insights) = %d, want 1", len(insights))
				}
				for _, f := range insights[0].Flags {
					if f == "low_tps" {
						t.Error("zero TPS should not be flagged as low_tps")
					}
				}
			},
		},
		{
			name: "zero_success_rate_not_flagged",
			setup: func(r *mockRepo) {
				r.listTopModelUsageFn = func(_ context.Context, _ usage.Query) ([]usage.BreakdownRow, error) {
					return []usage.BreakdownRow{
						{ProviderCode: "openai", ModelAPIID: "gpt-4", CallCount: 5, TotalCostMicroUSD: 1500000, AvgTokensPerSecond: 20.0, SuccessRate: 0},
					}, nil
				}
			},
			query: usage.Query{Range: "30d"},
			check: func(t *testing.T, insights []usage.ModelInsight) {
				if len(insights) != 1 {
					t.Fatalf("len(insights) = %d, want 1", len(insights))
				}
				for _, f := range insights[0].Flags {
					if f == "high_failure" {
						t.Error("zero SuccessRate should not be flagged as high_failure")
					}
				}
			},
		},
		{
			name: "max_8_results",
			setup: func(r *mockRepo) {
				var rows []usage.BreakdownRow
				for i := 0; i < 10; i++ {
					rows = append(rows, usage.BreakdownRow{
						ProviderCode:       "openai",
						ModelAPIID:         "gpt-4",
						ModelDisplayName:   "GPT-4",
						CallCount:          10,
						TotalCostMicroUSD:  1500000,
						AvgTokensPerSecond: 2.0,
						SuccessRate:        0.7,
					})
				}
				r.listTopModelUsageFn = func(_ context.Context, _ usage.Query) ([]usage.BreakdownRow, error) {
					return rows, nil
				}
			},
			query: usage.Query{Range: "30d"},
			check: func(t *testing.T, insights []usage.ModelInsight) {
				if len(insights) != 8 {
					t.Errorf("len(insights) = %d, want 8 (max cap)", len(insights))
				}
			},
		},
		{
			name: "model_display_name_trimmed",
			setup: func(r *mockRepo) {
				r.listTopModelUsageFn = func(_ context.Context, _ usage.Query) ([]usage.BreakdownRow, error) {
					return []usage.BreakdownRow{
						{ProviderCode: "openai", ModelAPIID: "gpt-4", ModelDisplayName: "  GPT-4  ", CallCount: 5, TotalCostMicroUSD: 200000, AvgTokensPerSecond: 2.0, SuccessRate: 0.95},
					}, nil
				}
			},
			query: usage.Query{Range: "30d"},
			check: func(t *testing.T, insights []usage.ModelInsight) {
				if len(insights) != 1 {
					t.Fatalf("len(insights) = %d, want 1", len(insights))
				}
				if insights[0].ModelDisplayName != "GPT-4" {
					t.Errorf("ModelDisplayName = %q, want %q", insights[0].ModelDisplayName, "GPT-4")
				}
			},
		},
		{
			name: "repo_error",
			setup: func(r *mockRepo) {
				r.listTopModelUsageFn = func(_ context.Context, _ usage.Query) ([]usage.BreakdownRow, error) {
					return nil, errors.New("db error")
				}
			},
			query:   usage.Query{Range: "30d"},
			wantErr: true,
		},
		{
			name: "empty_result",
			setup: func(r *mockRepo) {
				r.listTopModelUsageFn = func(_ context.Context, _ usage.Query) ([]usage.BreakdownRow, error) {
					return nil, nil
				}
			},
			query: usage.Query{Range: "30d"},
			check: func(t *testing.T, insights []usage.ModelInsight) {
				if len(insights) != 0 {
					t.Errorf("len(insights) = %d, want 0", len(insights))
				}
			},
		},
		{
			name: "query_limit_set_to_32",
			setup: func(r *mockRepo) {
				r.listTopModelUsageFn = func(_ context.Context, q usage.Query) ([]usage.BreakdownRow, error) {
					if q.Limit != 32 {
						t.Errorf("Limit = %d, want 32", q.Limit)
					}
					return nil, nil
				}
			},
			query: usage.Query{Range: "30d"},
			check: func(t *testing.T, insights []usage.ModelInsight) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepo{}
			tt.setup(repo)
			uc := newTestUsecase(repo)
			insights, err := uc.InefficientModels(context.Background(), tt.query)
			if tt.wantErr {
				if err == nil {
					t.Fatal("InefficientModels() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("InefficientModels() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, insights)
			}
		})
	}
}

func TestExportUsageEventsCSV(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*mockRepo)
		query   usage.Query
		wantErr bool
		check   func(t *testing.T, csv string)
	}{
		{
			name: "basic_csv_output",
			setup: func(r *mockRepo) {
				r.listModelUsageEventsFn = func(_ context.Context, _ usage.Query) ([]usage.TokenUsageEvent, error) {
					return []usage.TokenUsageEvent{
						{
							OccurredAt:       "2025-03-15T10:00:00Z",
							UsageKind:        "chat",
							AgentID:          "agent-1",
							ProviderCode:     "openai",
							ModelAPIID:       "gpt-4",
							SessionID:        "sess-1",
							TeamID:           "team-1",
							InputTokens:      100,
							OutputTokens:     200,
							TotalTokens:      300,
							TotalCostMicroUSD: 500,
							LatencyMS:        1200,
							Status:           "success",
							ErrorMessage:     "",
						},
					}, nil
				}
			},
			query: usage.Query{Range: "7d"},
			check: func(t *testing.T, csv string) {
				if !strings.HasPrefix(csv, "occurred_at,") {
					t.Errorf("missing CSV header, got: %q", csv[:50])
				}
				lines := strings.Split(strings.TrimSpace(csv), "\n")
				if len(lines) != 2 {
					t.Fatalf("expected 2 lines (header + 1 data), got %d", len(lines))
				}
				if !strings.Contains(lines[1], "100,200,300,500,1200") {
					t.Errorf("data line missing token/cost values: %q", lines[1])
				}
			},
		},
		{
			name: "empty_events_header_only",
			setup: func(r *mockRepo) {
				r.listModelUsageEventsFn = func(_ context.Context, _ usage.Query) ([]usage.TokenUsageEvent, error) {
					return nil, nil
				}
			},
			query: usage.Query{Range: "30d"},
			check: func(t *testing.T, csv string) {
				lines := strings.Split(strings.TrimSpace(csv), "\n")
				if len(lines) != 1 {
					t.Errorf("expected 1 line (header only), got %d", len(lines))
				}
				if !strings.HasPrefix(csv, "occurred_at,") {
					t.Errorf("missing CSV header, got: %q", csv[:50])
				}
			},
		},
		{
			name: "events_error",
			setup: func(r *mockRepo) {
				r.listModelUsageEventsFn = func(_ context.Context, _ usage.Query) ([]usage.TokenUsageEvent, error) {
					return nil, errors.New("db error")
				}
			},
			query:   usage.Query{Range: "30d"},
			wantErr: true,
		},
		{
			name: "limit_set_to_5000",
			setup: func(r *mockRepo) {
				r.listModelUsageEventsFn = func(_ context.Context, q usage.Query) ([]usage.TokenUsageEvent, error) {
					if q.Limit != 5000 {
						t.Errorf("Limit = %d, want 5000", q.Limit)
					}
					return nil, nil
				}
			},
			query: usage.Query{Range: "30d"},
			check: func(t *testing.T, csv string) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepo{}
			tt.setup(repo)
			uc := newTestUsecase(repo)
			csv, err := uc.ExportUsageEventsCSV(context.Background(), tt.query)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ExportUsageEventsCSV() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ExportUsageEventsCSV() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, csv)
			}
		})
	}
}

func TestNormalizeQuery(t *testing.T) {
	now := time.Date(2025, 3, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		query usage.Query
		check func(t *testing.T, result usage.Query)
	}{
		{
			name:  "both_dates_set_passthrough",
			query: usage.Query{StartDate: "2025-03-01", EndDate: "2025-03-15", Range: "7d"},
			check: func(t *testing.T, result usage.Query) {
				if result.StartDate != "2025-03-01" {
					t.Errorf("StartDate = %q, want %q", result.StartDate, "2025-03-01")
				}
				if result.EndDate != "2025-03-15" {
					t.Errorf("EndDate = %q, want %q", result.EndDate, "2025-03-15")
				}
			},
		},
		{
			name:  "range_today",
			query: usage.Query{Range: "today"},
			check: func(t *testing.T, result usage.Query) {
				if result.StartDate != "2025-03-15" {
					t.Errorf("StartDate = %q, want %q", result.StartDate, "2025-03-15")
				}
				if result.EndDate != "2025-03-15" {
					t.Errorf("EndDate = %q, want %q", result.EndDate, "2025-03-15")
				}
			},
		},
		{
			name:  "range_7d",
			query: usage.Query{Range: "7d"},
			check: func(t *testing.T, result usage.Query) {
				if result.StartDate != "2025-03-09" {
					t.Errorf("StartDate = %q, want %q", result.StartDate, "2025-03-09")
				}
				if result.EndDate != "2025-03-15" {
					t.Errorf("EndDate = %q, want %q", result.EndDate, "2025-03-15")
				}
			},
		},
		{
			name:  "range_month",
			query: usage.Query{Range: "month"},
			check: func(t *testing.T, result usage.Query) {
				if result.StartDate != "2025-03-01" {
					t.Errorf("StartDate = %q, want %q", result.StartDate, "2025-03-01")
				}
				if result.EndDate != "2025-03-15" {
					t.Errorf("EndDate = %q, want %q", result.EndDate, "2025-03-15")
				}
			},
		},
		{
			name:  "range_30d",
			query: usage.Query{Range: "30d"},
			check: func(t *testing.T, result usage.Query) {
				if result.StartDate != "2025-02-14" {
					t.Errorf("StartDate = %q, want %q", result.StartDate, "2025-02-14")
				}
				if result.EndDate != "2025-03-15" {
					t.Errorf("EndDate = %q, want %q", result.EndDate, "2025-03-15")
				}
			},
		},
		{
			name:  "range_empty_defaults_to_30d",
			query: usage.Query{Range: ""},
			check: func(t *testing.T, result usage.Query) {
				if result.StartDate != "2025-02-14" {
					t.Errorf("StartDate = %q, want %q", result.StartDate, "2025-02-14")
				}
				if result.EndDate != "2025-03-15" {
					t.Errorf("EndDate = %q, want %q", result.EndDate, "2025-03-15")
				}
			},
		},
		{
			name:  "range_unknown_defaults_to_30d",
			query: usage.Query{Range: "custom"},
			check: func(t *testing.T, result usage.Query) {
				if result.StartDate != "2025-02-14" {
					t.Errorf("StartDate = %q, want %q", result.StartDate, "2025-02-14")
				}
				if result.EndDate != "2025-03-15" {
					t.Errorf("EndDate = %q, want %q", result.EndDate, "2025-03-15")
				}
			},
		},
		{
			name:  "only_start_date_missing",
			query: usage.Query{EndDate: "2025-03-15", Range: "7d"},
			check: func(t *testing.T, result usage.Query) {
				if result.StartDate != "2025-03-09" {
					t.Errorf("StartDate = %q, want %q", result.StartDate, "2025-03-09")
				}
				if result.EndDate != "2025-03-15" {
					t.Errorf("EndDate = %q, want %q (should be preserved)", result.EndDate, "2025-03-15")
				}
			},
		},
		{
			name:  "only_end_date_missing",
			query: usage.Query{StartDate: "2025-03-01", Range: "7d"},
			check: func(t *testing.T, result usage.Query) {
				if result.StartDate != "2025-03-01" {
					t.Errorf("StartDate = %q, want %q (should be preserved)", result.StartDate, "2025-03-01")
				}
				if result.EndDate != "2025-03-15" {
					t.Errorf("EndDate = %q, want %q", result.EndDate, "2025-03-15")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepo{}
			uc := newTestUsecase(repo)
			result := usage.NormalizeQuery(uc, tt.query, now)
			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}

func TestAlertRecentlyFired(t *testing.T) {
	now := time.Date(2025, 3, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		setup func(u *usage.Usecase)
		alert usage.BudgetAlert
		want  bool
	}{
		{
			name:  "never_fired",
			setup: func(u *usage.Usecase) {},
			alert: usage.BudgetAlert{ID: "alert-1", LastFiredAt: ""},
			want:  false,
		},
		{
			name: "recently_fired_in_memory",
			setup: func(u *usage.Usecase) {
				usage.MarkAlertFired(u, "alert-1", now.Add(-30*time.Minute))
			},
			alert: usage.BudgetAlert{ID: "alert-1", LastFiredAt: ""},
			want:  true,
		},
		{
			name:  "recently_fired_last_fired_at",
			setup: func(u *usage.Usecase) {},
			alert: usage.BudgetAlert{ID: "alert-2", LastFiredAt: now.Add(-30 * time.Minute).Format(time.RFC3339)},
			want:  true,
		},
		{
			name: "cooldown_expired_in_memory",
			setup: func(u *usage.Usecase) {
				usage.MarkAlertFired(u, "alert-1", now.Add(-61*time.Minute))
			},
			alert: usage.BudgetAlert{ID: "alert-1", LastFiredAt: ""},
			want:  false,
		},
		{
			name:  "cooldown_expired_last_fired_at",
			setup: func(u *usage.Usecase) {},
			alert: usage.BudgetAlert{ID: "alert-2", LastFiredAt: now.Add(-61 * time.Minute).Format(time.RFC3339)},
			want:  false,
		},
		{
			name:  "invalid_last_fired_at_format",
			setup: func(u *usage.Usecase) {},
			alert: usage.BudgetAlert{ID: "alert-3", LastFiredAt: "not-a-valid-timestamp"},
			want:  false,
		},
		{
			name:  "exactly_at_cooldown_boundary",
			setup: func(u *usage.Usecase) {},
			alert: usage.BudgetAlert{ID: "alert-4", LastFiredAt: now.Add(-60 * time.Minute).Format(time.RFC3339)},
			want:  false,
		},
		{
			name:  "just_inside_cooldown",
			setup: func(u *usage.Usecase) {},
			alert: usage.BudgetAlert{ID: "alert-5", LastFiredAt: now.Add(-59 * time.Minute).Format(time.RFC3339)},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepo{}
			uc := newTestUsecase(repo)
			tt.setup(uc)
			got := usage.AlertRecentlyFired(uc, tt.alert, now)
			if got != tt.want {
				t.Errorf("AlertRecentlyFired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyPricingUSDToEvent(t *testing.T) {
	tests := []struct {
		name  string
		event usage.TokenUsageEvent
		snap  usage.ModelPricingSnapshot
		check func(t *testing.T, e usage.TokenUsageEvent)
	}{
		{
			name:  "usd_per_1m_takes_priority",
			event: usage.TokenUsageEvent{},
			snap: usage.ModelPricingSnapshot{
				InputPriceUSDPer1M:       3.0,
				InputPriceMicroUSDPer1K:  9999,
				OutputPriceUSDPer1M:      15.0,
				OutputPriceMicroUSDPer1K: 9999,
			},
			check: func(t *testing.T, e usage.TokenUsageEvent) {
				if e.InputPriceUSDPer1M != 3.0 {
					t.Errorf("InputPriceUSDPer1M = %f, want 3.0", e.InputPriceUSDPer1M)
				}
				if e.InputPriceMicroUSDPer1K != 3000 {
					t.Errorf("InputPriceMicroUSDPer1K = %d, want 3000 (from USD/1M conversion)", e.InputPriceMicroUSDPer1K)
				}
				if e.OutputPriceUSDPer1M != 15.0 {
					t.Errorf("OutputPriceUSDPer1M = %f, want 15.0", e.OutputPriceUSDPer1M)
				}
				if e.OutputPriceMicroUSDPer1K != 15000 {
					t.Errorf("OutputPriceMicroUSDPer1K = %d, want 15000 (from USD/1M conversion)", e.OutputPriceMicroUSDPer1K)
				}
			},
		},
		{
			name:  "micro_per_1k_fallback_when_usd_zero",
			event: usage.TokenUsageEvent{},
			snap: usage.ModelPricingSnapshot{
				InputPriceUSDPer1M:      0,
				InputPriceMicroUSDPer1K: 5000,
				OutputPriceUSDPer1M:     0,
				OutputPriceMicroUSDPer1K: 8000,
			},
			check: func(t *testing.T, e usage.TokenUsageEvent) {
				if e.InputPriceMicroUSDPer1K != 5000 {
					t.Errorf("InputPriceMicroUSDPer1K = %d, want 5000 (fallback)", e.InputPriceMicroUSDPer1K)
				}
				if e.OutputPriceMicroUSDPer1K != 8000 {
					t.Errorf("OutputPriceMicroUSDPer1K = %d, want 8000 (fallback)", e.OutputPriceMicroUSDPer1K)
				}
			},
		},
		{
			name:  "all_zero_prices",
			event: usage.TokenUsageEvent{},
			snap:  usage.ModelPricingSnapshot{},
			check: func(t *testing.T, e usage.TokenUsageEvent) {
				if e.InputPriceMicroUSDPer1K != 0 {
					t.Errorf("InputPriceMicroUSDPer1K = %d, want 0", e.InputPriceMicroUSDPer1K)
				}
				if e.OutputPriceMicroUSDPer1K != 0 {
					t.Errorf("OutputPriceMicroUSDPer1K = %d, want 0", e.OutputPriceMicroUSDPer1K)
				}
				if e.CachedInputPriceMicroUSDPer1K != 0 {
					t.Errorf("CachedInputPriceMicroUSDPer1K = %d, want 0", e.CachedInputPriceMicroUSDPer1K)
				}
			},
		},
		{
			name:  "all_six_price_kinds_from_usd_per_1m",
			event: usage.TokenUsageEvent{},
			snap: usage.ModelPricingSnapshot{
				InputPriceUSDPer1M:       2.0,
				OutputPriceUSDPer1M:      4.0,
				CacheReadPriceUSDPer1M:   1.0,
				CacheWritePriceUSDPer1M:  5.0,
				ReasoningPriceUSDPer1M:   6.0,
				EmbeddingPriceUSDPer1M:   0.5,
			},
			check: func(t *testing.T, e usage.TokenUsageEvent) {
				if e.InputPriceUSDPer1M != 2.0 {
					t.Errorf("InputPriceUSDPer1M = %f, want 2.0", e.InputPriceUSDPer1M)
				}
				if e.OutputPriceUSDPer1M != 4.0 {
					t.Errorf("OutputPriceUSDPer1M = %f, want 4.0", e.OutputPriceUSDPer1M)
				}
				if e.CacheReadPriceUSDPer1M != 1.0 {
					t.Errorf("CacheReadPriceUSDPer1M = %f, want 1.0", e.CacheReadPriceUSDPer1M)
				}
				if e.CacheWritePriceUSDPer1M != 5.0 {
					t.Errorf("CacheWritePriceUSDPer1M = %f, want 5.0", e.CacheWritePriceUSDPer1M)
				}
				if e.ReasoningPriceUSDPer1M != 6.0 {
					t.Errorf("ReasoningPriceUSDPer1M = %f, want 6.0", e.ReasoningPriceUSDPer1M)
				}
				if e.EmbeddingPriceUSDPer1M != 0.5 {
					t.Errorf("EmbeddingPriceUSDPer1M = %f, want 0.5", e.EmbeddingPriceUSDPer1M)
				}
				if e.InputPriceMicroUSDPer1K != 2000 {
					t.Errorf("InputPriceMicroUSDPer1K = %d, want 2000", e.InputPriceMicroUSDPer1K)
				}
				if e.OutputPriceMicroUSDPer1K != 4000 {
					t.Errorf("OutputPriceMicroUSDPer1K = %d, want 4000", e.OutputPriceMicroUSDPer1K)
				}
				if e.CachedInputPriceMicroUSDPer1K != 1000 {
					t.Errorf("CachedInputPriceMicroUSDPer1K = %d, want 1000", e.CachedInputPriceMicroUSDPer1K)
				}
				if e.CacheWritePriceMicroUSDPer1K != 5000 {
					t.Errorf("CacheWritePriceMicroUSDPer1K = %d, want 5000", e.CacheWritePriceMicroUSDPer1K)
				}
				if e.ReasoningPriceMicroUSDPer1K != 6000 {
					t.Errorf("ReasoningPriceMicroUSDPer1K = %d, want 6000", e.ReasoningPriceMicroUSDPer1K)
				}
				if e.EmbeddingPriceMicroUSDPer1K != 500 {
					t.Errorf("EmbeddingPriceMicroUSDPer1K = %d, want 500", e.EmbeddingPriceMicroUSDPer1K)
				}
			},
		},
		{
			name:  "mixed_usd_and_micro_fallback",
			event: usage.TokenUsageEvent{},
			snap: usage.ModelPricingSnapshot{
				InputPriceUSDPer1M:       3.0,
				OutputPriceUSDPer1M:      0,
				OutputPriceMicroUSDPer1K: 7000,
				CacheReadPriceUSDPer1M:   0,
				CachedInputPriceMicroUSDPer1K: 2000,
			},
			check: func(t *testing.T, e usage.TokenUsageEvent) {
				if e.InputPriceMicroUSDPer1K != 3000 {
					t.Errorf("InputPriceMicroUSDPer1K = %d, want 3000 (from USD/1M)", e.InputPriceMicroUSDPer1K)
				}
				if e.OutputPriceMicroUSDPer1K != 7000 {
					t.Errorf("OutputPriceMicroUSDPer1K = %d, want 7000 (micro fallback)", e.OutputPriceMicroUSDPer1K)
				}
				if e.CachedInputPriceMicroUSDPer1K != 2000 {
					t.Errorf("CachedInputPriceMicroUSDPer1K = %d, want 2000 (micro fallback)", e.CachedInputPriceMicroUSDPer1K)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := tt.event
			usage.ApplyPricingUSDToEvent(&e, tt.snap)
			if tt.check != nil {
				tt.check(t, e)
			}
		})
	}
}

func TestApplyPricingUSDToEvent_NilEvent(t *testing.T) {
	usage.ApplyPricingUSDToEvent(nil, usage.ModelPricingSnapshot{InputPriceUSDPer1M: 3.0})
}

func TestCheckQuota_EmptyScope(t *testing.T) {
	tests := []struct {
		name      string
		scopeType string
		scopeID   string
		wantErr   bool
		wantCode  int32
		wantReason string
	}{
		{
			name:      "empty_scope_type",
			scopeType: "",
			scopeID:   "agent-1",
			wantErr:   true,
			wantCode:  400,
			wantReason: "USAGE_QUOTA",
		},
		{
			name:      "empty_scope_id",
			scopeType: "agent",
			scopeID:   "",
			wantErr:   true,
			wantCode:  400,
			wantReason: "USAGE_QUOTA",
		},
		{
			name:      "whitespace_scope_type",
			scopeType: "  ",
			scopeID:   "agent-1",
			wantErr:   true,
			wantCode:  400,
			wantReason: "USAGE_QUOTA",
		},
		{
			name:      "whitespace_scope_id",
			scopeType: "agent",
			scopeID:   "  ",
			wantErr:   true,
			wantCode:  400,
			wantReason: "USAGE_QUOTA",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepo{}
			uc := newTestUsecase(repo)
			_, err := uc.CheckQuota(context.Background(), tt.scopeType, tt.scopeID)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			se := kerrors.FromError(err)
			if se == nil {
				t.Fatalf("expected kratos error, got %T", err)
			}
			if se.Code != tt.wantCode {
				t.Errorf("code = %d, want %d", se.Code, tt.wantCode)
			}
			if se.Reason != tt.wantReason {
				t.Errorf("reason = %q, want %q", se.Reason, tt.wantReason)
			}
		})
	}
}

func TestCheckQuota_DisabledQuota(t *testing.T) {
	repo := &mockRepo{}
	repo.getQuotaFn = func(_ context.Context, _, _ string) (usage.Quota, error) {
		return usage.Quota{
			ScopeType:       "agent",
			ScopeID:         "agent-1",
			MonthlyMicroUSD: 0,
			PeriodStart:     "2025-03-01",
			PeriodEnd:       "2025-03-31",
		}, nil
	}
	uc := newTestUsecase(repo)
	qc, err := uc.CheckQuota(context.Background(), "agent", "agent-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !qc.Allowed {
		t.Error("Allowed = false, want true for disabled quota")
	}
	if qc.Reason != "quota disabled" {
		t.Errorf("Reason = %q, want %q", qc.Reason, "quota disabled")
	}
}

func TestCheckQuota_NoQuotaConfigured(t *testing.T) {
	repo := &mockRepo{}
	repo.getQuotaFn = func(_ context.Context, _, _ string) (usage.Quota, error) {
		return usage.Quota{}, shared.ErrQuotaNotFound
	}
	uc := newTestUsecase(repo)
	qc, err := uc.CheckQuota(context.Background(), "agent", "agent-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !qc.Allowed {
		t.Error("Allowed = false, want true when no quota configured")
	}
	if qc.Reason != "no quota configured" {
		t.Errorf("Reason = %q, want %q", qc.Reason, "no quota configured")
	}
}
