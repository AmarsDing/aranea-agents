package a2a

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

type mockFederationAuditRepo struct {
	createFn  func(context.Context, FederationAuditLog) (FederationAuditLog, error)
	updateFn  func(context.Context, string, string, int64, string) error
	listFn    func(context.Context, FederationAuditFilter) ([]FederationAuditLog, int, error)
	countFn   func(context.Context, string, string, time.Time) (int, error)
	created   []FederationAuditLog
	updateCnt int
	lastSince time.Time
}

func (m *mockFederationAuditRepo) CreateAudit(ctx context.Context, l FederationAuditLog) (FederationAuditLog, error) {
	if m.createFn != nil {
		return m.createFn(ctx, l)
	}
	m.created = append(m.created, l)
	return l, nil
}

func (m *mockFederationAuditRepo) UpdateAuditResult(ctx context.Context, id, status string, latencyMs int64, errMsg string) error {
	m.updateCnt++
	if m.updateFn != nil {
		return m.updateFn(ctx, id, status, latencyMs, errMsg)
	}
	return nil
}

func (m *mockFederationAuditRepo) ListAudits(ctx context.Context, filter FederationAuditFilter) ([]FederationAuditLog, int, error) {
	if m.listFn != nil {
		return m.listFn(ctx, filter)
	}
	return nil, 0, nil
}

func (m *mockFederationAuditRepo) CountCallsSince(ctx context.Context, callerOrgID, calleeOrgID string, since time.Time) (int, error) {
	m.lastSince = since
	if m.countFn != nil {
		return m.countFn(ctx, callerOrgID, calleeOrgID, since)
	}
	return 0, nil
}

func TestAuditLogger_RecordAllowed(t *testing.T) {
	repo := &mockFederationAuditRepo{}
	l := NewAuditLogger(repo, loggateway.NewNoop())

	entry, err := l.RecordAllowed(context.Background(), FederationAuditLog{
		CallerOrgID:   FederationLocalOrgID,
		CalleeOrgID:   "org-b",
		CallerAgentID: "agent-1",
		CalleeAgentID: "remote-1",
		Capability:    "chat",
	})
	if err != nil {
		t.Fatalf("RecordAllowed() error: %v", err)
	}
	if entry.ID == "" {
		t.Error("entry.ID empty, want generated")
	}
	if entry.Decision != DecisionAllowed {
		t.Errorf("Decision = %q, want %q", entry.Decision, DecisionAllowed)
	}
	if entry.Status != FederationCallStatusPending {
		t.Errorf("Status = %q, want %q", entry.Status, FederationCallStatusPending)
	}
	if entry.Direction != AuditDirectionOutbound {
		t.Errorf("Direction = %q, want %q", entry.Direction, AuditDirectionOutbound)
	}
	if len(repo.created) != 1 {
		t.Fatalf("repo create calls = %d, want 1", len(repo.created))
	}
}

func TestAuditLogger_RecordAllowedKeepsProvidedID(t *testing.T) {
	repo := &mockFederationAuditRepo{}
	l := NewAuditLogger(repo, loggateway.NewNoop())
	entry, err := l.RecordAllowed(context.Background(), FederationAuditLog{
		ID:          "audit-fixed",
		CallerOrgID: FederationLocalOrgID,
		CalleeOrgID: "org-b",
		Decision:    DecisionAllowed,
	})
	if err != nil {
		t.Fatalf("RecordAllowed() error: %v", err)
	}
	if entry.ID != "audit-fixed" {
		t.Errorf("entry.ID = %q, want %q", entry.ID, "audit-fixed")
	}
}

func TestAuditLogger_RecordAllowedFailClosed(t *testing.T) {
	repo := &mockFederationAuditRepo{
		createFn: func(context.Context, FederationAuditLog) (FederationAuditLog, error) {
			return FederationAuditLog{}, errors.New("db down")
		},
	}
	l := NewAuditLogger(repo, loggateway.NewNoop())
	// FED-NFR1: an allowed decision whose audit cannot be persisted must
	// return an error so the caller aborts the invocation (fail-closed).
	if _, err := l.RecordAllowed(context.Background(), FederationAuditLog{
		CallerOrgID: FederationLocalOrgID,
		CalleeOrgID: "org-b",
	}); err == nil {
		t.Fatal("RecordAllowed() expected error (fail-closed), got nil")
	}
}

func TestAuditLogger_RecordDeniedBestEffort(t *testing.T) {
	repo := &mockFederationAuditRepo{}
	l := NewAuditLogger(repo, loggateway.NewNoop())
	l.RecordDenied(context.Background(), FederationAuditLog{
		CallerOrgID: FederationLocalOrgID,
		CalleeOrgID: "org-c",
		Capability:  "search",
		Decision:    DecisionDeniedTrust,
	})
	if len(repo.created) != 1 {
		t.Fatalf("repo create calls = %d, want 1", len(repo.created))
	}
	got := repo.created[0]
	if got.Decision != DecisionDeniedTrust {
		t.Errorf("Decision = %q, want %q", got.Decision, DecisionDeniedTrust)
	}
	if got.Status != FederationCallStatusPending {
		t.Errorf("Status = %q, want %q", got.Status, FederationCallStatusPending)
	}
	if got.ID == "" {
		t.Error("ID empty, want generated")
	}
}

func TestAuditLogger_RecordDeniedCreationFailureDoesNotPanic(t *testing.T) {
	repo := &mockFederationAuditRepo{
		createFn: func(context.Context, FederationAuditLog) (FederationAuditLog, error) {
			return FederationAuditLog{}, errors.New("db down")
		},
	}
	l := NewAuditLogger(repo, loggateway.NewNoop())
	// The call is already rejected; audit failure must only be logged, never
	// escalate (design F.5 AuditLogger semantics).
	l.RecordDenied(context.Background(), FederationAuditLog{
		CallerOrgID: FederationLocalOrgID,
		CalleeOrgID: "org-c",
		Decision:    DecisionDeniedPolicy,
	})
}

func TestAuditLogger_RecordResult(t *testing.T) {
	repo := &mockFederationAuditRepo{}
	l := NewAuditLogger(repo, loggateway.NewNoop())
	var gotID, gotStatus, gotErr string
	var gotLatency int64
	repo.updateFn = func(_ context.Context, id, status string, latencyMs int64, errMsg string) error {
		gotID, gotStatus, gotLatency, gotErr = id, status, latencyMs, errMsg
		return nil
	}
	l.RecordResult(context.Background(), "audit-1", FederationCallStatusSuccess, 123, "")
	if gotID != "audit-1" || gotStatus != FederationCallStatusSuccess || gotLatency != 123 || gotErr != "" {
		t.Errorf("UpdateAuditResult args = (%q, %q, %d, %q), want (audit-1, success, 123, \"\")",
			gotID, gotStatus, gotLatency, gotErr)
	}
}

func TestAuditLogger_RecordResultFailureDoesNotPanic(t *testing.T) {
	repo := &mockFederationAuditRepo{
		updateFn: func(context.Context, string, string, int64, string) error {
			return errors.New("db down")
		},
	}
	l := NewAuditLogger(repo, loggateway.NewNoop())
	// The invocation result already exists; an audit update failure must only
	// be logged (Warn), never rewrite the call outcome (design F.5).
	l.RecordResult(context.Background(), "audit-1", FederationCallStatusError, 5, "boom")
	if repo.updateCnt != 1 {
		t.Errorf("repo update calls = %d, want 1", repo.updateCnt)
	}
}
