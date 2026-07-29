package a2a

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type mockRemoteInvokeExecutor struct {
	invokeFn   func(ctx context.Context, remote RemoteAgent, capability, payloadJSON string, timeoutSec int) (string, error)
	calls      int
	gotRemote  RemoteAgent
	gotCap     string
	gotPayload string
}

func (m *mockRemoteInvokeExecutor) InvokeRemote(ctx context.Context, remote RemoteAgent, capability, payloadJSON string, timeoutSec int) (string, error) {
	m.calls++
	m.gotRemote, m.gotCap, m.gotPayload = remote, capability, payloadJSON
	if m.invokeFn != nil {
		return m.invokeFn(ctx, remote, capability, payloadJSON, timeoutSec)
	}
	return `{"output":"ok"}`, nil
}

// newTestFederationUsecase builds a FederationUsecase over the standard mocks
// with one trusted active org and no policies. fn customizes the parts.
func newTestFederationUsecase(fn func(*testFedParts)) (*FederationUsecase, *testFedParts) {
	p := &testFedParts{
		orgs: &mockFederationOrgRepo{
			getFn: func(_ context.Context, id string) (FederationOrg, error) {
				return FederationOrg{ID: id, Name: "Org B", Domain: "b.example.com", TrustLevel: TrustLevelTrusted, Status: OrgStatusActive}, nil
			},
		},
		policyRepo: &mockFederationPolicyRepo{},
		auditRepo:  &mockFederationAuditRepo{},
		remotes: &mockRemoteAgentLister{
			listFn: func(context.Context, string) ([]RemoteAgent, error) {
				return []RemoteAgent{{
					ID: "remote-1", Workspace: "ws-1", OrgID: "org-b", Enabled: true,
					RemoteURL: "https://b.example.com/a2a",
				}}, nil
			},
		},
		executor: &mockRemoteInvokeExecutor{},
	}
	if fn != nil {
		fn(p)
	}
	engine := NewPolicyEngine(p.policyRepo, loggateway.NewNoop())
	if err := engine.Load(context.Background()); err != nil {
		panic(err)
	}
	gov := &FederationGovernance{
		Trust:  NewTrustManager(loggateway.NewNoop()),
		Policy: engine,
		Quota:  NewQuotaChecker(engine, p.auditRepo, nil, loggateway.NewNoop()),
		Audit:  NewAuditLogger(p.auditRepo, loggateway.NewNoop()),
	}
	u := NewFederationUsecase(p.orgs, gov, NewDirectory(p.orgs, p.remotes),
		NewAgentCardSync(p.remotes, nil, nil, loggateway.NewNoop()),
		p.remotes, p.executor)
	return u, p
}

type testFedParts struct {
	orgs       *mockFederationOrgRepo
	policyRepo *mockFederationPolicyRepo
	auditRepo  *mockFederationAuditRepo
	remotes    *mockRemoteAgentLister
	executor   *mockRemoteInvokeExecutor
}

func validInvokeInput() FederatedInvokeInput {
	return FederatedInvokeInput{
		OrgID: "org-b", AgentID: "remote-1", Capability: "chat",
		PayloadJSON: `{"text":"hi"}`, Workspace: "ws-1", CallerAgentID: "agent-1",
	}
}

// --- InvokeFederated governance chain (design F.6) ---

func TestFederationUsecase_InvokeValidation(t *testing.T) {
	u, _ := newTestFederationUsecase(nil)
	for _, in := range []FederatedInvokeInput{
		{AgentID: "a", Capability: "c"},
		{OrgID: "o", Capability: "c"},
		{OrgID: "o", AgentID: "a"},
	} {
		if _, err := u.InvokeFederated(context.Background(), in); !apierror.IsCode(err, apierror.CodeBadRequest) {
			t.Errorf("InvokeFederated(%+v) err = %v, want 400", in, err)
		}
	}
}

func TestFederationUsecase_InvokeOrgNotFound(t *testing.T) {
	u, _ := newTestFederationUsecase(func(p *testFedParts) {
		p.orgs.getFn = func(context.Context, string) (FederationOrg, error) {
			return FederationOrg{}, apierror.NotFound(apierror.DomainA2AFed, "not found")
		}
	})
	if _, err := u.InvokeFederated(context.Background(), validInvokeInput()); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("err = %v, want 404", err)
	}
}

func TestFederationUsecase_InvokeOrgSuspended(t *testing.T) {
	u, p := newTestFederationUsecase(func(pp *testFedParts) {
		pp.orgs.getFn = func(_ context.Context, id string) (FederationOrg, error) {
			return FederationOrg{ID: id, TrustLevel: TrustLevelTrusted, Status: OrgStatusSuspended}, nil
		}
	})
	if _, err := u.InvokeFederated(context.Background(), validInvokeInput()); !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("err = %v, want 403", err)
	}
	if p.executor.calls != 0 {
		t.Errorf("executor called %d times, want 0", p.executor.calls)
	}
}

