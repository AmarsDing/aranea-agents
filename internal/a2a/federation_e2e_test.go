package a2a

// Federation end-to-end tests (T17, design F.13): httptest mocks the remote
// organization's A2A endpoint; the full governance chain (org → trust → policy
// → quota → audit fail-closed → target resolve → real remote HTTP invoke →
// result audit) runs through FederationUsecase with in-memory repos.
//
// Covers requirement acceptance criteria 1-7 (26-a2a-protocol.md §子模块.4)
// plus FED-F8 api_key auth, FED-F3 unreachable-remote error path, and the
// audit fail-closed acceptance item (development §子模块.5).
//
// All tests in this file are sequential (no t.Parallel): allowAllRemoteIPs
// swaps a package-level SSRF predicate.

import (
	"context"
	"strings"
	"testing"

	a2abiz "aranea-agents/internal/biz/a2a"
	"aranea-agents/pkg/apierror"
)

// findAudits returns entries matching decision ("" = any).
func findAudits(entries []a2abiz.FederationAuditLog, decision string) []a2abiz.FederationAuditLog {
	out := make([]a2abiz.FederationAuditLog, 0, len(entries))
	for _, e := range entries {
		if decision == "" || e.Decision == decision {
			out = append(out, e)
		}
	}
	return out
}

// Criterion 1: register two organizations; the org list shows them.
func TestFederationE2E_RegisterAndListOrgs(t *testing.T) {
	f := newFederationE2EFixture(t)
	orgA := f.registerOrg(t, "Acme Corp", "acme.example.com", a2abiz.TrustLevelTrusted)
	orgB := f.registerOrg(t, "Globex", "globex.example.com", a2abiz.TrustLevelNeutral)

	orgs, err := f.uc.ListOrgs(context.Background())
	if err != nil {
		t.Fatalf("list orgs: %v", err)
	}
	if len(orgs) != 2 {
		t.Fatalf("expected 2 orgs, got %d", len(orgs))
	}
	seen := map[string]string{}
	for _, o := range orgs {
		seen[o.ID] = o.Domain
	}
	if seen[orgA.ID] != "acme.example.com" || seen[orgB.ID] != "globex.example.com" {
		t.Fatalf("org list missing registered orgs: %v", seen)
	}
	if orgA.TrustLevel != a2abiz.TrustLevelTrusted || orgA.Status != a2abiz.OrgStatusActive {
		t.Fatalf("unexpected org state: %+v", orgA)
	}
}

// Criterion 2: directory discovery filters by capability and org; untrusted
// orgs are excluded.
func TestFederationE2E_DirectoryFilterByCapability(t *testing.T) {
	f := newFederationE2EFixture(t)
	ctx := context.Background()
	orgA := f.registerOrg(t, "Acme Corp", "acme.example.com", a2abiz.TrustLevelTrusted)
	orgB := f.registerOrg(t, "Globex", "globex.example.com", a2abiz.TrustLevelNeutral)
	f.addRemote(orgA.ID, "ra-a1", "https://a2a.acme.example.com/agent1", "none", "", "summarize", "translate")
	f.addRemote(orgA.ID, "ra-a2", "https://a2a.acme.example.com/agent2", "none", "", "code-review")
	f.addRemote(orgB.ID, "ra-b1", "https://a2a.globex.example.com/agent1", "none", "", "summarize")

	// capability filter: only agents exposing "summarize"
	entries, err := f.uc.ListFederationAgents(ctx, "summarize", "")
	if err != nil {
		t.Fatalf("directory: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 summarize entries, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Card.Source != a2abiz.SourceRemote {
			t.Fatalf("expected remote source card, got %q", e.Card.Source)
		}
	}

	// org filter: only orgA agents
	entries, err = f.uc.ListFederationAgents(ctx, "", orgA.ID)
	if err != nil {
		t.Fatalf("directory org filter: %v", err)
	}
	if len(entries) != 2 || entries[0].Org.ID != orgA.ID {
		t.Fatalf("expected 2 orgA entries, got %+v", entries)
	}

	// unknown org filter → empty
	entries, err = f.uc.ListFederationAgents(ctx, "", "org-unknown")
	if err != nil {
		t.Fatalf("directory unknown org: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty directory for unknown org, got %d", len(entries))
	}

	// untrusted org excluded from directory
	if err := f.uc.SetTrustLevel(ctx, orgB.ID, a2abiz.TrustLevelUntrusted); err != nil {
		t.Fatalf("set trust: %v", err)
	}
	entries, err = f.uc.ListFederationAgents(ctx, "summarize", "")
	if err != nil {
		t.Fatalf("directory after untrust: %v", err)
	}
	if len(entries) != 1 || entries[0].Org.ID != orgA.ID {
		t.Fatalf("expected only orgA summarize entry after untrusting orgB, got %+v", entries)
	}
}

