package usage

import (
	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
	"context"
	"testing"
)

func TestGetQuota(t *testing.T) {
	tests := []struct {
		name       string
		scopeType  string
		scopeID    string
		setup      func(*mockUsageRepo)
		wantErr    bool
		wantReason string
		wantCode   apierror.Code
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
			wantCode:   apierror.CodeNotFound,
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
				se, ok := apierror.From(err)
				if !ok {
					t.Fatalf("expected apierror, got %T", err)
				}
				if se.Domain != tt.wantReason {
					t.Errorf("domain = %q, want %q", se.Domain, tt.wantReason)
				}
				if se.Code != tt.wantCode {
					t.Errorf("code = %s, want %s", se.Code, tt.wantCode)
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
