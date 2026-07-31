package service

import (
	"context"
	"errors"
	"testing"
	"time"

	v1 "aranea-agents/api/kratos/a2a/v1"
	a2abiz "aranea-agents/internal/biz/a2a"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

// --- local mocks over the federation narrow ports ---

type fedOrgRepoMock struct {
	orgs    []a2abiz.FederationOrg
	upsert  func(a2abiz.FederationOrg) (a2abiz.FederationOrg, error)
	deleted string
	trustID string
	trustLv string
}

func (m *fedOrgRepoMock) UpsertOrg(_ context.Context, org a2abiz.FederationOrg) (a2abiz.FederationOrg, error) {
	if m.upsert != nil {
		return m.upsert(org)
	}
	if org.ID == "" {
		org.ID = "org-1"
	}
	m.orgs = append(m.orgs, org)
	return org, nil
}

func (m *fedOrgRepoMock) GetOrg(_ context.Context, id string) (a2abiz.FederationOrg, error) {
	for _, o := range m.orgs {
		if o.ID == id {
			return o, nil
		}
	}
	return a2abiz.FederationOrg{}, apierror.NotFound(apierror.DomainA2AFed, "org %s not found", id)
}

func (m *fedOrgRepoMock) ListOrgs(context.Context) ([]a2abiz.FederationOrg, error) {
	return m.orgs, nil
}

func (m *fedOrgRepoMock) UpdateOrgTrust(_ context.Context, id, trustLevel string) error {
	m.trustID, m.trustLv = id, trustLevel
	for i, o := range m.orgs {
		if o.ID == id {
			m.orgs[i].TrustLevel = trustLevel
			return nil
		}
	}
	return apierror.NotFound(apierror.DomainA2AFed, "org %s not found", id)
}

func (m *fedOrgRepoMock) DeleteOrg(_ context.Context, id string) error {
	m.deleted = id
	return nil
}

type fedPolicyRepoMock struct {
	policies []a2abiz.FederationPolicy
}

func (m *fedPolicyRepoMock) UpsertPolicy(_ context.Context, p a2abiz.FederationPolicy) (a2abiz.FederationPolicy, error) {
	if p.ID == "" {
		p.ID = "pol-1"
	}
	m.policies = append(m.policies, p)
	return p, nil
}

func (m *fedPolicyRepoMock) GetPolicy(_ context.Context, callerOrgID, calleeOrgID string) (a2abiz.FederationPolicy, error) {
	for _, p := range m.policies {
		if p.CallerOrgID == callerOrgID && p.CalleeOrgID == calleeOrgID {
			return p, nil
		}
	}
	return a2abiz.FederationPolicy{}, apierror.NotFound(apierror.DomainA2AFed, "policy not found")
}

func (m *fedPolicyRepoMock) ListPolicies(context.Context) ([]a2abiz.FederationPolicy, error) {
	return m.policies, nil
}

func (m *fedPolicyRepoMock) DeletePolicy(_ context.Context, id string) error {
	out := m.policies[:0]
	for _, p := range m.policies {
		if p.ID != id {
			out = append(out, p)
		}
	}
	m.policies = out
	return nil
}

type fedAuditRepoMock struct {
	created []a2abiz.FederationAuditLog
	filter  a2abiz.FederationAuditFilter
	items   []a2abiz.FederationAuditLog
	total   int
}

func (m *fedAuditRepoMock) CreateAudit(_ context.Context, l a2abiz.FederationAuditLog) (a2abiz.FederationAuditLog, error) {
	m.created = append(m.created, l)
	return l, nil
}

func (m *fedAuditRepoMock) UpdateAuditResult(context.Context, string, string, int64, string) error {
	return nil
}

func (m *fedAuditRepoMock) ListAudits(_ context.Context, f a2abiz.FederationAuditFilter) ([]a2abiz.FederationAuditLog, int, error) {
	m.filter = f
	return m.items, m.total, nil
}

func (m *fedAuditRepoMock) CountCallsSince(context.Context, string, string, time.Time) (int, error) {
	return 0, nil
}

type fedRemoteListerMock struct {
	items []a2abiz.RemoteAgent
}

func (m *fedRemoteListerMock) ListRemoteAgents(context.Context, string) ([]a2abiz.RemoteAgent, error) {
	return m.items, nil
}

type fedExecutorMock struct {
	result string
	err    error
}

func (m *fedExecutorMock) InvokeRemote(context.Context, a2abiz.RemoteAgent, string, string, int) (string, error) {
	return m.result, m.err
}

type fedCardDiscovererMock struct{}

func (fedCardDiscovererMock) DiscoverRemoteCard(_ context.Context, in a2abiz.RemoteCardDiscoverInput) (a2abiz.AgentCard, error) {
	return a2abiz.AgentCard{AgentID: "card-synced", Enabled: true}, nil
}

type fedCardWriterMock struct {
	updated []string
}

func (m *fedCardWriterMock) UpdateRemoteAgentCard(_ context.Context, id string, _ a2abiz.AgentCard) error {
	m.updated = append(m.updated, id)
	return nil
}

// newTestFederationService builds a FederationService over in-memory mocks.
func newTestFederationService(t *testing.T, orgs []a2abiz.FederationOrg, remotes []a2abiz.RemoteAgent, executor a2abiz.RemoteInvokeExecutor) (*FederationService, *fedOrgRepoMock, *fedAuditRepoMock) {
	t.Helper()
	orgRepo := &fedOrgRepoMock{orgs: orgs}
	policyRepo := &fedPolicyRepoMock{}
	auditRepo := &fedAuditRepoMock{}
	remoteLister := &fedRemoteListerMock{items: remotes}
	if executor == nil {
		executor = &fedExecutorMock{result: `{"ok":true}`}
	}
	engine := a2abiz.NewPolicyEngine(policyRepo, loggateway.NewNoop())
	if err := engine.Load(context.Background()); err != nil {
		t.Fatalf("policy engine load: %v", err)
	}
	gov := &a2abiz.FederationGovernance{
		Trust:  a2abiz.NewTrustManager(loggateway.NewNoop()),
		Policy: engine,
		Quota:  a2abiz.NewQuotaChecker(engine, auditRepo, nil, loggateway.NewNoop()),
		Audit:  a2abiz.NewAuditLogger(auditRepo, loggateway.NewNoop()),
	}
	uc := a2abiz.NewFederationUsecase(orgRepo, gov,
		a2abiz.NewDirectory(orgRepo, remoteLister),
		a2abiz.NewAgentCardSync(remoteLister, fedCardDiscovererMock{}, &fedCardWriterMock{}, loggateway.NewNoop()),
		remoteLister, executor, nil)
	return NewFederationService(uc), orgRepo, auditRepo
}

func fedAdminCtx() context.Context {
	return auth.NewContext(context.Background(), &auth.Auth{UserID: 1, Access: "admin"})
}

func sampleFedOrg() a2abiz.FederationOrg {
	return a2abiz.FederationOrg{
		ID: "org-b", Name: "Org B", Domain: "b.example.com",
		PublicBaseURL: "https://b.example.com", TrustLevel: a2abiz.TrustLevelTrusted,
		AuthType: "api_key", AuthConfigJSON: `{"key":"secret"}`,
		Status:   a2abiz.OrgStatusActive,
		JoinedAt: time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC),
	}
}

