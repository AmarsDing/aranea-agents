package biz

import (
	"context"
	"errors"
	"testing"

	sessstatus "aranea-agents/internal/biz/session"
)

// stubSessionStatePort is a test double for SessionStatePort. It records
// every transition so tests can assert ordering and reason values.
type stubSessionStatePort struct {
	transitions []transitionCall
	nextErr     error
}

type transitionCall struct {
	SessionID string
	Target    sessstatus.SessionStatus
	Reason    sessstatus.SessionStatusReason
}

func (s *stubSessionStatePort) GetSessionState(context.Context, string) (map[string]string, error) {
	return nil, nil
}
func (s *stubSessionStatePort) SaveSessionState(context.Context, string, map[string]string) error {
	return nil
}
func (s *stubSessionStatePort) PatchSessionState(context.Context, string, map[string]string, []string) error {
	return nil
}
func (s *stubSessionStatePort) GetSessionRevision(context.Context, string) (int64, error) {
	return 0, nil
}
func (s *stubSessionStatePort) BumpSessionRevision(context.Context, string) (int64, error) {
	return 0, nil
}
func (s *stubSessionStatePort) TransitionStatus(_ context.Context, sessionID string, target sessstatus.SessionStatus, reason sessstatus.SessionStatusReason) error {
	s.transitions = append(s.transitions, transitionCall{
		SessionID: sessionID,
		Target:    target,
		Reason:    reason,
	})
	return s.nextErr
}

func TestTurnLifecycleUsecase_TransitionStatus_nilReceiver(t *testing.T) {
	var u *TurnLifecycleUsecase
	// Must not panic.
	u.TransitionStatus(context.Background(), "s", sessstatus.SessionStatusRunning, "")
}

func TestTurnLifecycleUsecase_TransitionStatus_nilSessions(t *testing.T) {
	u := NewTurnLifecycleUsecase(TurnLifecycleUsecaseConfig{})
	// Must not panic when the underlying port is missing.
	u.TransitionStatus(context.Background(), "s", sessstatus.SessionStatusRunning, "")
}

func TestTurnLifecycleUsecase_TransitionStatus_emptySessionID(t *testing.T) {
	stub := &stubSessionStatePort{}
	u := NewTurnLifecycleUsecase(TurnLifecycleUsecaseConfig{Sessions: stub})
	u.TransitionStatus(context.Background(), "   ", sessstatus.SessionStatusRunning, "")
	if len(stub.transitions) != 0 {
		t.Fatalf("expected no transitions for blank sessionID, got %d", len(stub.transitions))
	}
}

func TestTurnLifecycleUsecase_TransitionStatus_passesArguments(t *testing.T) {
	stub := &stubSessionStatePort{}
	u := NewTurnLifecycleUsecase(TurnLifecycleUsecaseConfig{Sessions: stub})
	u.TransitionStatus(context.Background(), "s-1", sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonTimeout)

	if len(stub.transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(stub.transitions))
	}
	got := stub.transitions[0]
	if got.SessionID != "s-1" || got.Target != sessstatus.SessionStatusInterrupted || got.Reason != sessstatus.StatusReasonTimeout {
		t.Fatalf("got %+v", got)
	}
}

func TestTurnLifecycleUsecase_TransitionStatus_swallowsErrors(t *testing.T) {
	stub := &stubSessionStatePort{nextErr: errors.New("db down")}
	u := NewTurnLifecycleUsecase(TurnLifecycleUsecaseConfig{Sessions: stub})
	// Errors must NOT propagate; transitions are best-effort.
	// TransitionStatus returns nothing, so the test only verifies the call
	// does not panic and that the stub recorded the attempt.
	u.TransitionStatus(context.Background(), "s-1", sessstatus.SessionStatusRunning, "")
	if len(stub.transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(stub.transitions))
	}
}

func TestTurnLifecycleUsecase_ConvenienceMethods(t *testing.T) {
	stub := &stubSessionStatePort{}
	u := NewTurnLifecycleUsecase(TurnLifecycleUsecaseConfig{Sessions: stub})
	ctx := context.Background()

	u.MarkRunning(ctx, "s-1")
	u.MarkCompleted(ctx, "s-1")
	u.MarkInterrupted(ctx, "s-1", sessstatus.StatusReasonError)
	u.MarkAwaiting(ctx, "s-1", sessstatus.StatusReasonToolConfirmation)

	want := []transitionCall{
		{"s-1", sessstatus.SessionStatusRunning, ""},
		{"s-1", sessstatus.SessionStatusCompleted, ""},
		{"s-1", sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError},
		{"s-1", sessstatus.SessionStatusAwaitingConfirmation, sessstatus.StatusReasonToolConfirmation},
	}
	if len(stub.transitions) != len(want) {
		t.Fatalf("got %d transitions, want %d", len(stub.transitions), len(want))
	}
	for i, w := range want {
		if stub.transitions[i] != w {
			t.Errorf("transition[%d] = %+v, want %+v", i, stub.transitions[i], w)
		}
	}
}
