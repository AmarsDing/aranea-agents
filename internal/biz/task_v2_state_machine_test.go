package biz

import (
	"testing"
)

// ── Transition table (P2-Y1) ────────────────────────────────────────────────

func TestTaskV2StateMachine_Transition(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		from    TaskStatus
		event   TaskV2TransitionEvent
		want    TaskStatus
		wantErr bool
	}{
		// pending
		{"Pending→Running", TaskStatusPending, TaskV2EventStart, TaskStatusRunning, false},
		{"Pending→Completed", TaskStatusPending, TaskV2EventComplete, TaskStatusCompleted, false},
		{"Pending→Failed", TaskStatusPending, TaskV2EventFail, TaskStatusFailed, false},
		{"Pending→Cancelled", TaskStatusPending, TaskV2EventCancel, TaskStatusCancelled, false},
		{"Pending→Interrupted (orphaned recovery)", TaskStatusPending, TaskV2EventInterrupt, TaskStatusInterrupted, false},
		{"Pending+Resume invalid", TaskStatusPending, TaskV2EventResume, "", true},
		// running
		{"Running→Completed", TaskStatusRunning, TaskV2EventComplete, TaskStatusCompleted, false},
		{"Running→Failed", TaskStatusRunning, TaskV2EventFail, TaskStatusFailed, false},
		{"Running→Cancelled", TaskStatusRunning, TaskV2EventCancel, TaskStatusCancelled, false},
		{"Running→Interrupted (orphaned recovery)", TaskStatusRunning, TaskV2EventInterrupt, TaskStatusInterrupted, false},
		{"Running+Start invalid (no re-start)", TaskStatusRunning, TaskV2EventStart, "", true},
		// interrupted
		{"Interrupted→Running (resume CAS)", TaskStatusInterrupted, TaskV2EventResume, TaskStatusRunning, false},
		{"Interrupted→Completed", TaskStatusInterrupted, TaskV2EventComplete, TaskStatusCompleted, false},
		{"Interrupted→Failed", TaskStatusInterrupted, TaskV2EventFail, TaskStatusFailed, false},
		{"Interrupted→Cancelled", TaskStatusInterrupted, TaskV2EventCancel, TaskStatusCancelled, false},
		{"Interrupted+Interrupt invalid", TaskStatusInterrupted, TaskV2EventInterrupt, "", true},
		// terminal states: no outgoing transitions
		{"Completed terminal", TaskStatusCompleted, TaskV2EventStart, "", true},
		{"Completed+Complete invalid (monotonic)", TaskStatusCompleted, TaskV2EventComplete, "", true},
		{"Failed terminal", TaskStatusFailed, TaskV2EventResume, "", true},
		{"Cancelled terminal", TaskStatusCancelled, TaskV2EventComplete, "", true},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := TransitionTaskV2Status(tt.from, tt.event)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("TransitionTaskV2Status(%s, %s): want error, got %s", tt.from, tt.event, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("TransitionTaskV2Status(%s, %s): %v", tt.from, tt.event, err)
			}
			if got != tt.want {
				t.Fatalf("TransitionTaskV2Status(%s, %s) = %s, want %s", tt.from, tt.event, got, tt.want)
			}
		})
	}
}

func TestTaskV2StateMachine_CanTransition(t *testing.T) {
	t.Parallel()
	tests := []struct {
		from, to TaskStatus
		want     bool
	}{
		{TaskStatusPending, TaskStatusRunning, true},
		{TaskStatusPending, TaskStatusInterrupted, true},
		{TaskStatusRunning, TaskStatusCompleted, true},
		{TaskStatusRunning, TaskStatusFailed, true},
		{TaskStatusRunning, TaskStatusCancelled, true},
		{TaskStatusInterrupted, TaskStatusRunning, true},
		{TaskStatusInterrupted, TaskStatusCompleted, true},
		// illegal
		{TaskStatusRunning, TaskStatusPending, false},
		{TaskStatusCompleted, TaskStatusRunning, false},
		{TaskStatusFailed, TaskStatusRunning, false},
		{TaskStatusCancelled, TaskStatusPending, false},
		{TaskStatusRunning, TaskStatusRunning, false},
		{TaskStatusInterrupted, TaskStatusInterrupted, false},
	}
	for _, tt := range tests {
		if got := CanTransitionTaskV2Status(tt.from, tt.to); got != tt.want {
			t.Errorf("CanTransitionTaskV2Status(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

// TestIsTerminalTaskV2Status pins the terminal set: completed/failed/cancelled
// are terminal; interrupted is NOT (resumable); unknown is NOT.
func TestIsTerminalTaskV2Status(t *testing.T) {
	t.Parallel()
	tests := []struct {
		state TaskStatus
		want  bool
	}{
		{TaskStatusCompleted, true},
		{TaskStatusFailed, true},
		{TaskStatusCancelled, true},
		{TaskStatusPending, false},
		{TaskStatusRunning, false},
		{TaskStatusInterrupted, false}, // resumable recovery placeholder
		{TaskStatus("bogus"), false},   // unknown statuses are not terminal
	}
	for _, tt := range tests {
		if got := IsTerminalTaskV2Status(tt.state); got != tt.want {
			t.Errorf("IsTerminalTaskV2Status(%s) = %v, want %v", tt.state, got, tt.want)
		}
	}
}

// TestTerminalTaskV2Statuses guards the persistence-layer CAS contract:
// CompleteTaskTerminal's StatusNotIn list must be exactly this set.
func TestTerminalTaskV2Statuses(t *testing.T) {
	t.Parallel()
	got := TerminalTaskV2Statuses()
	want := []TaskStatus{TaskStatusCancelled, TaskStatusCompleted, TaskStatusFailed} // sorted
	if len(got) != len(want) {
		t.Fatalf("TerminalTaskV2Statuses() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("TerminalTaskV2Statuses()[%d] = %s, want %s (full: %v)", i, got[i], want[i], got)
		}
	}
}