// --- RegisterFederationOrg ---

func TestFederationService_RegisterOrgRequiresAdmin(t *testing.T) {
	svc, _, _ := newTestFederationService(t, nil, nil, nil)
	if _, err := svc.RegisterFederationOrg(context.Background(), &v1.RegisterFederationOrgRequest{
		Name: "Org B", Domain: "b.example.com",
	}); !apierror.IsCode(err, apierror.CodeUnauthorized) {
		t.Fatalf("err = %v, want 401", err)
	}
}

func TestFederationService_RegisterOrgMapping(t *testing.T) {
	svc, orgRepo, _ := newTestFederationService(t, nil, nil, nil)
	out, err := svc.RegisterFederationOrg(fedAdminCtx(), &v1.RegisterFederationOrgRequest{
		Name: "Org B", Domain: "b.example.com", PublicBaseUrl: "https://b.example.com",
		TrustLevel: "trusted", AuthType: "api_key", AuthConfigJson: `{"key":"secret"}`,
	})
	if err != nil {
		t.Fatalf("RegisterFederationOrg() error: %v", err)
	}
	if out.GetId() == "" || out.GetName() != "Org B" || out.GetDomain() != "b.example.com" {
		t.Errorf("out = %+v", out)
	}
	if !out.GetAuthConfigSet() {
		t.Error("auth_config_set = false, want true (auth_config_json provided)")
	}
	if len(orgRepo.orgs) != 1 || orgRepo.orgs[0].TrustLevel != "trusted" {
		t.Errorf("persisted orgs = %+v", orgRepo.orgs)
	}
}

