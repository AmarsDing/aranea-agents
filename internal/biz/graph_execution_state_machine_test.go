package biz

import "testing"

func TestIsGraphExecTerminalStatus(t *testing.T) {
	terminal := []string{GraphExecStatusCompleted, GraphExecStatusFailed, GraphExecStatusCancelled}
	for _, s := range terminal {
		if !IsGraphExecTerminalStatus(s) {
			t.Errorf("expected %q to be terminal", s)
		}
	}
	nonTerminal := []string{GraphExecStatusRunning, GraphExecStatusWaitingHuman}
	for _, s := range nonTerminal {
		if IsGraphExecTerminalStatus(s) {
			t.Errorf("expected %q to NOT be terminal", s)
		}
	}
}

func TestValidateGraphExecTransition(t *testing.T) {
	valid := [][2]string{
		{GraphExecStatusRunning, GraphExecStatusCompleted},
		{GraphExecStatusRunning, GraphExecStatusFailed},
		{GraphExecStatusRunning, GraphExecStatusCancelled},
		{GraphExecStatusRunning, GraphExecStatusWaitingHuman},
		{GraphExecStatusWaitingHuman, GraphExecStatusRunning},
		{GraphExecStatusWaitingHuman, GraphExecStatusFailed},
		{GraphExecStatusWaitingHuman, GraphExecStatusCancelled},
		{GraphExecStatusRunning, GraphExecStatusRunning},
	}
	for _, tc := range valid {
		if !ValidateGraphExecTransition(tc[0], tc[1]) {
			t.Errorf("expected transition %s → %s to be valid", tc[0], tc[1])
		}
	}
	invalid := [][2]string{
		{GraphExecStatusCompleted, GraphExecStatusRunning},
		{GraphExecStatusFailed, GraphExecStatusRunning},
		{GraphExecStatusCancelled, GraphExecStatusRunning},
		{GraphExecStatusCompleted, GraphExecStatusFailed},
		{GraphExecStatusFailed, GraphExecStatusCompleted},
	}
	for _, tc := range invalid {
		if ValidateGraphExecTransition(tc[0], tc[1]) {
			t.Errorf("expected transition %s → %s to be INVALID", tc[0], tc[1])
		}
	}
}

func TestGraphExecStateMachine(t *testing.T) {
	sm := NewGraphExecStateMachine()

	// Test Transition
	tests := []struct {
		from  GraphExecState
		event GraphExecEvent
		want  GraphExecState
		ok    bool
	}{
		{GraphExecStateRunning, GraphExecEventFinish, GraphExecStateCompleted, true},
		{GraphExecStateRunning, GraphExecEventFail, GraphExecStateFailed, true},
		{GraphExecStateRunning, GraphExecEventCancel, GraphExecStateCancelled, true},
		{GraphExecStateRunning, GraphExecEventAwaitHuman, GraphExecStateWaitingHuman, true},
		{GraphExecStateWaitingHuman, GraphExecEventResume, GraphExecStateRunning, true},
		{GraphExecStateWaitingHuman, GraphExecEventFail, GraphExecStateFailed, true},
		{GraphExecStateWaitingHuman, GraphExecEventCancel, GraphExecStateCancelled, true},
		{GraphExecStateCompleted, GraphExecEventFail, "", false},
		{GraphExecStateFailed, GraphExecEventResume, "", false},
		{GraphExecStateCancelled, GraphExecEventFinish, "", false},
	}
	for _, tt := range tests {
		got, err := sm.Transition(tt.from, tt.event)
		if tt.ok {
			if err != nil {
				t.Errorf("Transition(%s, %s): unexpected error: %v", tt.from, tt.event, err)
			} else if got != tt.want {
				t.Errorf("Transition(%s, %s) = %s; want %s", tt.from, tt.event, got, tt.want)
			}
		} else {
			if err == nil {
				t.Errorf("Transition(%s, %s): expected error, got nil", tt.from, tt.event)
			}
		}
	}

	// Test ValidTargets
	targets := sm.ValidTargets(GraphExecStateRunning)
	if len(targets) != 4 {
		t.Errorf("ValidTargets(Running) = %v; want 4 targets", targets)
	}
	targets = sm.ValidTargets(GraphExecStateCompleted)
	if len(targets) != 0 {
		t.Errorf("ValidTargets(Completed) = %v; want 0 targets", targets)
	}
}
