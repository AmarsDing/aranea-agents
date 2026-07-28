package data

import (
	"context"
	"testing"
	"time"

	biza2a "aranea-agents/internal/biz/a2a"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

func setupFederationTestRepo(t *testing.T) *A2AFederationRepo {
	t.Helper()
	client, _ := testhelper.SetupTestPG(t)
	d := newDataFromClient(client, loggateway.NewNoop())
	return NewA2AFederationRepo(d, loggateway.NewNoop())
}

func TestA2AFederationRepo_OrgUpsertPreservesIdentity(t *testing.T) {
	repo := setupFederationTestRepo(t)
	ctx := context.Background()

	first, err := repo.UpsertOrg(ctx, biza2a.FederationOrg{
		Name:          "Acme",
		Domain:        "acme.example.com",
		PublicBaseURL: "https://a2a.acme.example.com",
		AuthType:      "api_key",
	})
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if first.ID == "" {
		t.Fatal("expected generated id")
	}
	if first.TrustLevel != biza2a.TrustLevelNeutral {
		t.Fatalf("default trust = %q, want neutral", first.TrustLevel)
	}
	if first.Status != biza2a.OrgStatusActive {
		t.Fatalf("default status = %q, want active", first.Status)
	}

	// Re-register the same domain: identity + joined_at preserved, mutable fields updated.
	time.Sleep(10 * time.Millisecond)
	second, err := repo.UpsertOrg(ctx, biza2a.FederationOrg{
		Name:          "Acme Renamed",
		Domain:        "acme.example.com",
		PublicBaseURL: "https://a2a2.acme.example.com",
		TrustLevel:    biza2a.TrustLevelTrusted,
	})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("re-upsert changed id: %q -> %q", first.ID, second.ID)
	}
	if !second.JoinedAt.Equal(first.JoinedAt) {
		t.Fatalf("re-upsert changed joined_at: %v -> %v", first.JoinedAt, second.JoinedAt)
	}
	if second.Name != "Acme Renamed" || second.TrustLevel != biza2a.TrustLevelTrusted {
		t.Fatalf("mutable fields not updated: %+v", second)
	}
}

func TestA2AFederationRepo_OrgGetListTrustDelete(t *testing.T) {
	repo := setupFederationTestRepo(t)
	ctx := context.Background()

	a, err := repo.UpsertOrg(ctx, biza2a.FederationOrg{Name: "A", Domain: "a.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpsertOrg(ctx, biza2a.FederationOrg{Name: "B", Domain: "b.example.com"}); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetOrg(ctx, a.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Domain != "a.example.com" {
		t.Fatalf("get domain = %q", got.Domain)
	}

	orgs, err := repo.ListOrgs(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(orgs) != 2 {
		t.Fatalf("list len = %d, want 2", len(orgs))
	}

	if err := repo.UpdateOrgTrust(ctx, a.ID, biza2a.TrustLevelUntrusted); err != nil {
		t.Fatalf("update trust: %v", err)
	}
	got, err = repo.GetOrg(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TrustLevel != biza2a.TrustLevelUntrusted {
		t.Fatalf("trust = %q, want untrusted", got.TrustLevel)
	}

	if err := repo.DeleteOrg(ctx, a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.GetOrg(ctx, a.ID); err == nil {
		t.Fatal("expected not-found after delete")
	}
}

func TestA2AFederationRepo_PolicyUpsertByOrgPair(t *testing.T) {
	repo := setupFederationTestRepo(t)
	ctx := context.Background()

	first, err := repo.UpsertPolicy(ctx, biza2a.FederationPolicy{
		CallerOrgID: biza2a.FederationLocalOrgID,
		CalleeOrgID: "org-1",
		MaxPerMin:   60,
		DailyQuota:  1000,
	})
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if first.Action != biza2a.PolicyActionAllow {
		t.Fatalf("default action = %q, want allow", first.Action)
	}

	// Same org pair: updates in place, id preserved.
	second, err := repo.UpsertPolicy(ctx, biza2a.FederationPolicy{
		CallerOrgID: biza2a.FederationLocalOrgID,
		CalleeOrgID: "org-1",
		Action:      biza2a.PolicyActionDeny,
		MaxPerMin:   0,
		DailyQuota:  0,
	})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("pair upsert changed id: %q -> %q", first.ID, second.ID)
	}
	if second.Action != biza2a.PolicyActionDeny || second.MaxPerMin != 0 {
		t.Fatalf("policy not updated: %+v", second)
	}

	got, err := repo.GetPolicy(ctx, biza2a.FederationLocalOrgID, "org-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Action != biza2a.PolicyActionDeny {
		t.Fatalf("get action = %q", got.Action)
	}

	policies, err := repo.ListPolicies(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(policies) != 1 {
		t.Fatalf("list len = %d, want 1", len(policies))
	}

	if err := repo.DeletePolicy(ctx, first.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.GetPolicy(ctx, biza2a.FederationLocalOrgID, "org-1"); err == nil {
		t.Fatal("expected not-found after delete")
	}
}

func TestA2AFederationRepo_AuditLifecycle(t *testing.T) {
	repo := setupFederationTestRepo(t)
	ctx := context.Background()

	created, err := repo.CreateAudit(ctx, biza2a.FederationAuditLog{
		CallerOrgID:   biza2a.FederationLocalOrgID,
		CalleeOrgID:   "org-1",
		CallerAgentID: "agent-a",
		CalleeAgentID: "agent-b",
		Capability:    "chat",
		Decision:      biza2a.DecisionAllowed,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected generated id")
	}
	if created.Direction != biza2a.AuditDirectionOutbound {
		t.Fatalf("default direction = %q, want outbound", created.Direction)
	}
	if created.Status != biza2a.FederationCallStatusPending {
		t.Fatalf("default status = %q, want pending", created.Status)
	}

	if err := repo.UpdateAuditResult(ctx, created.ID, biza2a.FederationCallStatusSuccess, 123, ""); err != nil {
		t.Fatalf("update result: %v", err)
	}

	// A denied call for the same pair must not consume quota.
	if _, err := repo.CreateAudit(ctx, biza2a.FederationAuditLog{
		CallerOrgID: biza2a.FederationLocalOrgID,
		CalleeOrgID: "org-1",
		Capability:  "chat",
		Decision:    biza2a.DecisionDeniedPolicy,
		Status:      biza2a.FederationCallStatusError,
	}); err != nil {
		t.Fatal(err)
	}

	logs, total, err := repo.ListAudits(ctx, biza2a.FederationAuditFilter{
		CalleeOrgID: "org-1",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 2 || len(logs) != 2 {
		t.Fatalf("list total = %d len = %d, want 2/2", total, len(logs))
	}
	// Desc by created_at: latest first.
	if logs[0].Decision != biza2a.DecisionDeniedPolicy {
		t.Fatalf("latest log decision = %q, want denied_policy", logs[0].Decision)
	}
	if logs[1].Status != biza2a.FederationCallStatusSuccess || logs[1].LatencyMs != 123 {
		t.Fatalf("updated log = %+v", logs[1])
	}

	// Filter by decision.
	denied, total, err := repo.ListAudits(ctx, biza2a.FederationAuditFilter{
		Decision: biza2a.DecisionDeniedPolicy,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(denied) != 1 {
		t.Fatalf("denied filter total = %d, want 1", total)
	}

	// Daily quota counts only decision=allowed.
	n, err := repo.CountCallsSince(ctx, biza2a.FederationLocalOrgID, "org-1", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("allowed count = %d, want 1 (denied must not consume quota)", n)
	}
}