func TestFederationService_RegisterOrgValidation(t *testing.T) {
	svc, _, _ := newTestFederationService(t, nil, nil, nil)
	if _, err := svc.RegisterFederationOrg(fedAdminCtx(), &v1.RegisterFederationOrgRequest{Name: "Org B"}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("err = %v, want 400", err)
	}
}

// --- ListFederationOrgs ---

func TestFederationService_ListOrgsMapping(t *testing.T) {
	svc, _, _ := newTestFederationService(t, []a2abiz.FederationOrg{sampleFedOrg()}, nil, nil)
	out, err := svc.ListFederationOrgs(context.Background(), &v1.ListFederationOrgsRequest{})
	if err != nil {
		t.Fatalf("ListFederationOrgs() error: %v", err)
	}
	if len(out.GetItems()) != 1 {
		t.Fatalf("items = %d, want 1", len(out.GetItems()))
	}
	item := out.GetItems()[0]
	if item.GetId() != "org-b" || item.GetTrustLevel() != "trusted" || !item.GetAuthConfigSet() {
		t.Errorf("item = %+v", item)
	}
	if item.GetJoinedAt() == "" {
		t.Error("joined_at empty, want RFC3339")
	}
}

// --- DeleteFederationOrg ---

func TestFederationService_DeleteOrg(t *testing.T) {
	svc, orgRepo, _ := newTestFederationService(t, nil, nil, nil)
	if _, err := svc.DeleteFederationOrg(context.Background(), &v1.DeleteFederationOrgRequest{Id: "org-b"}); !apierror.IsCode(err, apierror.CodeUnauthorized) {
		t.Fatalf("no-auth err = %v, want 401", err)
	}
	if _, err := svc.DeleteFederationOrg(fedAdminCtx(), &v1.DeleteFederationOrgRequest{Id: "org-b"}); err != nil {
		t.Fatalf("DeleteFederationOrg() error: %v", err)
	}
	if orgRepo.deleted != "org-b" {
		t.Errorf("deleted = %q", orgRepo.deleted)
	}
}

// --- SetFederationTrustLevel ---

func TestFederationService_SetTrustLevelReturnsUpdatedOrg(t *testing.T) {
	svc, orgRepo, _ := newTestFederationService(t, []a2abiz.FederationOrg{sampleFedOrg()}, nil, nil)
	out, err := svc.SetFederationTrustLevel(fedAdminCtx(), &v1.SetFederationTrustLevelRequest{
		Id: "org-b", TrustLevel: "neutral",
	})
	if err != nil {
		t.Fatalf("SetFederationTrustLevel() error: %v", err)
	}
	if out.GetTrustLevel() != "neutral" {
		t.Errorf("trust_level = %q, want neutral", out.GetTrustLevel())
	}
	if orgRepo.trustID != "org-b" || orgRepo.trustLv != "neutral" {
		t.Errorf("repo args = (%q, %q)", orgRepo.trustID, orgRepo.trustLv)
	}
}

func TestFederationService_SetTrustLevelInvalid(t *testing.T) {
	svc, _, _ := newTestFederationService(t, []a2abiz.FederationOrg{sampleFedOrg()}, nil, nil)
	if _, err := svc.SetFederationTrustLevel(fedAdminCtx(), &v1.SetFederationTrustLevelRequest{
		Id: "org-b", TrustLevel: "bogus",
	}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("err = %v, want 400", err)
	}
}

// --- SyncFederationOrgCards ---

func TestFederationService_SyncCardsRequiresAdmin(t *testing.T) {
	svc, _, _ := newTestFederationService(t, []a2abiz.FederationOrg{sampleFedOrg()}, nil, nil)
	if _, err := svc.SyncFederationOrgCards(context.Background(), &v1.SyncFederationOrgCardsRequest{Id: "org-b"}); !apierror.IsCode(err, apierror.CodeUnauthorized) {
		t.Fatalf("err = %v, want 401", err)
	}
}

