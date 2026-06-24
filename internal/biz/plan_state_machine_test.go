package biz

import (
	"errors"
	"testing"

	"aranea-agents/internal/biz/shared"
)

func TestPlanStateMachine_LegalTransitions(t *testing.T) {
	tests := []struct {
		name      string
		from      PlanStatus
		event     PlanEvent
		wantTo    PlanStatus
		wantError bool
	}{
		{"Draft→Approved", PlanStatusDraft, PlanEventApprove, PlanStatusApproved, false},
		{"Approved→Confirmed", PlanStatusApproved, PlanEventConfirm, PlanStatusConfirmed, false},
		{"Approved→Executing", PlanStatusApproved, PlanEventStart, PlanStatusExecuting, false},
		{"Confirmed→Executing", PlanStatusConfirmed, PlanEventStart, PlanStatusExecuting, false},
		{"Executing→Completed", PlanStatusExecuting, PlanEventComplete, PlanStatusCompleted, false},
		{"Executing→Failed", PlanStatusExecuting, PlanEventFail, PlanStatusFailed, false},
		{"Failed→Draft", PlanStatusFailed, PlanEventRetry, PlanStatusDraft, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := NewPlanStateMachine()
			got, err := sm.Transition(tt.from, tt.event)
			if tt.wantError {
				if err == nil {
					t.Fatalf("expected error, got nil (to=%s)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantTo {
				t.Fatalf("expected %s, got %s", tt.wantTo, got)
			}
		})
	}
}

func TestPlanStateMachine_IllegalTransitions(t *testing.T) {
	tests := []struct {
		name  string
		from  PlanStatus
		event PlanEvent
	}{
		// Terminal state has no outgoing transitions
		{"Completed→Draft", PlanStatusCompleted, PlanEventRetry},
		// Invalid transitions
		{"Draft→Executing (must approve first)", PlanStatusDraft, PlanEventStart},
		{"Draft→Completed (no direct path)", PlanStatusDraft, PlanEventComplete},
		{"Approved→Completed (must execute first)", PlanStatusApproved, PlanEventComplete},
		{"Confirmed→Completed (must execute first)", PlanStatusConfirmed, PlanEventComplete},
		{"Failed→Executing (must retry to draft first)", PlanStatusFailed, PlanEventStart},
		// Unknown event for state
		{"Executing→Approve (invalid for state)", PlanStatusExecuting, PlanEventApprove},
		{"Completed→Fail (terminal)", PlanStatusCompleted, PlanEventFail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := NewPlanStateMachine()
			_, err := sm.Transition(tt.from, tt.event)
			if err == nil {
				t.Fatalf("expected error for %s + %s, got nil", tt.from, tt.event)
			}
			if !errors.Is(err, shared.ErrInvalidTransition) {
				t.Fatalf("expected ErrInvalidTransition, got: %v", err)
			}
		})
	}
}

func TestPlanStateMachine_CanTransition(t *testing.T) {
	sm := NewPlanStateMachine()
	tests := []struct {
		from, to PlanStatus
		want     bool
	}{
		{PlanStatusDraft, PlanStatusApproved, true},
		{PlanStatusApproved, PlanStatusConfirmed, true},
		{PlanStatusApproved, PlanStatusExecuting, true},
		{PlanStatusConfirmed, PlanStatusExecuting, true},
		{PlanStatusExecuting, PlanStatusCompleted, true},
		{PlanStatusExecuting, PlanStatusFailed, true},
		{PlanStatusFailed, PlanStatusDraft, true},
		// Invalid direct transitions
		{PlanStatusDraft, PlanStatusExecuting, false},
		{PlanStatusDraft, PlanStatusCompleted, false},
		{PlanStatusCompleted, PlanStatusDraft, false},
		{PlanStatusCompleted, PlanStatusFailed, false},
		{PlanStatusFailed, PlanStatusExecuting, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"→"+string(tt.to), func(t *testing.T) {
			got := sm.CanTransition(tt.from, tt.to)
			if got != tt.want {
				t.Fatalf("CanTransition(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestPlanStateMachine_ValidTargets(t *testing.T) {
	sm := NewPlanStateMachine()
	tests := []struct {
		from        PlanStatus
		wantCount   int
		mustContain []PlanStatus
	}{
		{PlanStatusDraft, 1, []PlanStatus{PlanStatusApproved}},
		{PlanStatusApproved, 2, []PlanStatus{PlanStatusConfirmed, PlanStatusExecuting}},
		{PlanStatusConfirmed, 1, []PlanStatus{PlanStatusExecuting}},
		{PlanStatusExecuting, 2, []PlanStatus{PlanStatusCompleted, PlanStatusFailed}},
		{PlanStatusFailed, 1, []PlanStatus{PlanStatusDraft}},
		// Terminal state has no valid targets
		{PlanStatusCompleted, 0, nil},
	}

	for _, tt := range tests {
		t.Run(string(tt.from), func(t *testing.T) {
			targets := sm.ValidTargets(tt.from)
			if len(targets) != tt.wantCount {
				t.Fatalf("ValidTargets(%s) returned %d targets, want %d: %v", tt.from, len(targets), tt.wantCount, targets)
			}
			for _, want := range tt.mustContain {
				found := false
				for _, got := range targets {
					if got == want {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("ValidTargets(%s) missing %s", tt.from, want)
				}
			}
		})
	}
}

func TestIsPlanTerminal(t *testing.T) {
	tests := []struct {
		state PlanStatus
		want  bool
	}{
		{PlanStatusCompleted, true},
		{PlanStatusDraft, false},
		{PlanStatusApproved, false},
		{PlanStatusConfirmed, false},
		{PlanStatusExecuting, false},
		{PlanStatusFailed, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := IsPlanTerminal(tt.state); got != tt.want {
				t.Fatalf("IsPlanTerminal(%s) = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}
