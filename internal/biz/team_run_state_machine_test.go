package biz

import (
	"testing"
)

func newTeamRunSM(t *testing.T) *TeamRunStateMachine {
	t.Helper()
	return NewTeamRunStateMachine()
}

// ── Valid transitions ─────────────────────────────────────────────────────────

func TestTeamRunStateMachine_ValidTransitions(t *testing.T) {
	sm := newTeamRunSM(t)

	cases := []struct {
		from TeamRunState
		to   TeamRunState
		want bool
	}{
		{TeamRunState(TeamRunStatusPending), TeamRunState(TeamRunStatusRunning), true},
		{TeamRunState(TeamRunStatusPending), TeamRunState(TeamRunStatusCancelled), true},
		{TeamRunState(TeamRunStatusRunning), TeamRunState(TeamRunStatusWaitingHuman), true},
		{TeamRunState(TeamRunStatusRunning), TeamRunState(TeamRunStatusSuccess), true},
		{TeamRunState(TeamRunStatusRunning), TeamRunState(TeamRunStatusFailed), true},
		{TeamRunState(TeamRunStatusRunning), TeamRunState(TeamRunStatusCancelled), true},
		{TeamRunState(TeamRunStatusWaitingHuman), TeamRunState(TeamRunStatusRunning), true},
		{TeamRunState(TeamRunStatusWaitingHuman), TeamRunState(TeamRunStatusSuccess), true},
		{TeamRunState(TeamRunStatusWaitingHuman), TeamRunState(TeamRunStatusFailed), true},
		{TeamRunState(TeamRunStatusWaitingHuman), TeamRunState(TeamRunStatusCancelled), true},
	}

	for _, tc := range cases {
		got := sm.CanTransition(tc.from, tc.to)
		if got != tc.want {
			t.Errorf("CanTransition(%q, %q) = %v, want %v", string(tc.from), string(tc.to), got, tc.want)
		}
	}
}

// ── Invalid transitions ──────────────────────────────────────────────────────

func TestTeamRunStateMachine_InvalidTransitions(t *testing.T) {
	sm := newTeamRunSM(t)

	cases := []struct {
		name string
		from TeamRunState
		to   TeamRunState
	}{
		// Terminal states reject all transitions
		{"success→running", TeamRunState(TeamRunStatusSuccess), TeamRunState(TeamRunStatusRunning)},
		{"success→failed", TeamRunState(TeamRunStatusSuccess), TeamRunState(TeamRunStatusFailed)},
		{"failed→running", TeamRunState(TeamRunStatusFailed), TeamRunState(TeamRunStatusRunning)},
		{"cancelled→running", TeamRunState(TeamRunStatusCancelled), TeamRunState(TeamRunStatusRunning)},

		// Pending cannot reach success/failed/waiting_human directly
		{"pending→success", TeamRunState(TeamRunStatusPending), TeamRunState(TeamRunStatusSuccess)},
		{"pending→failed", TeamRunState(TeamRunStatusPending), TeamRunState(TeamRunStatusFailed)},
		{"pending→waiting_human", TeamRunState(TeamRunStatusPending), TeamRunState(TeamRunStatusWaitingHuman)},

		// Running cannot resume (only waiting_human can)
		{"running→running", TeamRunState(TeamRunStatusRunning), TeamRunState(TeamRunStatusRunning)},

		// WaitingHuman cannot go back to pending
		{"waiting_human→pending", TeamRunState(TeamRunStatusWaitingHuman), TeamRunState(TeamRunStatusPending)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sm.CanTransition(tc.from, tc.to)
			if got {
				t.Errorf("CanTransition(%q, %q): expected false, got true", string(tc.from), string(tc.to))
			}
		})
	}
}

// ── TransitionTeamRunStatus ───────────────────────────────────────────────────

