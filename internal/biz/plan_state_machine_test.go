package biz

import (
	"errors"
	"testing"

	"aranea-agents/internal/biz/shared"
)

func TestPlanStateMachine_LegalTransitions(t *testing.T) {
	tests := []struct {
		name      string
		from      LegacyPlanStatus
		event     PlanEvent
		wantTo    LegacyPlanStatus
		wantError bool
	}{
		{"Draft→Approved", LegacyPlanStatusDraft, PlanEventApprove, LegacyPlanStatusApproved, false},
		{"Approved→Confirmed", LegacyPlanStatusApproved, PlanEventConfirm, LegacyPlanStatusConfirmed, false},
		{"Approved→Executing", LegacyPlanStatusApproved, PlanEventStart, LegacyPlanStatusExecuting, false},
		{"Confirmed→Executing", LegacyPlanStatusConfirmed, PlanEventStart, LegacyPlanStatusExecuting, false},
		{"Executing→Completed", LegacyPlanStatusExecuting, PlanEventComplete, LegacyPlanStatusCompleted, false},
		{"Executing→Failed", LegacyPlanStatusExecuting, PlanEventFail, LegacyPlanStatusFailed, false},
		{"Failed→Draft", LegacyPlanStatusFailed, PlanEventRetry, LegacyPlanStatusDraft, false},
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
		from  LegacyPlanStatus
		event PlanEvent
	}{
		// Terminal state has no outgoing transitions
		{"Completed→Draft", LegacyPlanStatusCompleted, PlanEventRetry},
		// Invalid transitions
		{"Draft→Executing (must approve first)", LegacyPlanStatusDraft, PlanEventStart},
		{"Draft→Completed (no direct path)", LegacyPlanStatusDraft, PlanEventComplete},
		{"Approved→Completed (must execute first)", LegacyPlanStatusApproved, PlanEventComplete},
		{"Confirmed→Completed (must execute first)", LegacyPlanStatusConfirmed, PlanEventComplete},
		{"Failed→Executing (must retry to draft first)", LegacyPlanStatusFailed, PlanEventStart},
		// Unknown event for state
		{"Executing→Approve (invalid for state)", LegacyPlanStatusExecuting, PlanEventApprove},
		{"Completed→Fail (terminal)", LegacyPlanStatusCompleted, PlanEventFail},
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
		from, to LegacyPlanStatus
		want     bool
	}{
		{LegacyPlanStatusDraft, LegacyPlanStatusApproved, true},
		{LegacyPlanStatusApproved, LegacyPlanStatusConfirmed, true},
		{LegacyPlanStatusApproved, LegacyPlanStatusExecuting, true},
		{LegacyPlanStatusConfirmed, LegacyPlanStatusExecuting, true},
		{LegacyPlanStatusExecuting, LegacyPlanStatusCompleted, true},
		{LegacyPlanStatusExecuting, LegacyPlanStatusFailed, true},
		{LegacyPlanStatusFailed, LegacyPlanStatusDraft, true},
		// Invalid direct transitions
		{LegacyPlanStatusDraft, LegacyPlanStatusExecuting, false},
		{LegacyPlanStatusDraft, LegacyPlanStatusCompleted, false},
		{LegacyPlanStatusCompleted, LegacyPlanStatusDraft, false},
		{LegacyPlanStatusCompleted, LegacyPlanStatusFailed, false},
		{LegacyPlanStatusFailed, LegacyPlanStatusExecuting, false},
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
		from        LegacyPlanStatus
		wantCount   int
		mustContain []LegacyPlanStatus
	}{
		{LegacyPlanStatusDraft, 1, []LegacyPlanStatus{LegacyPlanStatusApproved}},
		{LegacyPlanStatusApproved, 2, []LegacyPlanStatus{LegacyPlanStatusConfirmed, LegacyPlanStatusExecuting}},
		{LegacyPlanStatusConfirmed, 1, []LegacyPlanStatus{LegacyPlanStatusExecuting}},
		{LegacyPlanStatusExecuting, 2, []LegacyPlanStatus{LegacyPlanStatusCompleted, LegacyPlanStatusFailed}},
		{LegacyPlanStatusFailed, 1, []LegacyPlanStatus{LegacyPlanStatusDraft}},
		// Terminal state has no valid targets
		{LegacyPlanStatusCompleted, 0, nil},
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
		state LegacyPlanStatus
		want  bool
	}{
		{LegacyPlanStatusCompleted, true},
		{LegacyPlanStatusDraft, false},
		{LegacyPlanStatusApproved, false},
		{LegacyPlanStatusConfirmed, false},
		{LegacyPlanStatusExecuting, false},
		{LegacyPlanStatusFailed, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := IsPlanTerminal(tt.state); got != tt.want {
				t.Fatalf("IsPlanTerminal(%s) = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}
