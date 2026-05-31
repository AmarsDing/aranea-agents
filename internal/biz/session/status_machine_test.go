package session

import (
	"testing"
)

func TestSessionStatusMachine_TransitionTo_ValidTransitions(t *testing.T) {
	tests := []struct {
		from   SessionStatus
		to     SessionStatus
		reason SessionStatusReason
	}{
		{SessionStatusIdle, SessionStatusRunning, ""},
		{SessionStatusRunning, SessionStatusCompleted, ""},
		{SessionStatusRunning, SessionStatusInterrupted, StatusReasonUserCancelled},
		{SessionStatusRunning, SessionStatusInterrupted, StatusReasonTimeout},
		{SessionStatusRunning, SessionStatusInterrupted, StatusReasonError},
		{SessionStatusRunning, SessionStatusInterrupted, StatusReasonBudgetEscalated},
		{SessionStatusRunning, SessionStatusInterrupted, StatusReasonContextOverflow},
		{SessionStatusRunning, SessionStatusInterrupted, StatusReasonServerShutdown},
		{SessionStatusRunning, SessionStatusInterrupted, StatusReasonUnexpectedShutdown},
		{SessionStatusRunning, SessionStatusAwaitingConfirmation, StatusReasonToolConfirmation},
		{SessionStatusRunning, SessionStatusAwaitingConfirmation, StatusReasonAgentAwaitingReply},
		{SessionStatusAwaitingConfirmation, SessionStatusRunning, ""},
		{SessionStatusAwaitingConfirmation, SessionStatusInterrupted, StatusReasonUserCancelled},
		{SessionStatusAwaitingConfirmation, SessionStatusInterrupted, StatusReasonConfirmationTimeout},
		{SessionStatusCompleted, SessionStatusRunning, ""},
		{SessionStatusInterrupted, SessionStatusRunning, ""},
	}
	for _, tt := range tests {
		m := NewSessionStatusMachine(tt.from, "", "")
		if err := m.TransitionTo(tt.to, tt.reason); err != nil {
			t.Errorf("TransitionTo(%s→%s) should succeed, got error: %v", tt.from, tt.to, err)
		}
		if m.Status() != tt.to {
			t.Errorf("after transition, status = %s, want %s", m.Status(), tt.to)
		}
		if tt.reason != "" && m.StatusReason() != tt.reason {
			t.Errorf("after transition, reason = %s, want %s", m.StatusReason(), tt.reason)
		}
	}
}

func TestSessionStatusMachine_TransitionTo_InvalidTransitions(t *testing.T) {
	tests := []struct {
		from SessionStatus
		to   SessionStatus
	}{
		{SessionStatusIdle, SessionStatusCompleted},
		{SessionStatusIdle, SessionStatusInterrupted},
		{SessionStatusIdle, SessionStatusAwaitingConfirmation},
		{SessionStatusCompleted, SessionStatusCompleted},
		{SessionStatusCompleted, SessionStatusInterrupted},
		{SessionStatusInterrupted, SessionStatusCompleted},
		{SessionStatusInterrupted, SessionStatusInterrupted},
		{SessionStatusRunning, SessionStatusIdle},
		{SessionStatusRunning, SessionStatusRunning},
	}
	for _, tt := range tests {
		m := NewSessionStatusMachine(tt.from, "", "")
		if err := m.TransitionTo(tt.to, ""); err == nil {
			t.Errorf("TransitionTo(%s→%s) should fail, but succeeded", tt.from, tt.to)
		}
	}
}

func TestSessionStatusMachine_IsProtected(t *testing.T) {
	if IsProtectedStatus(SessionStatusIdle) {
		t.Error("idle should not be protected")
	}
	if !IsProtectedStatus(SessionStatusRunning) {
		t.Error("running should be protected")
	}
	if !IsProtectedStatus(SessionStatusAwaitingConfirmation) {
		t.Error("awaiting_confirmation should be protected")
	}
	if IsProtectedStatus(SessionStatusCompleted) {
		t.Error("completed should not be protected")
	}
	if IsProtectedStatus(SessionStatusInterrupted) {
		t.Error("interrupted should not be protected")
	}
}

func TestSessionStatusMachine_CanTransitionTo(t *testing.T) {
	m := NewSessionStatusMachine(SessionStatusIdle, "", "")
	if !m.CanTransitionTo(SessionStatusRunning) {
		t.Error("idle should be able to transition to running")
	}
	if m.CanTransitionTo(SessionStatusCompleted) {
		t.Error("idle should not be able to transition to completed")
	}
}
