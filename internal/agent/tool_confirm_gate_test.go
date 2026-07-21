package agent

import (
	"context"
	"testing"
	"time"

	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"
)

func TestToolConfirmApprovedStructured(t *testing.T) {
	t.Parallel()
	if !toolConfirmApproved(serviceawaitreply.ReplyApprove) {
		t.Fatal("expected approve token to be approved")
	}
	if toolConfirmApproved(serviceawaitreply.ReplyDeny) {
		t.Fatal("expected deny token to be rejected")
	}
}

func TestToolConfirmApprovedTextFallback(t *testing.T) {
	t.Parallel()
	if !toolConfirmApproved("yes") {
		t.Fatal("expected text yes to be approved")
	}
	if toolConfirmApproved("no") {
		t.Fatal("expected text no to be rejected")
	}
}

func TestToolConfirmApprovedAlwaysTokens(t *testing.T) {
	t.Parallel()
	// Grant-scoped approve tokens must also count as approved for the
	// current invocation; the grant side effect is handled separately.
	if !toolConfirmApproved(serviceawaitreply.ReplyApproveSession) {
		t.Fatal("expected approve_session token to be approved")
	}
	if !toolConfirmApproved(serviceawaitreply.ReplyApproveAlways) {
		t.Fatal("expected approve_always token to be approved")
	}
}

func newTestGate(catalog map[string]confirmCatalogEntry, persisted func(ctx context.Context, agentID, toolKey string) bool) *toolConfirmGate {
	return &toolConfirmGate{
		catalog:        catalog,
		sessionGrants:  newToolGrantStore(time.Now),
		persistedGrant: persisted,
	}
}

func TestToolConfirmGate_Decide_DefaultAllow(t *testing.T) {
	t.Parallel()
	g := newTestGate(map[string]confirmCatalogEntry{
		"bash": {requiresConfirm: true},
	}, nil)
	d := g.decide(context.Background(), "sess-1", "agent-1", "file_read", nil)
	if d.needsConfirm || d.reason != confirmReasonDefaultAllow {
		t.Fatalf("decide = (%v,%q), want (false,%q)", d.needsConfirm, d.reason, confirmReasonDefaultAllow)
	}
}

func TestToolConfirmGate_Decide_PolicyCatalogWithoutGrant(t *testing.T) {
	t.Parallel()
	g := newTestGate(map[string]confirmCatalogEntry{
		"bash": {requiresConfirm: true},
	}, nil)
	d := g.decide(context.Background(), "sess-1", "agent-1", "bash", nil)
	if !d.needsConfirm || d.reason != confirmReasonPolicyCatalog {
		t.Fatalf("decide = (%v,%q), want (true,%q)", d.needsConfirm, d.reason, confirmReasonPolicyCatalog)
	}
}

func TestToolConfirmGate_Decide_SessionGrantHit(t *testing.T) {
	t.Parallel()
	g := newTestGate(map[string]confirmCatalogEntry{
		"bash": {requiresConfirm: true},
	}, nil)
	g.sessionGrants.GrantSession("sess-1", "agent-1", "bash")
	d := g.decide(context.Background(), "sess-1", "agent-1", "bash", nil)
	if d.needsConfirm || d.reason != confirmReasonGrantSession {
		t.Fatalf("decide = (%v,%q), want (false,%q)", d.needsConfirm, d.reason, confirmReasonGrantSession)
	}
}

func TestToolConfirmGate_Decide_PersistedGrantHit(t *testing.T) {
	t.Parallel()
	g := newTestGate(map[string]confirmCatalogEntry{
		"bash": {requiresConfirm: true},
	}, func(_ context.Context, agentID, toolKey string) bool {
		return agentID == "agent-1" && toolKey == "bash"
	})
	d := g.decide(context.Background(), "sess-1", "agent-1", "bash", nil)
	if d.needsConfirm || d.reason != confirmReasonGrantPersisted {
		t.Fatalf("decide = (%v,%q), want (false,%q)", d.needsConfirm, d.reason, confirmReasonGrantPersisted)
	}
}

func TestToolConfirmGate_Decide_SessionGrantIsolation(t *testing.T) {
	t.Parallel()
	g := newTestGate(map[string]confirmCatalogEntry{
		"bash": {requiresConfirm: true},
	}, nil)
	g.sessionGrants.GrantSession("sess-1", "agent-1", "bash")
	// A grant made in sess-1 must never leak into another session.
	d := g.decide(context.Background(), "sess-2", "agent-1", "bash", nil)
	if !d.needsConfirm || d.reason != confirmReasonPolicyCatalog {
		t.Fatalf("cross-session decide = (%v,%q), want (true,%q)", d.needsConfirm, d.reason, confirmReasonPolicyCatalog)
	}
}

func TestToolConfirmGate_Decide_PersistedBeforeSession(t *testing.T) {
	t.Parallel()
	g := newTestGate(map[string]confirmCatalogEntry{
		"bash": {requiresConfirm: true},
	}, func(_ context.Context, _, _ string) bool { return true })
	g.sessionGrants.GrantSession("sess-1", "agent-1", "bash")
	// Both tiers hit: persisted must win (Grok chain order).
	d := g.decide(context.Background(), "sess-1", "agent-1", "bash", nil)
	if d.needsConfirm || d.reason != confirmReasonGrantPersisted {
		t.Fatalf("decide = (%v,%q), want (false,%q)", d.needsConfirm, d.reason, confirmReasonGrantPersisted)
	}
}