func TestFederationService_SyncCardsMapping(t *testing.T) {
	remotes := []a2abiz.RemoteAgent{
		{ID: "remote-1", Workspace: "ws-1", OrgID: "org-b", Enabled: true, RemoteURL: "https://b.example.com/a2a"},
		{ID: "remote-2", Workspace: "ws-1", OrgID: "org-other", Enabled: true, RemoteURL: "https://c.example.com/a2a"},
	}
	svc, _, _ := newTestFederationService(t, []a2abiz.FederationOrg{sampleFedOrg()}, remotes, nil)
	out, err := svc.SyncFederationOrgCards(fedAdminCtx(), &v1.SyncFederationOrgCardsRequest{Id: "org-b"})
	if err != nil {
		t.Fatalf("SyncFederationOrgCards() error: %v", err)
	}
	if out.GetSynced() != 1 {
		t.Errorf("synced = %d, want 1 (only org-b's enabled remote)", out.GetSynced())
	}
}

// --- UpsertFederationPolicy ---

func TestFederationService_UpsertPolicyMapping(t *testing.T) {
	svc, _, _ := newTestFederationService(t, nil, nil, nil)
	out, err := svc.UpsertFederationPolicy(fedAdminCtx(), &v1.UpsertFederationPolicyRequest{
		CallerOrgId: "local", CalleeOrgId: "org-b", Action: "allow", MaxPerMin: 10, DailyQuota: 100,
	})
	if err != nil {
		t.Fatalf("UpsertFederationPolicy() error: %v", err)
	}
	if out.GetId() == "" || out.GetCallerOrgId() != "local" || out.GetMaxPerMin() != 10 || out.GetDailyQuota() != 100 {
		t.Errorf("out = %+v", out)
	}
}

func TestFederationService_UpsertPolicyValidation(t *testing.T) {
	svc, _, _ := newTestFederationService(t, nil, nil, nil)
	if _, err := svc.UpsertFederationPolicy(fedAdminCtx(), &v1.UpsertFederationPolicyRequest{
		CallerOrgId: "local", Action: "allow",
	}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("err = %v, want 400", err)
	}
}

// --- DiscoverFederationAgents ---

func TestFederationService_DiscoverAgentsMapping(t *testing.T) {
	remotes := []a2abiz.RemoteAgent{{
		ID: "remote-1", Workspace: "ws-1", OrgID: "org-b", DisplayName: "B Agent",
		RemoteURL: "https://b.example.com/a2a", Enabled: true,
		DiscoveredCard: a2abiz.AgentCard{
			AgentID: "remote-1", DisplayName: "B Agent", Enabled: true,
			Capabilities: []a2abiz.Capability{{Name: "chat", Description: "chat capability"}},
		},
	}}
	svc, _, _ := newTestFederationService(t, []a2abiz.FederationOrg{sampleFedOrg()}, remotes, nil)
	out, err := svc.DiscoverFederationAgents(context.Background(), &v1.DiscoverFederationAgentsRequest{})
	if err != nil {
		t.Fatalf("DiscoverFederationAgents() error: %v", err)
	}
	if len(out.GetItems()) != 1 {
		t.Fatalf("items = %d, want 1", len(out.GetItems()))
	}
	entry := out.GetItems()[0]
	if entry.GetOrg().GetId() != "org-b" || entry.GetRemoteAgent().GetId() != "remote-1" || entry.GetCard().GetAgentId() != "remote-1" {
		t.Errorf("entry = %+v", entry)
	}
}

// --- InvokeFederatedAgent ---

func TestFederationService_InvokeRequiresAdmin(t *testing.T) {
	svc, _, _ := newTestFederationService(t, []a2abiz.FederationOrg{sampleFedOrg()}, nil, nil)
	_, err := svc.InvokeFederatedAgent(context.Background(), &v1.InvokeFederatedAgentRequest{
		OrgId: "org-b", AgentId: "remote-1", Capability: "chat", PayloadJson: `{}`,
	})
	if !apierror.IsCode(err, apierror.CodeUnauthorized) {
		t.Fatalf("err = %v, want 401", err)
	}
}

