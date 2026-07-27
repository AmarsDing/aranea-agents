package service

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/usage/v1"
	"aranea-agents/internal/biz/usage"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/auth"
)

// stubUsageRepo implements usage.Repo with only GetQuota functional;
// all other methods panic (embedded nil interfaces).
type stubUsageRepo struct {
	usage.AnalyticsRepo
	usage.WriteRepo
	usage.QuotaRepo
}

func (stubUsageRepo) GetQuota(_ context.Context, scopeType, scopeID string) (usage.Quota, error) {
	return usage.Quota{
		ID:              "quota-1",
		ScopeType:       scopeType,
		ScopeID:         scopeID,
		MonthlyMicroUSD: 1000000,
		PeriodStart:     "2026-07-01",
		PeriodEnd:       "2026-07-31",
	}, nil
}

func newTestUsageService() *UsageService {
	uc := usage.NewUsecase(stubUsageRepo{}, nil)
	return NewUsageService(uc)
}

// TestUsageService_GetUsageQuota_RejectsNonSystemNonAdmin verifies that
// a caller without system workspace and without admin access is rejected.
func TestUsageService_GetUsageQuota_RejectsNonSystemNonAdmin(t *testing.T) {
	svc := newTestUsageService()

	// Context with a regular workspace (not system) and no auth
	ctx := workspace.WithContext(context.Background(), "ws-regular")

	_, err := svc.GetUsageQuota(ctx, &v1.GetUsageQuotaRequest{
		ScopeType: "global",
		ScopeId:   "global",
	})
	if err == nil {
		t.Fatal("expected Forbidden error for non-system non-admin caller")
	}
}

// TestUsageService_GetUsageQuota_AllowsSystem verifies that
// a system workspace caller is allowed.
func TestUsageService_GetUsageQuota_AllowsSystem(t *testing.T) {
	svc := newTestUsageService()

	ctx := workspace.WithSystemWorkspace(context.Background())

	q, err := svc.GetUsageQuota(ctx, &v1.GetUsageQuotaRequest{
		ScopeType: "global",
		ScopeId:   "global",
	})
	if err != nil {
		t.Fatalf("system caller should be allowed: %v", err)
	}
	if q.GetScopeType() != "global" {
		t.Errorf("unexpected scope type: %s", q.GetScopeType())
	}
}

// TestUsageService_GetUsageQuota_AllowsAdmin verifies that
// a caller with admin access (but not system workspace) is allowed.
// This is the RED test: currently assertSystemCaller only checks
// workspace.IsSystem, so admin callers get Forbidden.
func TestUsageService_GetUsageQuota_AllowsAdmin(t *testing.T) {
	svc := newTestUsageService()

	// Admin user in a regular workspace
	adminAuth := &auth.Auth{UserID: 1, Access: "admin"}
	ctx := auth.NewContext(workspace.WithContext(context.Background(), "ws-regular"), adminAuth)

	q, err := svc.GetUsageQuota(ctx, &v1.GetUsageQuotaRequest{
		ScopeType: "global",
		ScopeId:   "global",
	})
	if err != nil {
		t.Fatalf("admin caller should be allowed: %v", err)
	}
	if q.GetScopeType() != "global" {
		t.Errorf("unexpected scope type: %s", q.GetScopeType())
	}
}

// TestUsageService_GetUsageQuota_RejectsNonAdminAuth verifies that
// a non-admin authenticated user is still rejected.
func TestUsageService_GetUsageQuota_RejectsNonAdminAuth(t *testing.T) {
	svc := newTestUsageService()

	userAuth := &auth.Auth{UserID: 2, Access: "user"}
	ctx := auth.NewContext(workspace.WithContext(context.Background(), "ws-regular"), userAuth)

	_, err := svc.GetUsageQuota(ctx, &v1.GetUsageQuotaRequest{
		ScopeType: "global",
		ScopeId:   "global",
	})
	if err == nil {
		t.Fatal("expected Forbidden error for non-admin authenticated user")
	}
}