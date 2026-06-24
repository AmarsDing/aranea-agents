package biz

import (
	"errors"
	"testing"

	"aranea-agents/internal/biz/shared"
)

func TestTaskStateMachine_LegalTransitions(t *testing.T) {
	tests := []struct {
		name      string
		from      TaskStatus
		event     TaskTransitionEvent
		wantTo    TaskStatus
		wantError bool
	}{
		// Pending transitions
		{"Pending→Claimed", TaskStatusPending, TaskEventClaim, TaskStatusClaimed, false},
		{"Pending→PendingAssignment", TaskStatusPending, TaskEventAssignDynamic, TaskStatusPendingAssignment, false},
		// PendingAssignment transitions
		{"PendingAssignment→Pending", TaskStatusPendingAssignment, TaskEventReassign, TaskStatusPending, false},
		{"PendingAssignment→Claimed", TaskStatusPendingAssignment, TaskEventClaim, TaskStatusClaimed, false},
		// Claimed transitions
		{"Claimed→Complete", TaskStatusClaimed, TaskEventComplete, TaskStatusComplete, false},
		{"Claimed→ReviewRequired", TaskStatusClaimed, TaskEventCompleteNeedReview, TaskStatusReviewRequired, false},
		{"Claimed→Blocked", TaskStatusClaimed, TaskEventBlock, TaskStatusBlocked, false},
		{"Claimed→TimedOut", TaskStatusClaimed, TaskEventTimeout, TaskStatusTimedOut, false},
		// Blocked transitions
		{"Blocked→Pending", TaskStatusBlocked, TaskEventUnblock, TaskStatusPending, false},
		// ReviewRequired transitions
		{"ReviewRequired→Complete", TaskStatusReviewRequired, TaskEventApprove, TaskStatusComplete, false},
		{"ReviewRequired→Claimed", TaskStatusReviewRequired, TaskEventReject, TaskStatusClaimed, false},
		// TimedOut transitions
		{"TimedOut→Pending", TaskStatusTimedOut, TaskEventRetry, TaskStatusPending, false},
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
		from  TaskStatus
		event TaskTransitionEvent
	}{
		// Terminal states have no outgoing transitions
		{"Complete→Claimed", TaskStatusComplete, TaskEventClaim},
		{"Failed→Pending", TaskStatusFailed, TaskEventRetry},
		{"Cancelled→Pending", TaskStatusCancelled, TaskEventRetry},
		{"Crashed→Pending", TaskStatusCrashed, TaskEventRetry},
		// Invalid transitions
		{"Pending→Complete (no direct path)", TaskStatusPending, TaskEventComplete},
		{"Blocked→Claimed (must unblock first)", TaskStatusBlocked, TaskEventClaim},
		{"ReviewRequired→Blocked (invalid)", TaskStatusReviewRequired, TaskEventBlock},
		{"TimedOut→Claimed (must retry first)", TaskStatusTimedOut, TaskEventClaim},
		// Unknown event for state
		{"Pending→Approve (invalid for state)", TaskStatusPending, TaskEventApprove},
		{"Claimed→Retry (invalid for state)", TaskStatusClaimed, TaskEventRetry},
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
		from, to TaskStatus
		want     bool
	}{
		{TaskStatusPending, TaskStatusClaimed, true},
		{TaskStatusClaimed, TaskStatusComplete, true},
		{TaskStatusClaimed, TaskStatusBlocked, true},
		{TaskStatusBlocked, TaskStatusPending, true},
		{TaskStatusReviewRequired, TaskStatusComplete, true},
		{TaskStatusTimedOut, TaskStatusPending, true},
		// Invalid direct transitions
		{TaskStatusPending, TaskStatusComplete, false},
		{TaskStatusComplete, TaskStatusPending, false},
		{TaskStatusBlocked, TaskStatusClaimed, false},
		{TaskStatusFailed, TaskStatusPending, false},
		{TaskStatusCancelled, TaskStatusPending, false},
		{TaskStatusCrashed, TaskStatusPending, false},
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
		from       TaskStatus
		wantCount  int
		mustContain []TaskStatus
	}{
		{TaskStatusPending, 2, []TaskStatus{TaskStatusClaimed, TaskStatusPendingAssignment}},
		{TaskStatusClaimed, 4, []TaskStatus{TaskStatusComplete, TaskStatusReviewRequired, TaskStatusBlocked, TaskStatusTimedOut}},
		{TaskStatusBlocked, 1, []TaskStatus{TaskStatusPending}},
		{TaskStatusReviewRequired, 2, []TaskStatus{TaskStatusComplete, TaskStatusClaimed}},
		{TaskStatusTimedOut, 1, []TaskStatus{TaskStatusPending}},
		// Terminal states have no valid targets
		{TaskStatusComplete, 0, nil},
		{TaskStatusFailed, 0, nil},
		{TaskStatusCancelled, 0, nil},
		{TaskStatusCrashed, 0, nil},
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
		state TaskStatus
		want  bool
	}{
		{TaskStatusComplete, true},
		{TaskStatusFailed, true},
		{TaskStatusCancelled, true},
		{TaskStatusCrashed, true},
		{TaskStatusPending, false},
		{TaskStatusClaimed, false},
		{TaskStatusBlocked, false},
		{TaskStatusReviewRequired, false},
		{TaskStatusTimedOut, false},
		{TaskStatusPendingAssignment, false},
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