func TestTransitionTeamRunStatus(t *testing.T) {
	cases := []struct {
		from  string
		event TeamRunEvent
		want  string
	}{
		{TeamRunStatusPending, TeamRunEventStart, TeamRunStatusRunning},
		{TeamRunStatusRunning, TeamRunEventWaitHuman, TeamRunStatusWaitingHuman},
		{TeamRunStatusRunning, TeamRunEventComplete, TeamRunStatusSuccess},
		{TeamRunStatusRunning, TeamRunEventFail, TeamRunStatusFailed},
		{TeamRunStatusRunning, TeamRunEventCancel, TeamRunStatusCancelled},
		{TeamRunStatusWaitingHuman, TeamRunEventResume, TeamRunStatusRunning},
		{TeamRunStatusWaitingHuman, TeamRunEventComplete, TeamRunStatusSuccess},
		{TeamRunStatusWaitingHuman, TeamRunEventFail, TeamRunStatusFailed},
		{TeamRunStatusWaitingHuman, TeamRunEventCancel, TeamRunStatusCancelled},
	}

	for _, tc := range cases {
		got, err := TransitionTeamRunStatus(tc.from, tc.event)
		if err != nil {
			t.Errorf("TransitionTeamRunStatus(%q, %q): unexpected error: %v", tc.from, string(tc.event), err)
			continue
		}
		if got != tc.want {
			t.Errorf("TransitionTeamRunStatus(%q, %q) = %q, want %q", tc.from, string(tc.event), got, tc.want)
		}
	}
}

// ── Invalid transition events ─────────────────────────────────────────────────

func TestTransitionTeamRunStatus_Invalid(t *testing.T) {
	cases := []struct {
		name  string
		from  string
		event TeamRunEvent
	}{
		{"success→start", TeamRunStatusSuccess, TeamRunEventStart},
		{"success→complete", TeamRunStatusSuccess, TeamRunEventComplete},
		{"failed→start", TeamRunStatusFailed, TeamRunEventStart},
		{"cancelled→start", TeamRunStatusCancelled, TeamRunEventStart},
		{"pending→complete", TeamRunStatusPending, TeamRunEventComplete},
		{"pending→fail", TeamRunStatusPending, TeamRunEventFail},
		{"pending→resume", TeamRunStatusPending, TeamRunEventResume},
		{"running→resume", TeamRunStatusRunning, TeamRunEventResume},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := TransitionTeamRunStatus(tc.from, tc.event)
			if err == nil {
				t.Errorf("TransitionTeamRunStatus(%q, %q): expected error, got %q", tc.from, string(tc.event), got)
			}
		})
	}
}

// ── ValidateTeamRunStatusTransition ───────────────────────────────────────────

func TestValidateTeamRunStatusTransition(t *testing.T) {
	cases := []struct {
		from string
		to   string
		want bool // nil error
	}{
		{TeamRunStatusPending, TeamRunStatusRunning, true},
		{TeamRunStatusRunning, TeamRunStatusSuccess, true},
		{TeamRunStatusRunning, TeamRunStatusWaitingHuman, true},
		{TeamRunStatusWaitingHuman, TeamRunStatusRunning, true},
		{TeamRunStatusSuccess, TeamRunStatusRunning, false},
		{TeamRunStatusFailed, TeamRunStatusRunning, false},
		{TeamRunStatusPending, TeamRunStatusSuccess, false},
	}

	for _, tc := range cases {
		err := ValidateTeamRunStatusTransition(tc.from, tc.to)
		got := err == nil
		if got != tc.want {
			t.Errorf("ValidateTeamRunStatusTransition(%q, %q): error=%v, want nil=%v", tc.from, tc.to, err, tc.want)
		}
	}
}

// ── Full lifecycle: Pending → Running → WaitingHuman → Running → Success ─────

func TestTeamRunStateMachine_FullLifecycle(t *testing.T) {
	steps := []struct {
		from  string
		event TeamRunEvent
		want  string
	}{
		{TeamRunStatusPending, TeamRunEventStart, TeamRunStatusRunning},
		{TeamRunStatusRunning, TeamRunEventWaitHuman, TeamRunStatusWaitingHuman},
		{TeamRunStatusWaitingHuman, TeamRunEventResume, TeamRunStatusRunning},
		{TeamRunStatusRunning, TeamRunEventComplete, TeamRunStatusSuccess},
	}

	current := TeamRunStatusPending
	for _, step := range steps {
		got, err := TransitionTeamRunStatus(step.from, step.event)
		if err != nil {
			t.Fatalf("TransitionTeamRunStatus(%q, %q): unexpected error: %v", step.from, string(step.event), err)
		}
		if got != step.want {
			t.Fatalf("TransitionTeamRunStatus(%q, %q) = %q, want %q", step.from, string(step.event), got, step.want)
		}
		current = got
	}

	if !IsTeamRunTerminalStatus(current) {
		t.Fatalf("expected terminal state, got %q", current)
	}
}
