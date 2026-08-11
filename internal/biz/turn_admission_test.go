package biz

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz/usage"
	"aranea-agents/pkg/apierror"
)

// stubQuotaEnforcer is a test double for the QuotaEnforcer port. It allows
// tests to configure per-scope outcomes.
type stubQuotaEnforcer struct {
	quotaResults  map[string]usage.QuotaCheck
	quotaErr      error
	teamResult    error
	teamCheckSeen string
}

func (s *stubQuotaEnforcer) CheckQuota(_ context.Context, scopeType, scopeID string) (usage.QuotaCheck, error) {
	if s.quotaErr != nil {
		return usage.QuotaCheck{}, s.quotaErr
	}
	if v, ok := s.quotaResults[scopeType+":"+scopeID]; ok {
		return v, nil
	}
	// Default: allowed, no configured cap.
	return usage.QuotaCheck{Allowed: true, Reason: usage.QuotaCheckReasonNoQuota}, nil
}

func (s *stubQuotaEnforcer) CheckTeamMemberQuotas(_ context.Context, teamID string) error {
	s.teamCheckSeen = teamID
	return s.teamResult
}

func TestTurnAdmissionUsecase_EnforceChatTurnQuotas_nilUsecase(t *testing.T) {
	var u *TurnAdmissionUsecase
	if err := u.EnforceChatTurnQuotas(context.Background(), "agent-1", "user-1"); err != nil {
		t.Fatalf("nil usecase should be a no-op, got %v", err)
	}
}

func TestTurnAdmissionUsecase_EnforceChatTurnQuotas_nilQuota(t *testing.T) {
	u := NewTurnAdmissionUsecase(TurnAdmissionUsecaseConfig{})
	if err := u.EnforceChatTurnQuotas(context.Background(), "agent-1", "user-1"); err != nil {
		t.Fatalf("nil quota should be a no-op, got %v", err)
	}
}

func TestTurnAdmissionUsecase_EnforceChatTurnQuotas_skipsEmptyScopes(t *testing.T) {
	q := &stubQuotaEnforcer{}
	u := NewTurnAdmissionUsecase(TurnAdmissionUsecaseConfig{Quota: q})

	cases := []struct {
		name    string
		agentID string
		userID  string
	}{
		{"both_empty", "", ""},
		{"whitespace_both", "   ", "   "},
		{"only_agent", "agent-1", ""},
		{"only_user", "", "user-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := u.EnforceChatTurnQuotas(context.Background(), tc.agentID, tc.userID); err != nil {
				t.Fatalf("empty IDs should skip the check, got %v", err)
			}
		})
	}
}

func TestTurnAdmissionUsecase_EnforceChatTurnQuotas_firstFailingScope(t *testing.T) {
	q := &stubQuotaEnforcer{
		quotaResults: map[string]usage.QuotaCheck{
			"agent:agent-1": {Allowed: false, Reason: "agent quota exceeded"},
			"user:user-1":   {Allowed: true},
			"global:global": {Allowed: true},
		},
	}
	u := NewTurnAdmissionUsecase(TurnAdmissionUsecaseConfig{Quota: q})

	err := u.EnforceChatTurnQuotas(context.Background(), "agent-1", "user-1")
	if err == nil {
		t.Fatal("expected error from agent scope, got nil")
	}
	ae, ok := apierror.From(err)
	if !ok {
		t.Fatalf("expected apierror, got %T", err)
	}
	if ae.Domain != "USAGE_QUOTA" {
		t.Fatalf("expected USAGE_QUOTA domain, got %s", ae.Domain)
	}
}