func TestFederationUsecase_InvokeUntrustedDenied(t *testing.T) {
	u, p := newTestFederationUsecase(func(pp *testFedParts) {
		pp.orgs.getFn = func(_ context.Context, id string) (FederationOrg, error) {
			return FederationOrg{ID: id, TrustLevel: TrustLevelUntrusted, Status: OrgStatusActive}, nil
		}
	})
	if _, err := u.InvokeFederated(context.Background(), validInvokeInput()); !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("err = %v, want 403", err)
	}
	if p.executor.calls != 0 {
		t.Errorf("executor called %d times, want 0", p.executor.calls)
	}
	if len(p.auditRepo.created) != 1 || p.auditRepo.created[0].Decision != DecisionDeniedTrust {
		t.Fatalf("denied audits = %+v, want 1 denied_trust", p.auditRepo.created)
	}
}

func TestFederationUsecase_InvokeDeniedByPolicy(t *testing.T) {
	u, p := newTestFederationUsecase(func(pp *testFedParts) {
		pp.policyRepo.listFn = func(context.Context) ([]FederationPolicy, error) {
			return []FederationPolicy{{
				ID: "p1", CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-b", Action: PolicyActionDeny,
			}}, nil
		}
	})
	if _, err := u.InvokeFederated(context.Background(), validInvokeInput()); !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("err = %v, want 403", err)
	}
	if p.executor.calls != 0 {
		t.Errorf("executor called %d times, want 0", p.executor.calls)
	}
	if len(p.auditRepo.created) != 1 || p.auditRepo.created[0].Decision != DecisionDeniedPolicy {
		t.Fatalf("denied audits = %+v, want 1 denied_policy", p.auditRepo.created)
	}
}

func TestFederationUsecase_InvokeDeniedByQuota(t *testing.T) {
	u, p := newTestFederationUsecase(func(pp *testFedParts) {
		pp.policyRepo.listFn = func(context.Context) ([]FederationPolicy, error) {
			return []FederationPolicy{{
				ID: "p1", CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-b",
				Action: PolicyActionAllow, DailyQuota: 1,
			}}, nil
		}
		pp.auditRepo.countFn = func(context.Context, string, string, time.Time) (int, error) {
			return 1, nil // already at quota
		}
	})
	if _, err := u.InvokeFederated(context.Background(), validInvokeInput()); !apierror.IsCode(err, apierror.CodeRateLimit) {
		t.Fatalf("err = %v, want 429", err)
	}
	if p.executor.calls != 0 {
		t.Errorf("executor called %d times, want 0", p.executor.calls)
	}
	if len(p.auditRepo.created) != 1 || p.auditRepo.created[0].Decision != DecisionDeniedQuota {
		t.Fatalf("denied audits = %+v, want 1 denied_quota", p.auditRepo.created)
	}
}

func TestFederationUsecase_InvokeAllowedAuditFailureFailClosed(t *testing.T) {
	u, p := newTestFederationUsecase(func(pp *testFedParts) {
		pp.auditRepo.createFn = func(context.Context, FederationAuditLog) (FederationAuditLog, error) {
			return FederationAuditLog{}, errors.New("db down")
		}
	})
	// FED-NFR1: allowed-decision audit failure aborts the invocation (500).
	if _, err := u.InvokeFederated(context.Background(), validInvokeInput()); !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("err = %v, want 500", err)
	}
	if p.executor.calls != 0 {
		t.Errorf("executor called %d times, want 0 (fail-closed)", p.executor.calls)
	}
}

func TestFederationUsecase_InvokeAgentNotRegistered(t *testing.T) {
	u, _ := newTestFederationUsecase(func(pp *testFedParts) {
		pp.remotes.listFn = func(context.Context, string) ([]RemoteAgent, error) {
			return nil, nil
		}
	})
	if _, err := u.InvokeFederated(context.Background(), validInvokeInput()); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("err = %v, want 404", err)
	}
}