// Criterion 3: successful governed invocation returns the remote result; the
// audit row carries decision=allowed + status=success.
func TestFederationE2E_InvokeSuccessWritesAudit(t *testing.T) {
	allowAllRemoteIPs(t)
	f := newFederationE2EFixture(t)
	ctx := context.Background()
	mock := newMockA2AEndpoint(t)
	org := f.registerOrg(t, "Acme Corp", "acme.example.com", a2abiz.TrustLevelNeutral)
	f.addRemote(org.ID, "ra-1", mock.url(), "none", "", "chat")

	res, err := f.uc.InvokeFederated(ctx, a2abiz.FederatedInvokeInput{
		OrgID: org.ID, AgentID: "ra-1", Capability: "chat",
		PayloadJSON: `{"text":"hello federation"}`, Workspace: "ws1", TimeoutSec: 5,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if res.Status != a2abiz.FederationCallStatusSuccess {
		t.Fatalf("expected success, got %q (%s)", res.Status, res.ErrorMessage)
	}
	if !strings.Contains(res.ResultJSON, "pong: hello federation") {
		t.Fatalf("unexpected result json: %s", res.ResultJSON)
	}
	if res.AuditID == "" {
		t.Fatal("expected audit id on success")
	}

	// remote saw the payload text
	reqs := mock.received()
	if len(reqs) != 1 || reqs[0].Text != "hello federation" {
		t.Fatalf("remote requests: %+v", reqs)
	}

	// audit: one row, allowed + success, attributed to local -> org
	entries := f.audits.snapshot()
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(entries))
	}
	e := entries[0]
	if e.ID != res.AuditID || e.Decision != a2abiz.DecisionAllowed || e.Status != a2abiz.FederationCallStatusSuccess {
		t.Fatalf("audit row mismatch: %+v", e)
	}
	if e.CallerOrgID != a2abiz.FederationLocalOrgID || e.CalleeOrgID != org.ID || e.CalleeAgentID != "ra-1" || e.Capability != "chat" {
		t.Fatalf("audit attribution mismatch: %+v", e)
	}
	if e.Direction != a2abiz.AuditDirectionOutbound {
		t.Fatalf("expected outbound direction, got %q", e.Direction)
	}
}

// Criterion 4: untrusted org invocation is rejected (403) with a denied_trust
// audit row; the remote endpoint is never contacted.
func TestFederationE2E_InvokeDeniedTrust(t *testing.T) {
	allowAllRemoteIPs(t)
	f := newFederationE2EFixture(t)
	ctx := context.Background()
	mock := newMockA2AEndpoint(t)
	org := f.registerOrg(t, "Acme Corp", "acme.example.com", a2abiz.TrustLevelTrusted)
	f.addRemote(org.ID, "ra-1", mock.url(), "none", "", "chat")

	if err := f.uc.SetTrustLevel(ctx, org.ID, a2abiz.TrustLevelUntrusted); err != nil {
		t.Fatalf("set trust: %v", err)
	}
	_, err := f.uc.InvokeFederated(ctx, a2abiz.FederatedInvokeInput{
		OrgID: org.ID, AgentID: "ra-1", Capability: "chat", PayloadJSON: `{}`, Workspace: "ws1",
	})
	if !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("expected 403 forbidden, got %v", err)
	}
	if got := len(mock.received()); got != 0 {
		t.Fatalf("remote must not be contacted on trust denial, got %d requests", got)
	}
	denied := findAudits(f.audits.snapshot(), a2abiz.DecisionDeniedTrust)
	if len(denied) != 1 || denied[0].CalleeOrgID != org.ID {
		t.Fatalf("expected 1 denied_trust audit, got %+v", denied)
	}
}