func TestTurnAdmissionUsecase_EnforceChatTurnQuotas_globalFailure(t *testing.T) {
	q := &stubQuotaEnforcer{
		quotaResults: map[string]usage.QuotaCheck{
			"agent:agent-1": {Allowed: true},
			"user:user-1":   {Allowed: true},
			"global:global": {Allowed: false, Reason: "global quota exceeded"},
		},
	}
	u := NewTurnAdmissionUsecase(TurnAdmissionUsecaseConfig{Quota: q})
	err := u.EnforceChatTurnQuotas(context.Background(), "agent-1", "user-1")
	if err == nil {
		t.Fatal("expected error from global scope, got nil")
	}
	ae, ok := apierror.From(err)
	if !ok {
		t.Fatalf("expected USAGE_QUOTA, got %v", err)
	}
	if ae.Domain != "USAGE_QUOTA" {
		t.Fatalf("expected USAGE_QUOTA, got %s", ae.Domain)
	}
}

func TestTurnAdmissionUsecase_EnforceChatTurnQuotas_quotaCheckError(t *testing.T) {
	q := &stubQuotaEnforcer{quotaErr: errors.New("repo boom")}
	u := NewTurnAdmissionUsecase(TurnAdmissionUsecaseConfig{Quota: q})
	err := u.EnforceChatTurnQuotas(context.Background(), "agent-1", "user-1")
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestTurnAdmissionUsecase_EnforceTeamMemberQuotas(t *testing.T) {
	q := &stubQuotaEnforcer{teamResult: nil}
	u := NewTurnAdmissionUsecase(TurnAdmissionUsecaseConfig{Quota: q})
	if err := u.EnforceTeamMemberQuotas(context.Background(), "team-1"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if q.teamCheckSeen != "team-1" {
		t.Fatalf("expected teamID propagated, got %q", q.teamCheckSeen)
	}

	want := errors.New("member quota exceeded")
	q = &stubQuotaEnforcer{teamResult: want}
	u = NewTurnAdmissionUsecase(TurnAdmissionUsecaseConfig{Quota: q})
	if err := u.EnforceTeamMemberQuotas(context.Background(), "team-1"); !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}

func TestTurnAdmissionUsecase_EvaluateContextPressure_nilUsecase(t *testing.T) {
	var u *TurnAdmissionUsecase
	if got := u.EvaluateContextPressure(context.Background(), Session{ID: "s"}, EntryPointWeb); got.Pressure {
		t.Fatal("nil usecase should not signal pressure")
	}
}

func TestTurnAdmissionUsecase_EvaluateContextPressure_emptySession(t *testing.T) {
	u := NewTurnAdmissionUsecase(TurnAdmissionUsecaseConfig{})
	if got := u.EvaluateContextPressure(context.Background(), Session{}, EntryPointWeb); got.Pressure {
		t.Fatal("empty session should not signal pressure")
	}
}

func TestTurnAdmissionUsecase_EvaluateContextPressure_defaultThreshold(t *testing.T) {
	// No threshold resolver → falls back to DefaultContextAdmissionThreshold.
	u := NewTurnAdmissionUsecase(TurnAdmissionUsecaseConfig{})
	sess := Session{ID: "s", AgentID: "a-1", ContextUsedRatio: 0.7}
	got := u.EvaluateContextPressure(context.Background(), sess, EntryPointWeb)
	if !got.Pressure {
		t.Fatalf("ratio 0.7 >= default 0.6, expected pressure=true, got %+v", got)
	}
	if got.Threshold != DefaultContextAdmissionThreshold {
		t.Fatalf("expected default threshold %v, got %v", DefaultContextAdmissionThreshold, got.Threshold)
	}
}

func TestTurnAdmissionUsecase_EvaluateContextPressure_belowThreshold(t *testing.T) {
	u := NewTurnAdmissionUsecase(TurnAdmissionUsecaseConfig{})
	sess := Session{ID: "s", AgentID: "a-1", ContextUsedRatio: 0.3}
	got := u.EvaluateContextPressure(context.Background(), sess, EntryPointWeb)
	if got.Pressure {
		t.Fatalf("ratio 0.3 below default 0.6, expected pressure=false, got %+v", got)
	}
}
