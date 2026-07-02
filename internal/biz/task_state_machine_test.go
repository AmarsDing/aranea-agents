package biz

import (
	"errors"
	"testing"

	"aranea-agents/internal/biz/shared"
)

func TestTaskStateMachine_LegalTransitions(t *testing.T) {
	tests := []struct {
		name      string
		from      GraphTaskStatus
		event     TaskTransitionEvent
		wantTo    GraphTaskStatus
		wantError bool
	}{
		// Pending transitions
		{"Pending→Claimed", GraphTaskStatusPending, TaskEventClaim, GraphTaskStatusClaimed, false},
		{"Pending→PendingAssignment", GraphTaskStatusPending, TaskEventAssignDynamic, GraphTaskStatusPendingAssignment, false},
		// PendingAssignment transitions
		{"PendingAssignment→Pending", GraphTaskStatusPendingAssignment, TaskEventReassign, GraphTaskStatusPending, false},
		{"PendingAssignment→Claimed", GraphTaskStatusPendingAssignment, TaskEventClaim, GraphTaskStatusClaimed, false},
		// Claimed transitions
		{"Claimed→Complete", GraphTaskStatusClaimed, TaskEventComplete, GraphTaskStatusComplete, false},
		{"Claimed→ReviewRequired", GraphTaskStatusClaimed, TaskEventCompleteNeedReview, GraphTaskStatusReviewRequired, false},
		{"Claimed→Blocked", GraphTaskStatusClaimed, TaskEventBlock, GraphTaskStatusBlocked, false},
		{"Claimed→TimedOut", GraphTaskStatusClaimed, TaskEventTimeout, GraphTaskStatusTimedOut, false},
		// Blocked transitions
		{"Blocked→Pending", GraphTaskStatusBlocked, TaskEventUnblock, GraphTaskStatusPending, false},
		// ReviewRequired transitions
		{"ReviewRequired→Complete", GraphTaskStatusReviewRequired, TaskEventApprove, GraphTaskStatusComplete, false},
		{"ReviewRequired→Claimed", GraphTaskStatusReviewRequired, TaskEventReject, GraphTaskStatusClaimed, false},
		// TimedOut transitions
		{"TimedOut→Pending", GraphTaskStatusTimedOut, TaskEventRetry, GraphTaskStatusPending, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := NewTaskStateMachine()
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

func TestTaskStateMachine_IllegalTransitions(t *testing.T) {
	tests := []struct {
		name  string
		from  GraphTaskStatus
		event TaskTransitionEvent
	}{
		// Terminal states have no outgoing transitions
		{"Complete→Claimed", GraphTaskStatusComplete, TaskEventClaim},
		{"Failed→Pending", GraphTaskStatusFailed, TaskEventRetry},
		{"Cancelled→Pending", GraphTaskStatusCancelled, TaskEventRetry},
		{"Crashed→Pending", GraphTaskStatusCrashed, TaskEventRetry},
		// Invalid transitions
		{"Pending→Complete (no direct path)", GraphTaskStatusPending, TaskEventComplete},
		{"Blocked→Claimed (must unblock first)", GraphTaskStatusBlocked, TaskEventClaim},
		{"ReviewRequired→Blocked (invalid)", GraphTaskStatusReviewRequired, TaskEventBlock},
		{"TimedOut→Claimed (must retry first)", GraphTaskStatusTimedOut, TaskEventClaim},
		// Unknown event for state
		{"Pending→Approve (invalid for state)", GraphTaskStatusPending, TaskEventApprove},
		{"Claimed→Retry (invalid for state)", GraphTaskStatusClaimed, TaskEventRetry},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := NewTaskStateMachine()
			_, err := sm.Transition(tt.from, tt.event)
			if err == nil {
				t.Fatalf("expected error for %s + %s, got nil", tt.from, tt.event)
			}
			if !isInvalidTransitionError(err) {
				t.Fatalf("expected ErrInvalidTransition, got: %v", err)
			}
		})
	}
}

func TestTaskStateMachine_CanTransition(t *testing.T) {
	sm := NewTaskStateMachine()
	tests := []struct {
		from, to GraphTaskStatus
		want     bool
	}{
		{GraphTaskStatusPending, GraphTaskStatusClaimed, true},
		{GraphTaskStatusClaimed, GraphTaskStatusComplete, true},
		{GraphTaskStatusClaimed, GraphTaskStatusBlocked, true},
		{GraphTaskStatusBlocked, GraphTaskStatusPending, true},
		{GraphTaskStatusReviewRequired, GraphTaskStatusComplete, true},
		{GraphTaskStatusTimedOut, GraphTaskStatusPending, true},
		// Invalid direct transitions
		{GraphTaskStatusPending, GraphTaskStatusComplete, false},
		{GraphTaskStatusComplete, GraphTaskStatusPending, false},
		{GraphTaskStatusBlocked, GraphTaskStatusClaimed, false},
		{GraphTaskStatusFailed, GraphTaskStatusPending, false},
		{GraphTaskStatusCancelled, GraphTaskStatusPending, false},
		{GraphTaskStatusCrashed, GraphTaskStatusPending, false},
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

func TestTaskStateMachine_ValidTargets(t *testing.T) {
	sm := NewTaskStateMachine()
	tests := []struct {
		from        GraphTaskStatus
		wantCount   int
		mustContain []GraphTaskStatus
	}{
		{GraphTaskStatusPending, 2, []GraphTaskStatus{GraphTaskStatusClaimed, GraphTaskStatusPendingAssignment}},
		{GraphTaskStatusClaimed, 4, []GraphTaskStatus{GraphTaskStatusComplete, GraphTaskStatusReviewRequired, GraphTaskStatusBlocked, GraphTaskStatusTimedOut}},
		{GraphTaskStatusBlocked, 1, []GraphTaskStatus{GraphTaskStatusPending}},
		{GraphTaskStatusReviewRequired, 2, []GraphTaskStatus{GraphTaskStatusComplete, GraphTaskStatusClaimed}},
		{GraphTaskStatusTimedOut, 1, []GraphTaskStatus{GraphTaskStatusPending}},
		// Terminal states have no valid targets
		{GraphTaskStatusComplete, 0, nil},
		{GraphTaskStatusFailed, 0, nil},
		{GraphTaskStatusCancelled, 0, nil},
		{GraphTaskStatusCrashed, 0, nil},
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

func TestIsTaskTerminal(t *testing.T) {
	tests := []struct {
		state GraphTaskStatus
		want  bool
	}{
		{GraphTaskStatusComplete, true},
		{GraphTaskStatusFailed, true},
		{GraphTaskStatusCancelled, true},
		{GraphTaskStatusCrashed, true},
		{GraphTaskStatusPending, false},
		{GraphTaskStatusClaimed, false},
		{GraphTaskStatusBlocked, false},
		{GraphTaskStatusReviewRequired, false},
		{GraphTaskStatusTimedOut, false},
		{GraphTaskStatusPendingAssignment, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := IsTaskTerminal(tt.state); got != tt.want {
				t.Fatalf("IsTaskTerminal(%s) = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}

// isInvalidTransitionError checks if err wraps ErrInvalidTransition.
func isInvalidTransitionError(err error) bool {
	return errors.Is(err, shared.ErrInvalidTransition)
}
