package biz

import "testing"

// TestActivityStateMachine_ToolBlockedToCancelled verifies the P1-1 fix:
// ToolBlocked → Cancelled transition must be legal via cancel event.
func TestActivityStateMachine_ToolBlockedToCancelled(t *testing.T) {
	got, err := TransitionActivityStatus(ActivityStatusToolBlocked, ActivityTransitionCancel)
	if err != nil {
		t.Fatalf("ToolBlocked → Cancelled failed: %v", err)
	}
	if got != ActivityStatusCancelled {
		t.Fatalf("expected Cancelled, got %s", got)
	}
}

// TestActivityStateMachine_ToolRunningToCancelled verifies the P1-1 fix:
// ToolRunning → Cancelled transition must be legal via cancel event.
func TestActivityStateMachine_ToolRunningToCancelled(t *testing.T) {
	got, err := TransitionActivityStatus(ActivityStatusToolRunning, ActivityTransitionCancel)
	if err != nil {
		t.Fatalf("ToolRunning → Cancelled failed: %v", err)
	}
	if got != ActivityStatusCancelled {
		t.Fatalf("expected Cancelled, got %s", got)
	}
}

// TestActivityStateMachine_ToolBlockedToCompleted verifies the P1-1 fix:
// ToolBlocked → Completed (Done event) transition must be legal
// for the confirm activity approve path.
func TestActivityStateMachine_ToolBlockedToCompleted(t *testing.T) {
	got, err := TransitionActivityStatus(ActivityStatusToolBlocked, ActivityTransitionDone)
	if err != nil {
		t.Fatalf("ToolBlocked → Completed failed: %v", err)
	}
	if got != ActivityStatusCompleted {
		t.Fatalf("expected Completed, got %s", got)
	}
}

// TestActivityStateMachine_TerminalStatesHaveNoOutgoing verifies FSM3:
// terminal states have no outgoing transitions.
func TestActivityStateMachine_TerminalStatesHaveNoOutgoing(t *testing.T) {
	terminals := []ActivityStatus{
		ActivityStatusCompleted,
		ActivityStatusFailed,
		ActivityStatusCancelled,
		ActivityStatusInterrupted,
		ActivityStatusPartialFailure,
	}
	events := []ActivityTransitionEvent{
		ActivityTransitionStart,
		ActivityTransitionToolStart,
		ActivityTransitionToolEnd,
		ActivityTransitionToolBlock,
		ActivityTransitionToolUnblock,
		ActivityTransitionDone,
		ActivityTransitionFail,
		ActivityTransitionCancel,
		ActivityTransitionInterrupt,
		ActivityTransitionPartial,
	}
	for _, s := range terminals {
		for _, e := range events {
			if _, err := TransitionActivityStatus(s, e); err == nil {
				t.Fatalf("terminal state %s should have no outgoing transition via %s", s, e)
			}
		}
	}
}