func TestFederationService_InvokeSuccess(t *testing.T) {
	remotes := []a2abiz.RemoteAgent{{
		ID: "remote-1", Workspace: "ws-1", OrgID: "org-b", Enabled: true,
		RemoteURL: "https://b.example.com/a2a",
	}}
	svc, _, _ := newTestFederationService(t, []a2abiz.FederationOrg{sampleFedOrg()}, remotes, nil)
	out, err := svc.InvokeFederatedAgent(fedAdminCtx(), &v1.InvokeFederatedAgentRequest{
		OrgId: "org-b", AgentId: "remote-1", Capability: "chat",
		PayloadJson: `{"text":"hi"}`, Workspace: "ws-1", CallerAgentId: "agent-1",
	})
	if err != nil {
		t.Fatalf("InvokeFederatedAgent() error: %v", err)
	}
	if out.GetAuditId() == "" || out.GetStatus() != "success" || out.GetResultJson() != `{"ok":true}` {
		t.Errorf("out = %+v", out)
	}
}

func TestFederationService_InvokeFailureStatusInResult(t *testing.T) {
	remotes := []a2abiz.RemoteAgent{{
		ID: "remote-1", Workspace: "ws-1", OrgID: "org-b", Enabled: true,
		RemoteURL: "https://b.example.com/a2a",
	}}
	svc, _, _ := newTestFederationService(t, []a2abiz.FederationOrg{sampleFedOrg()}, remotes,
		&fedExecutorMock{err: errors.New("connection refused")})
	out, err := svc.InvokeFederatedAgent(fedAdminCtx(), &v1.InvokeFederatedAgentRequest{
		OrgId: "org-b", AgentId: "remote-1", Capability: "chat", PayloadJson: `{}`, Workspace: "ws-1",
	})
	if err != nil {
		t.Fatalf("InvokeFederatedAgent() error: %v, want nil (status in result)", err)
	}
	if out.GetStatus() != "error" || out.GetErrorMessage() == "" || out.GetAuditId() == "" {
		t.Errorf("out = %+v", out)
	}
}

func TestFederationService_InvokeUntrustedForbidden(t *testing.T) {
	org := sampleFedOrg()
	org.TrustLevel = a2abiz.TrustLevelUntrusted
	svc, _, auditRepo := newTestFederationService(t, []a2abiz.FederationOrg{org}, nil, nil)
	_, err := svc.InvokeFederatedAgent(fedAdminCtx(), &v1.InvokeFederatedAgentRequest{
		OrgId: "org-b", AgentId: "remote-1", Capability: "chat", PayloadJson: `{}`,
	})
	if !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("err = %v, want 403", err)
	}
	if len(auditRepo.created) != 1 || auditRepo.created[0].Decision != a2abiz.DecisionDeniedTrust {
		t.Errorf("denied audits = %+v", auditRepo.created)
	}
}

// --- QueryFederationAuditLogs ---

func TestFederationService_QueryAuditLogsMapping(t *testing.T) {
	svc, _, auditRepo := newTestFederationService(t, nil, nil, nil)
	auditRepo.items = []a2abiz.FederationAuditLog{{
		ID: "aud-1", Direction: "outbound", CallerOrgID: "local", CalleeOrgID: "org-b",
		CalleeAgentID: "remote-1", Capability: "chat", Decision: "allowed", Status: "success",
		LatencyMs: 42, CreatedAt: time.Date(2026, 7, 29, 8, 0, 0, 0, time.UTC),
	}}
	auditRepo.total = 1
	out, err := svc.QueryFederationAuditLogs(context.Background(), &v1.QueryFederationAuditLogsRequest{
		CalleeOrgId: "org-b", Decision: "allowed", Limit: 10, Offset: 5,
	})
	if err != nil {
		t.Fatalf("QueryFederationAuditLogs() error: %v", err)
	}
	if out.GetTotal() != 1 || len(out.GetItems()) != 1 {
		t.Fatalf("out = %+v", out)
	}
	item := out.GetItems()[0]
	if item.GetId() != "aud-1" || item.GetLatencyMs() != 42 || item.GetCreatedAt() == "" {
		t.Errorf("item = %+v", item)
	}
	if auditRepo.filter.CalleeOrgID != "org-b" || auditRepo.filter.Decision != "allowed" ||
		auditRepo.filter.Limit != 10 || auditRepo.filter.Offset != 5 {
		t.Errorf("filter = %+v", auditRepo.filter)
	}
}