// Criterion 5: an explicit deny policy rejects the call (403) with a
// denied_policy audit row.
func TestFederationE2E_InvokeDeniedPolicy(t *testing.T) {
	allowAllRemoteIPs(t)
	f := newFederationE2EFixture(t)
	ctx := context.Background()
	mock := newMockA2AEndpoint(t)
	org := f.registerOrg(t, "Acme Corp", "acme.example.com", a2abiz.TrustLevelTrusted)
	f.addRemote(org.ID, "ra-1", mock.url(), "none", "", "chat")

	if _, err := f.uc.UpsertPolicy(ctx, a2abiz.FederationPolicy{
		CallerOrgID: a2abiz.FederationLocalOrgID, CalleeOrgID: org.ID, Action: a2abiz.PolicyActionDeny,
	}); err != nil {
		t.Fatalf("upsert policy: %v", err)
	}
	_, err := f.uc.InvokeFederated(ctx, a2abiz.FederatedInvokeInput{
		OrgID: org.ID, AgentID: "ra-1", Capability: "chat", PayloadJSON: `{}`, Workspace: "ws1",
	})
	if !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("expected 403 forbidden, got %v", err)
	}
	if got := len(mock.received()); got != 0 {
		t.Fatalf("remote must not be contacted on policy denial, got %d requests", got)
	}
	denied := findAudits(f.audits.snapshot(), a2abiz.DecisionDeniedPolicy)
	if len(denied) != 1 {
		t.Fatalf("expected 1 denied_policy audit, got %+v", denied)
	}
}

// Criterion 6: exceeding DailyQuota rejects with 429 and a denied_quota audit
// row; the first call within quota succeeds.
func TestFederationE2E_InvokeDeniedDailyQuota(t *testing.T) {
	allowAllRemoteIPs(t)
	f := newFederationE2EFixture(t)
	ctx := context.Background()
	mock := newMockA2AEndpoint(t)
	org := f.registerOrg(t, "Acme Corp", "acme.example.com", a2abiz.TrustLevelTrusted)
	f.addRemote(org.ID, "ra-1", mock.url(), "none", "", "chat")

	if _, err := f.uc.UpsertPolicy(ctx, a2abiz.FederationPolicy{
		CallerOrgID: a2abiz.FederationLocalOrgID, CalleeOrgID: org.ID,
		Action: a2abiz.PolicyActionAllow, DailyQuota: 1,
	}); err != nil {
		t.Fatalf("upsert policy: %v", err)
	}

	// first call within quota succeeds
	res, err := f.uc.InvokeFederated(ctx, a2abiz.FederatedInvokeInput{
		OrgID: org.ID, AgentID: "ra-1", Capability: "chat", PayloadJSON: `{}`, Workspace: "ws1", TimeoutSec: 5,
	})
	if err != nil || res.Status != a2abiz.FederationCallStatusSuccess {
		t.Fatalf("first invoke should succeed: res=%+v err=%v", res, err)
	}

	// second call exceeds daily quota → 429
	_, err = f.uc.InvokeFederated(ctx, a2abiz.FederatedInvokeInput{
		OrgID: org.ID, AgentID: "ra-1", Capability: "chat", PayloadJSON: `{}`, Workspace: "ws1",
	})
	if !apierror.IsCode(err, apierror.CodeRateLimit) {
		t.Fatalf("expected 429 rate limit, got %v", err)
	}
	if got := len(mock.received()); got != 1 {
		t.Fatalf("only the first call may reach the remote, got %d requests", got)
	}
	denied := findAudits(f.audits.snapshot(), a2abiz.DecisionDeniedQuota)
	if len(denied) != 1 {
		t.Fatalf("expected 1 denied_quota audit, got %+v", denied)
	}
}

// Criterion 7 + FED-F8: an org whose remote agent uses mTLS establishes the
// connection and completes the call.
func TestFederationE2E_InvokeMTLS(t *testing.T) {
	allowAllRemoteIPs(t)
	f := newFederationE2EFixture(t)
	ctx := context.Background()
	ca := newTestCA(t)
	mock := newMockMTLSEndpoint(t, ca)
	authConfigJSON := writeClientAuthFiles(t, ca)
	org := f.registerOrg(t, "Secure Org", "secure.example.com", a2abiz.TrustLevelTrusted)
	f.addRemote(org.ID, "ra-mtls", mock.url(), "mtls", authConfigJSON, "chat")

	res, err := f.uc.InvokeFederated(ctx, a2abiz.FederatedInvokeInput{
		OrgID: org.ID, AgentID: "ra-mtls", Capability: "chat",
		PayloadJSON: `{"text":"hello mtls"}`, Workspace: "ws1", TimeoutSec: 5,
	})
	if err != nil {
		t.Fatalf("mtls invoke: %v", err)
	}
	if res.Status != a2abiz.FederationCallStatusSuccess {
		t.Fatalf("expected mtls success, got %q (%s)", res.Status, res.ErrorMessage)
	}
	if !strings.Contains(res.ResultJSON, "pong: hello mtls") {
		t.Fatalf("unexpected mtls result: %s", res.ResultJSON)
	}
	if got := len(mock.received()); got != 1 {
		t.Fatalf("expected 1 mtls request, got %d", got)
	}
	allowed := findAudits(f.audits.snapshot(), a2abiz.DecisionAllowed)
	if len(allowed) != 1 || allowed[0].Status != a2abiz.FederationCallStatusSuccess {
		t.Fatalf("expected allowed+success audit for mtls, got %+v", allowed)
	}
}

