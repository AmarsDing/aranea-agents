package usage

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
)

func TestRecordTokenUsageEvent(t *testing.T) {
	tests := []struct {
		name    string
		input   TokenUsageEvent
		setup   func(*mockUsageRepo)
		wantErr bool
		check   func(t *testing.T, got TokenUsageEvent)
	}{
		{
			name: "valid_with_normalization",
			input: TokenUsageEvent{
				ID:           "evt-1",
				ProviderCode: "openai",
				ModelAPIID:   "gpt-4",
				InputTokens:  100,
				OutputTokens: 50,
				TotalTokens:  150,
				UsageKind:    "",
				CallCount:    0,
				Status:       "",
				AgentID:      "agent-1",
			},
			setup: func(r *mockUsageRepo) {
				r.recordTokenUsageEventFn = func(_ context.Context, e TokenUsageEvent) (TokenUsageEvent, error) {
					return e, nil
				}
			},
			check: func(t *testing.T, got TokenUsageEvent) {
				if got.UsageKind != "chat" {
					t.Fatalf("expected UsageKind 'chat', got %q", got.UsageKind)
				}
				if got.CallCount != 1 {
					t.Fatalf("expected CallCount 1, got %d", got.CallCount)
				}
				if got.Status != "success" {
					t.Fatalf("expected Status 'success', got %q", got.Status)
				}
				if got.OccurredAt == "" {
					t.Fatal("expected OccurredAt to be filled")
				}
				if got.CreatedAt == "" {
					t.Fatal("expected CreatedAt to be filled")
				}
				if got.DateKey == "" {
					t.Fatal("expected DateKey to be filled")
				}
				if got.HourKey == "" {
					t.Fatal("expected HourKey to be filled")
				}
			},
		},
		{
			name: "empty_id_returns_error",
			input: TokenUsageEvent{
				ID: "",
			},
			wantErr: true,
			check:   func(t *testing.T, _ TokenUsageEvent) {},
		},
		{
			name: "whitespace_id_returns_error",
			input: TokenUsageEvent{
				ID: "   ",
			},
			wantErr: true,
			check:   func(t *testing.T, _ TokenUsageEvent) {},
		},
		{
			name: "repo_error_propagation",
			input: TokenUsageEvent{
				ID: "evt-1",
			},
			setup: func(r *mockUsageRepo) {
				r.recordTokenUsageEventFn = func(_ context.Context, _ TokenUsageEvent) (TokenUsageEvent, error) {
					return TokenUsageEvent{}, errors.New("db write failed")
				}
			},
			wantErr: true,
			check:   func(t *testing.T, _ TokenUsageEvent) {},
		},
		{
			name: "status_normalization_ok_to_success",
			input: TokenUsageEvent{
				ID:     "evt-2",
				Status: "ok",
			},
			setup: func(r *mockUsageRepo) {
				r.recordTokenUsageEventFn = func(_ context.Context, e TokenUsageEvent) (TokenUsageEvent, error) {
					return e, nil
				}
			},
			check: func(t *testing.T, got TokenUsageEvent) {
				if got.Status != "success" {
					t.Fatalf("expected Status 'success', got %q", got.Status)
				}
			},
		},
		{
			name: "status_normalization_fail_to_failed",
			input: TokenUsageEvent{
				ID:     "evt-3",
				Status: "fail",
			},
			setup: func(r *mockUsageRepo) {
				r.recordTokenUsageEventFn = func(_ context.Context, e TokenUsageEvent) (TokenUsageEvent, error) {
					return e, nil
				}
			},
			check: func(t *testing.T, got TokenUsageEvent) {
				if got.Status != "failed" {
					t.Fatalf("expected Status 'failed', got %q", got.Status)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUsageRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := newTestUsecase(repo)
			got, err := uc.RecordTokenUsageEvent(context.Background(), tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestSetQuota(t *testing.T) {
	tests := []struct {
		name       string
		input      Quota
		setup      func(*mockUsageRepo)
		wantErr    bool
		wantReason string
		wantCode   apierror.Code
		check      func(t *testing.T, got Quota)
	}{
		{
			name: "valid_set",
			input: Quota{
				ScopeType:       "agent",
				ScopeID:         "agent-1",
				MonthlyMicroUSD: 50000,
			},
			setup: func(r *mockUsageRepo) {
				r.setQuotaFn = func(_ context.Context, q Quota) (Quota, error) {
					return q, nil
				}
			},
			check: func(t *testing.T, got Quota) {
				if got.ScopeType != "agent" {
					t.Fatalf("expected ScopeType 'agent', got %q", got.ScopeType)
				}
				if got.MonthlyMicroUSD != 50000 {
					t.Fatalf("expected MonthlyMicroUSD 50000, got %d", got.MonthlyMicroUSD)
				}
			},
		},
		{
			name: "empty_scope_type_returns_error",
			input: Quota{
				ScopeType:       "",
				ScopeID:         "agent-1",
				MonthlyMicroUSD: 50000,
			},
			wantErr:    true,
			wantReason: "USAGE_QUOTA",
			wantCode:   apierror.CodeBadRequest,
			check:      func(t *testing.T, _ Quota) {},
		},
		{
			name: "empty_scope_id_returns_error",
			input: Quota{
				ScopeType:       "agent",
				ScopeID:         "",
				MonthlyMicroUSD: 50000,
			},
			wantErr:    true,
			wantReason: "USAGE_QUOTA",
			wantCode:   apierror.CodeBadRequest,
			check:      func(t *testing.T, _ Quota) {},
		},
		{
			name: "negative_monthly_micro_usd_returns_error",
			input: Quota{
				ScopeType:       "agent",
				ScopeID:         "agent-1",
				MonthlyMicroUSD: -1,
			},
			wantErr:    true,
			wantReason: "USAGE_QUOTA",
			wantCode:   apierror.CodeBadRequest,
			check:      func(t *testing.T, _ Quota) {},
		},
		{
			name: "repo_scope_required_maps_to_bad_request",
			input: Quota{
				ScopeType:       "agent",
				ScopeID:         "agent-1",
				MonthlyMicroUSD: 100,
			},
			setup: func(r *mockUsageRepo) {
				r.setQuotaFn = func(_ context.Context, _ Quota) (Quota, error) {
					return Quota{}, shared.ErrUsageScopeRequired
				}
			},
			wantErr:    true,
			wantReason: "USAGE",
			wantCode:   apierror.CodeBadRequest,
			check:      func(t *testing.T, _ Quota) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUsageRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := newTestUsecase(repo)
			got, err := uc.SetQuota(context.Background(), tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantReason != "" {
					se, ok := apierror.From(err)
					if !ok {
						t.Fatalf("expected apierror, got %T", err)
					}
					if se.Domain != tt.wantReason {
						t.Fatalf("expected domain %q, got %q", tt.wantReason, se.Domain)
					}
					if se.Code != tt.wantCode {
						t.Fatalf("expected code %s, got %s", tt.wantCode, se.Code)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestListBudgetAlerts(t *testing.T) {
	tests := []struct {
		name      string
		scopeType string
		scopeID   string
		setup     func(*mockUsageRepo)
		wantErr   bool
		check     func(t *testing.T, alerts []BudgetAlert)
	}{
		{
			name:      "returns_alerts_from_repo",
			scopeType: "agent",
			scopeID:   "agent-1",
			setup: func(r *mockUsageRepo) {
				r.listBudgetAlertsFn = func(_ context.Context, _, _ string) ([]BudgetAlert, error) {
					return []BudgetAlert{
						{ID: "ba-1", ScopeType: "agent", ScopeID: "agent-1", AlertRatio: 0.8, Enabled: true},
						{ID: "ba-2", ScopeType: "agent", ScopeID: "agent-1", AlertRatio: 0.95, Enabled: true},
					}, nil
				}
			},
			check: func(t *testing.T, alerts []BudgetAlert) {
				if len(alerts) != 2 {
					t.Fatalf("expected 2 alerts, got %d", len(alerts))
				}
				if alerts[0].AlertRatio != 0.8 {
					t.Fatalf("expected AlertRatio 0.8, got %v", alerts[0].AlertRatio)
				}
				if alerts[1].ID != "ba-2" {
					t.Fatalf("expected ID 'ba-2', got %q", alerts[1].ID)
				}
			},
		},
		{
			name:      "empty_list",
			scopeType: "agent",
			scopeID:   "agent-none",
			setup: func(r *mockUsageRepo) {
				r.listBudgetAlertsFn = func(_ context.Context, _, _ string) ([]BudgetAlert, error) {
					return nil, nil
				}
			},
			check: func(t *testing.T, alerts []BudgetAlert) {
				if len(alerts) != 0 {
					t.Fatalf("expected 0 alerts, got %d", len(alerts))
				}
			},
		},
		{
			name:      "repo_error",
			scopeType: "agent",
			scopeID:   "agent-1",
			setup: func(r *mockUsageRepo) {
				r.listBudgetAlertsFn = func(_ context.Context, _, _ string) ([]BudgetAlert, error) {
					return nil, errors.New("db error")
				}
			},
			wantErr: true,
			check:   func(t *testing.T, _ []BudgetAlert) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUsageRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := newTestUsecase(repo)
			alerts, err := uc.ListBudgetAlerts(context.Background(), tt.scopeType, tt.scopeID)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, alerts)
			}
		})
	}
}

func TestSetBudgetAlert(t *testing.T) {
	tests := []struct {
		name       string
		input      BudgetAlert
		setup      func(*mockUsageRepo)
		wantErr    bool
		wantReason string
		wantCode   apierror.Code
		check      func(t *testing.T, got BudgetAlert)
	}{
		{
			name: "valid_set",
			input: BudgetAlert{
				ScopeType:  "agent",
				ScopeID:    "agent-1",
				AlertRatio: 0.8,
				Enabled:    true,
			},
			setup: func(r *mockUsageRepo) {
				r.setBudgetAlertFn = func(_ context.Context, a BudgetAlert) (BudgetAlert, error) {
					a.ID = "ba-new"
					return a, nil
				}
			},
			check: func(t *testing.T, got BudgetAlert) {
				if got.ScopeType != "agent" {
					t.Fatalf("expected ScopeType 'agent', got %q", got.ScopeType)
				}
				if got.AlertRatio != 0.8 {
					t.Fatalf("expected AlertRatio 0.8, got %v", got.AlertRatio)
				}
			},
		},
		{
			name: "empty_scope_type_returns_error",
			input: BudgetAlert{
				ScopeType:  "",
				ScopeID:    "agent-1",
				AlertRatio: 0.8,
			},
			wantErr:    true,
			wantReason: "USAGE_ALERT",
			wantCode:   apierror.CodeBadRequest,
			check:      func(t *testing.T, _ BudgetAlert) {},
		},
		{
			name: "empty_scope_id_returns_error",
			input: BudgetAlert{
				ScopeType:  "agent",
				ScopeID:    "",
				AlertRatio: 0.8,
			},
			wantErr:    true,
			wantReason: "USAGE_ALERT",
			wantCode:   apierror.CodeBadRequest,
			check:      func(t *testing.T, _ BudgetAlert) {},
		},
		{
			name: "zero_alert_ratio_returns_error",
			input: BudgetAlert{
				ScopeType:  "agent",
				ScopeID:    "agent-1",
				AlertRatio: 0,
			},
			wantErr:    true,
			wantReason: "USAGE_ALERT",
			wantCode:   apierror.CodeBadRequest,
			check:      func(t *testing.T, _ BudgetAlert) {},
		},
		{
			name: "negative_alert_ratio_returns_error",
			input: BudgetAlert{
				ScopeType:  "agent",
				ScopeID:    "agent-1",
				AlertRatio: -0.5,
			},
			wantErr:    true,
			wantReason: "USAGE_ALERT",
			wantCode:   apierror.CodeBadRequest,
			check:      func(t *testing.T, _ BudgetAlert) {},
		},
		{
			name: "alert_ratio_above_one_returns_error",
			input: BudgetAlert{
				ScopeType:  "agent",
				ScopeID:    "agent-1",
				AlertRatio: 1.5,
			},
			wantErr:    true,
			wantReason: "USAGE_ALERT",
			wantCode:   apierror.CodeBadRequest,
			check:      func(t *testing.T, _ BudgetAlert) {},
		},
		{
			name: "ratio_exactly_one_is_valid",
			input: BudgetAlert{
				ScopeType:  "agent",
				ScopeID:    "agent-1",
				AlertRatio: 1.0,
			},
			setup: func(r *mockUsageRepo) {
				r.setBudgetAlertFn = func(_ context.Context, a BudgetAlert) (BudgetAlert, error) {
					return a, nil
				}
			},
			check: func(t *testing.T, got BudgetAlert) {
				if got.AlertRatio != 1.0 {
					t.Fatalf("expected AlertRatio 1.0, got %v", got.AlertRatio)
				}
			},
		},
		{
			name: "repo_budget_alert_not_found_maps_to_not_found",
			input: BudgetAlert{
				ScopeType:  "agent",
				ScopeID:    "agent-1",
				AlertRatio: 0.8,
			},
			setup: func(r *mockUsageRepo) {
				r.setBudgetAlertFn = func(_ context.Context, _ BudgetAlert) (BudgetAlert, error) {
					return BudgetAlert{}, shared.ErrBudgetAlertNotFound
				}
			},
			wantErr:    true,
			wantReason: "USAGE_ALERT",
			wantCode:   apierror.CodeNotFound,
			check:      func(t *testing.T, _ BudgetAlert) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUsageRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := newTestUsecase(repo)
			got, err := uc.SetBudgetAlert(context.Background(), tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantReason != "" {
					se, ok := apierror.From(err)
					if !ok {
						t.Fatalf("expected apierror, got %T", err)
					}
					if se.Domain != tt.wantReason {
						t.Fatalf("expected domain %q, got %q", tt.wantReason, se.Domain)
					}
					if se.Code != tt.wantCode {
						t.Fatalf("expected code %s, got %s", tt.wantCode, se.Code)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}