func TestFederationUsecase_InvokeSuccess(t *testing.T) {
	u, p := newTestFederationUsecase(nil)
	out, err := u.InvokeFederated(context.Background(), validInvokeInput())
	if err != nil {
		t.Fatalf("InvokeFederated() error: %v", err)
	}
	if out.ResultJSON == "" {
		t.Error("result_json empty")
	}
	if out.AuditID == "" {
		t.Error("audit_id empty")
	}
	if out.Status != FederationCallStatusSuccess {
		t.Errorf("status = %q, want %q", out.Status, FederationCallStatusSuccess)
	}
	if p.executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1", p.executor.calls)
	}
	if p.executor.gotRemote.ID != "remote-1" || p.executor.gotCap != "chat" || p.executor.gotPayload != `{"text":"hi"}` {
		t.Errorf("executor args = (%q, %q, %q)", p.executor.gotRemote.ID, p.executor.gotCap, p.executor.gotPayload)
	}
	// allowed decision + success result.
	if len(p.auditRepo.created) != 1 || p.auditRepo.created[0].Decision != DecisionAllowed {
		t.Fatalf("audits = %+v, want 1 allowed", p.auditRepo.created)
	}
	entry := p.auditRepo.created[0]
	if entry.CallerOrgID != FederationLocalOrgID || entry.CalleeOrgID != "org-b" || entry.CalleeAgentID != "remote-1" || entry.Capability != "chat" {
		t.Errorf("audit fields = %+v", entry)
	}
	if p.auditRepo.updateCnt != 1 {
		t.Fatalf("result updates = %d, want 1", p.auditRepo.updateCnt)
	}
}

func TestFederationUsecase_InvokeRemoteFailureRecordsError(t *testing.T) {
	u, p := newTestFederationUsecase(func(pp *testFedParts) {
		pp.executor.invokeFn = func(context.Context, RemoteAgent, string, string, int) (string, error) {
			return "", apierror.Internal(apierror.DomainA2A, "remote invoke failed")
		}
	})
	var gotStatus, gotErr string
	p.auditRepo.updateFn = func(_ context.Context, _ string, status string, _ int64, errMsg string) error {
		gotStatus, gotErr = status, errMsg
		return nil
	}
	// Invoke failures are reported in the result (nil error), mirroring A2A
	// Invoke semantics — the caller still gets the audit correlation.
	out, err := u.InvokeFederated(context.Background(), validInvokeInput())
	if err != nil {
		t.Fatalf("InvokeFederated() error: %v, want nil (status in result)", err)
	}
	if out.Status != FederationCallStatusError || out.ErrorMessage == "" || out.AuditID == "" {
		t.Errorf("result = %+v, want status=error with message + audit_id", out)
	}
	if gotStatus != FederationCallStatusError || gotErr == "" {
		t.Errorf("RecordResult = (%q, %q), want (error, non-empty)", gotStatus, gotErr)
	}
}

func TestFederationUsecase_InvokeTimeoutRecordsTimeout(t *testing.T) {
	u, p := newTestFederationUsecase(func(pp *testFedParts) {
		pp.executor.invokeFn = func(context.Context, RemoteAgent, string, string, int) (string, error) {
			return "", apierror.Internal(apierror.DomainA2A, "remote invoke failed").WithCause(context.DeadlineExceeded)
		}
	})
	var gotStatus string
	p.auditRepo.updateFn = func(_ context.Context, _ string, status string, _ int64, _ string) error {
		gotStatus = status
		return nil
	}
	out, err := u.InvokeFederated(context.Background(), validInvokeInput())
	if err != nil {
		t.Fatalf("InvokeFederated() error: %v, want nil (status in result)", err)
	}
	if out.Status != FederationCallStatusTimeout {
		t.Errorf("result status = %q, want timeout", out.Status)
	}
	if gotStatus != FederationCallStatusTimeout {
		t.Errorf("RecordResult status = %q, want timeout", gotStatus)
	}
}

// --- Org management ---

func TestFederationUsecase_RegisterOrgValidation(t *testing.T) {
	u, _ := newTestFederationUsecase(nil)
	for _, org := range []FederationOrg{{Domain: "b.example.com"}, {Name: "Org B"}} {
		if _, err := u.RegisterOrg(context.Background(), org); !apierror.IsCode(err, apierror.CodeBadRequest) {
			t.Errorf("RegisterOrg(%+v) err = %v, want 400", org, err)
		}
	}
}

func TestFederationUsecase_RegisterOrgDelegates(t *testing.T) {
	u, p := newTestFederationUsecase(nil)
	var got FederationOrg
	p.orgs.upsertFn = func(_ context.Context, o FederationOrg) (FederationOrg, error) {
		got = o
		o.ID = "org-new"
		return o, nil
	}
	out, err := u.RegisterOrg(context.Background(), FederationOrg{Name: "Org B", Domain: "b.example.com"})
	if err != nil {
		t.Fatalf("RegisterOrg() error: %v", err)
	}
	if out.ID != "org-new" || got.Domain != "b.example.com" {
		t.Errorf("out = %+v, got = %+v", out, got)
	}
}