// FED-F8: api_key auth config is honored — the remote endpoint receives the
// credential header.
func TestFederationE2E_InvokeAPIKeyAuth(t *testing.T) {
	allowAllRemoteIPs(t)
	f := newFederationE2EFixture(t)
	ctx := context.Background()
	mock := newMockA2AEndpoint(t)
	mock.requireKey = "secret-key"
	org := f.registerOrg(t, "Key Org", "key.example.com", a2abiz.TrustLevelTrusted)
	f.addRemote(org.ID, "ra-key", mock.url(), "api_key", `{"api_key":"secret-key"}`, "chat")

	res, err := f.uc.InvokeFederated(ctx, a2abiz.FederatedInvokeInput{
		OrgID: org.ID, AgentID: "ra-key", Capability: "chat", PayloadJSON: `{}`, Workspace: "ws1", TimeoutSec: 5,
	})
	if err != nil {
		t.Fatalf("api_key invoke: %v", err)
	}
	if res.Status != a2abiz.FederationCallStatusSuccess {
		t.Fatalf("expected api_key success, got %q (%s)", res.Status, res.ErrorMessage)
	}
	reqs := mock.received()
	if len(reqs) != 1 || reqs[0].Header.Get("X-Api-Key") != "secret-key" {
		t.Fatalf("remote did not receive api key header: %+v", reqs)
	}
}

// FED-F3: unreachable remote returns a clear error status (not a Go error);
// the audit row is finalized with status=error.
func TestFederationE2E_RemoteUnreachable(t *testing.T) {
	allowAllRemoteIPs(t)
	f := newFederationE2EFixture(t)
	ctx := context.Background()
	org := f.registerOrg(t, "Acme Corp", "acme.example.com", a2abiz.TrustLevelTrusted)
	// 127.0.0.1:1 is guaranteed closed (port 1 is reserved/unbound).
	f.addRemote(org.ID, "ra-down", "http://127.0.0.1:1/", "none", "", "chat")

	res, err := f.uc.InvokeFederated(ctx, a2abiz.FederatedInvokeInput{
		OrgID: org.ID, AgentID: "ra-down", Capability: "chat", PayloadJSON: `{}`, Workspace: "ws1", TimeoutSec: 2,
	})
	if err != nil {
		t.Fatalf("invoke failures must be reported in result, got Go error: %v", err)
	}
	if res.Status != a2abiz.FederationCallStatusError {
		t.Fatalf("expected error status, got %q", res.Status)
	}
	if res.ErrorMessage == "" {
		t.Fatal("expected error message for unreachable remote")
	}
	allowed := findAudits(f.audits.snapshot(), a2abiz.DecisionAllowed)
	if len(allowed) != 1 || allowed[0].Status != a2abiz.FederationCallStatusError || allowed[0].ErrorMessage == "" {
		t.Fatalf("expected allowed+error audit, got %+v", allowed)
	}
}

// Development acceptance: when the allowed-decision audit cannot be persisted,
// the invocation is rejected (fail-closed) before any remote contact.
func TestFederationE2E_AuditPersistFailureRejectsCall(t *testing.T) {
	allowAllRemoteIPs(t)
	f := newFederationE2EFixture(t)
	ctx := context.Background()
	mock := newMockA2AEndpoint(t)
	org := f.registerOrg(t, "Acme Corp", "acme.example.com", a2abiz.TrustLevelTrusted)
	f.addRemote(org.ID, "ra-1", mock.url(), "none", "", "chat")

	f.audits.failCreate = true
	_, err := f.uc.InvokeFederated(ctx, a2abiz.FederatedInvokeInput{
		OrgID: org.ID, AgentID: "ra-1", Capability: "chat", PayloadJSON: `{}`, Workspace: "ws1",
	})
	if !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("expected 500 internal on audit persist failure, got %v", err)
	}
	if got := len(mock.received()); got != 0 {
		t.Fatalf("remote must not be contacted when audit fails closed, got %d requests", got)
	}
}
