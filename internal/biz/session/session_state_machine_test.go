package session

import (
	"errors"
	"testing"

	"aranea-agents/internal/biz/shared"
)

func TestSessionStateMachine_ValidTransitions(t *testing.T) {
	sm := NewSessionStateMachine()

	tests := []struct {
		from   SessionState
		event  SessionEvent
		wantTo SessionState
	}{
		{SessionStateIdle, SessionEventStart, SessionStateRunning},
		{SessionStateRunning, SessionEventComplete, SessionStateCompleted},
		{SessionStateRunning, SessionEventInterrupt, SessionStateInterrupted},
		{SessionStateRunning, SessionEventAwaitConfirmation, SessionStateAwaitingConfirmation},
		{SessionStateCompleted, SessionEventStart, SessionStateRunning},
		{SessionStateInterrupted, SessionEventResume, SessionStateRunning},
		{SessionStateAwaitingConfirmation, SessionEventResume, SessionStateRunning},
		{SessionStateAwaitingConfirmation, SessionEventCancel, SessionStateInterrupted},
	}

	for _, tt := range tests {
		got, err := sm.Transition(tt.from, tt.event)
		if err != nil {
			t.Errorf("Transition(%s, %s): unexpected error: %v", tt.from, tt.event, err)
			continue
		}
		if got != tt.wantTo {
			t.Errorf("Transition(%s, %s) = %s, want %s", tt.from, tt.event, got, tt.wantTo)
		}
	}
}

func TestSessionStateMachine_InvalidTransitions(t *testing.T) {
	sm := NewSessionStateMachine()

	tests := []struct {
		from  SessionState
		event SessionEvent
	}{
		// No transition from idle except start
		{SessionStateIdle, SessionEventComplete},
		{SessionStateIdle, SessionEventInterrupt},
		{SessionStateIdle, SessionEventResume},
		// No resume from running
		{SessionStateRunning, SessionEventResume},
		// No cancel from running
		{SessionStateRunning, SessionEventCancel},
		// No complete from awaiting_confirmation
		{SessionStateAwaitingConfirmation, SessionEventComplete},
		// No start from interrupted
		{SessionStateInterrupted, SessionEventStart},
	}

	for _, tt := range tests {
		_, err := sm.Transition(tt.from, tt.event)
		if err == nil {
			t.Errorf("Transition(%s, %s): expected error, got nil", tt.from, tt.event)
			continue
		}
		if !errors.Is(err, shared.ErrInvalidTransition) {
			t.Errorf("Transition(%s, %s): error = %v, want ErrInvalidTransition", tt.from, tt.event, err)
		}
	}
}

func TestSessionStateMachine_CanTransition(t *testing.T) {
	sm := NewSessionStateMachine()

	tests := []struct {
		from SessionState
		to   SessionState
		want bool
	}{
		{SessionStateIdle, SessionStateRunning, true},
		{SessionStateRunning, SessionStateCompleted, true},
		{SessionStateRunning, SessionStateInterrupted, true},
		{SessionStateRunning, SessionStateAwaitingConfirmation, true},
		{SessionStateIdle, SessionStateCompleted, false},
		{SessionStateIdle, SessionStateInterrupted, false},
		{SessionStateCompleted, SessionStateInterrupted, false},
		{SessionStateInterrupted, SessionStateCompleted, false},
	}

	for _, tt := range tests {
		got := sm.CanTransition(tt.from, tt.to)
		if got != tt.want {
			t.Errorf("CanTransition(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestSessionStateMachine_ValidTargets(t *testing.T) {
	sm := NewSessionStateMachine()

	tests := []struct {
		from SessionState
		want []SessionState
	}{
		{SessionStateIdle, []SessionState{SessionStateRunning}},
		{SessionStateRunning, []SessionState{SessionStateAwaitingConfirmation, SessionStateCompleted, SessionStateInterrupted}},
		{SessionStateCompleted, []SessionState{SessionStateRunning}},
		{SessionStateInterrupted, []SessionState{SessionStateRunning}},
		{SessionStateAwaitingConfirmation, []SessionState{SessionStateInterrupted, SessionStateRunning}},
	}

	for _, tt := range tests {
		got := sm.ValidTargets(tt.from)
		if len(got) != len(tt.want) {
			t.Errorf("ValidTargets(%s) = %v, want %v", tt.from, got, tt.want)
			continue
		}
		for i, v := range got {
			if v != tt.want[i] {
				t.Errorf("ValidTargets(%s)[%d] = %s, want %s", tt.from, i, v, tt.want[i])
			}
		}
	}
}

// Compile-time interface check.
func TestSessionStateMachine_ImplementsInterface(t *testing.T) {
	var _ shared.StateMachine[SessionState, SessionEvent] = NewSessionStateMachine()
	t.Log("SessionStateMachine implements shared.StateMachine[SessionState, SessionEvent]")
}