func TestFederationUsecase_GetOrgDelegates(t *testing.T) {
	u, _ := newTestFederationUsecase(nil)
	org, err := u.GetOrg(context.Background(), "org-b")
	if err != nil {
		t.Fatalf("GetOrg() error: %v", err)
	}
	if org.ID != "org-b" || org.Name != "Org B" {
		t.Errorf("org = %+v", org)
	}
}

func TestFederationUsecase_SetTrustLevelValidation(t *testing.T) {
	u, _ := newTestFederationUsecase(nil)
	if err := u.SetTrustLevel(context.Background(), "org-b", "bogus"); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("err = %v, want 400", err)
	}
}

func TestFederationUsecase_SetTrustLevelDelegates(t *testing.T) {
	u, p := newTestFederationUsecase(nil)
	var gotID, gotLevel string
	p.orgs.trustFn = func(_ context.Context, id, level string) error {
		gotID, gotLevel = id, level
		return nil
	}
	if err := u.SetTrustLevel(context.Background(), "org-b", TrustLevelUntrusted); err != nil {
		t.Fatalf("SetTrustLevel() error: %v", err)
	}
	if gotID != "org-b" || gotLevel != TrustLevelUntrusted {
		t.Errorf("args = (%q, %q)", gotID, gotLevel)
	}
}

// --- Policy management ---

func TestFederationUsecase_UpsertPolicyValidation(t *testing.T) {
	u, _ := newTestFederationUsecase(nil)
	for _, p := range []FederationPolicy{
		{CalleeOrgID: "org-b", Action: PolicyActionAllow},
		{CallerOrgID: FederationLocalOrgID, Action: PolicyActionAllow},
		{CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-b", Action: "bogus"},
		{CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-b", Action: PolicyActionAllow, MaxPerMin: -1},
		{CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-b", Action: PolicyActionAllow, DailyQuota: -1},
	} {
		if _, err := u.UpsertPolicy(context.Background(), p); !apierror.IsCode(err, apierror.CodeBadRequest) {
			t.Errorf("UpsertPolicy(%+v) err = %v, want 400", p, err)
		}
	}
}

func TestFederationUsecase_UpsertPolicyRefreshesEngine(t *testing.T) {
	u, p := newTestFederationUsecase(func(pp *testFedParts) {
		pp.policyRepo.upsertFn = func(_ context.Context, in FederationPolicy) (FederationPolicy, error) {
			in.ID = "p-new"
			return in, nil
		}
	})
	in := FederationPolicy{
		CallerOrgID: FederationLocalOrgID, CalleeOrgID: "org-b",
		Action: PolicyActionAllow, MaxPerMin: 10, DailyQuota: 100,
	}
	out, err := u.UpsertPolicy(context.Background(), in)
	if err != nil {
		t.Fatalf("UpsertPolicy() error: %v", err)
	}
	if out.ID != "p-new" {
		t.Errorf("out.ID = %q", out.ID)
	}
	cached, found := u.gov.Policy.Evaluate(FederationLocalOrgID, "org-b")
	if !found || cached.MaxPerMin != 10 || cached.DailyQuota != 100 {
		t.Errorf("engine cache = %+v (found=%v), want refreshed entry", cached, found)
	}
	_ = p
}

// --- Audit query ---

func TestFederationUsecase_QueryAuditLogsDefaultsLimit(t *testing.T) {
	u, p := newTestFederationUsecase(nil)
	var gotFilter FederationAuditFilter
	p.auditRepo.listFn = func(_ context.Context, f FederationAuditFilter) ([]FederationAuditLog, int, error) {
		gotFilter = f
		return nil, 0, nil
	}
	if _, _, err := u.QueryAuditLogs(context.Background(), FederationAuditFilter{}); err != nil {
		t.Fatalf("QueryAuditLogs() error: %v", err)
	}
	if gotFilter.Limit != 50 {
		t.Errorf("default limit = %d, want 50", gotFilter.Limit)
	}
}

func TestFederationUsecase_QueryAuditLogsCapsLimit(t *testing.T) {
	u, p := newTestFederationUsecase(nil)
	var gotFilter FederationAuditFilter
	p.auditRepo.listFn = func(_ context.Context, f FederationAuditFilter) ([]FederationAuditLog, int, error) {
		gotFilter = f
		return nil, 0, nil
	}
	if _, _, err := u.QueryAuditLogs(context.Background(), FederationAuditFilter{Limit: 9999}); err != nil {
		t.Fatalf("QueryAuditLogs() error: %v", err)
	}
	if gotFilter.Limit != 200 {
		t.Errorf("capped limit = %d, want 200", gotFilter.Limit)
	}
}
