package biz

import (
	"testing"

	"aranea-agents/internal/biz/shared"
)

// Compile-time interface check.
var _ shared.StateMachine[TeamRunState, TeamRunEvent] = (*TeamRunStateMachine)(nil)

func newTeamRunSM(t *testing.T) *TeamRunStateMachine {
	t.Helper()
	return NewTeamRunStateMachine()
}

// ── Valid transitions ─────────────────────────────────────────────────────────

func TestTeamRunStateMachine_ValidTransitions(t *testing.T) {
	sm := newTeamRunSM(t)

	cases := []struct {
		from  TeamRunState
		event TeamRunEvent
		want  TeamRunState
	}{
		{TeamRunStatePending, TeamRunEventStart, TeamRunStateRunning},
		{TeamRunStatePending, TeamRunEventCancel, TeamRunStateCancelled},
		{TeamRunStateRunning, TeamRunEventAwaitHuman, TeamRunStateWaitingHuman},
		{TeamRunStateRunning, TeamRunEventSucceed, TeamRunStateSuccess},
		{TeamRunStateRunning, TeamRunEventFail, TeamRunStateFailed},
		{TeamRunStateRunning, TeamRunEventCancel, TeamRunStateCancelled},
		{TeamRunStateWaitingHuman, TeamRunEventResume, TeamRunStateRunning},
		{TeamRunStateWaitingHuman, TeamRunEventSucceed, TeamRunStateSuccess},
		{TeamRunStateWaitingHuman, TeamRunEventFail, TeamRunStateFailed},
		{TeamRunStateWaitingHuman, TeamRunEventCancel, TeamRunStateCancelled},
	}

	for _, tc := range cases {
		got, err := sm.Transition(tc.from, tc.event)
		if err != nil {
			t.Errorf("Transition(%q, %q): unexpected error: %v", string(tc.from), string(tc.event), err)
			continue
		}
		if got != tc.want {
			t.Errorf("Transition(%q, %q) = %q, want %q", string(tc.from), string(tc.event), string(got), string(tc.want))
		}
	}
}

// ── Invalid transitions ──────────────────────────────────────────────────────

func TestTeamRunStateMachine_InvalidTransitions(t *testing.T) {
	sm := newTeamRunSM(t)

	cases := []struct {
		name  string
		from  TeamRunState
		event TeamRunEvent
	}{
		// Terminal states reject all events
		{"success→start", TeamRunStateSuccess, TeamRunEventStart},
		{"success→succeed", TeamRunStateSuccess, TeamRunEventSucceed},
		{"success→fail", TeamRunStateSuccess, TeamRunEventFail},
		{"success→cancel", TeamRunStateSuccess, TeamRunEventCancel},
		{"failed→start", TeamRunStateFailed, TeamRunEventStart},
		{"failed→succeed", TeamRunStateFailed, TeamRunEventSucceed},
		{"failed→fail", TeamRunStateFailed, TeamRunEventFail},
		{"failed→cancel", TeamRunStateFailed, TeamRunEventCancel},
		{"cancelled→start", TeamRunStateCancelled, TeamRunEventStart},
		{"cancelled→succeed", TeamRunStateCancelled, TeamRunEventSucceed},

		// Pending cannot succeed/fail/await_human directly
		{"pending→succeed", TeamRunStatePending, TeamRunEventSucceed},
		{"pending→fail", TeamRunStatePending, TeamRunEventFail},
		{"pending→await_human", TeamRunStatePending, TeamRunEventAwaitHuman},
		{"pending→resume", TeamRunStatePending, TeamRunEventResume},

		// Running cannot resume (only waiting_human can)
		{"running→resume", TeamRunStateRunning, TeamRunEventResume},

		// WaitingHuman cannot start (only pending can)
		{"waiting_human→start", TeamRunStateWaitingHuman, TeamRunEventStart},

		// WaitingHuman cannot await_human again
		{"waiting_human→await_human", TeamRunStateWaitingHuman, TeamRunEventAwaitHuman},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sm.Transition(tc.from, tc.event)
			if err == nil {
				t.Errorf("Transition(%q, %q): expected error, got state %q", string(tc.from), string(tc.event), string(got))
			}
		})
	}
}

// ── CanTransition ─────────────────────────────────────────────────────────────

func TestTeamRunStateMachine_CanTransition(t *testing.T) {
	sm := newTeamRunSM(t)

	cases := []struct {
		from, to TeamRunState
		want     bool
	}{
		{TeamRunStatePending, TeamRunStateRunning, true},
		{TeamRunStatePending, TeamRunStateCancelled, true},
		{TeamRunStateRunning, TeamRunStateWaitingHuman, true},
		{TeamRunStateRunning, TeamRunStateSuccess, true},
		{TeamRunStateRunning, TeamRunStateFailed, true},
		{TeamRunStateRunning, TeamRunStateCancelled, true},
		{TeamRunStateWaitingHuman, TeamRunStateRunning, true},
		{TeamRunStateWaitingHuman, TeamRunStateSuccess, true},
		{TeamRunStateWaitingHuman, TeamRunStateFailed, true},
		{TeamRunStateWaitingHuman, TeamRunStateCancelled, true},

		// Cannot reach from terminal states
		{TeamRunStateSuccess, TeamRunStateRunning, false},
		{TeamRunStateSuccess, TeamRunStateFailed, false},
		{TeamRunStateFailed, TeamRunStateRunning, false},
		{TeamRunStateCancelled, TeamRunStateRunning, false},

		// No self-transitions
		{TeamRunStateRunning, TeamRunStateRunning, false},
		{TeamRunStatePending, TeamRunStatePending, false},
	}

	for _, tc := range cases {
		got := sm.CanTransition(tc.from, tc.to)
		if got != tc.want {
			t.Errorf("CanTransition(%q, %q) = %v, want %v", string(tc.from), string(tc.to), got, tc.want)
		}
	}
}

// ── ValidTargets ──────────────────────────────────────────────────────────────

func TestTeamRunStateMachine_ValidTargets(t *testing.T) {
	sm := newTeamRunSM(t)

	cases := []struct {
		from TeamRunState
		want []TeamRunState
	}{
		{TeamRunStatePending, []TeamRunState{TeamRunStateCancelled, TeamRunStateRunning}},
		{TeamRunStateRunning, []TeamRunState{TeamRunStateCancelled, TeamRunStateFailed, TeamRunStateSuccess, TeamRunStateWaitingHuman}},
		{TeamRunStateWaitingHuman, []TeamRunState{TeamRunStateCancelled, TeamRunStateFailed, TeamRunStateRunning, TeamRunStateSuccess}},
		{TeamRunStateSuccess, nil},
		{TeamRunStateFailed, nil},
		{TeamRunStateCancelled, nil},
	}

	for _, tc := range cases {
		got := sm.ValidTargets(tc.from)
		if len(got) != len(tc.want) {
			t.Errorf("ValidTargets(%q) = %v, want %v", string(tc.from), got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("ValidTargets(%q)[%d] = %q, want %q", string(tc.from), i, string(got[i]), string(tc.want[i]))
			}
		}
	}
}

// ── ParseTeamRunState ─────────────────────────────────────────────────────────

func TestParseTeamRunState(t *testing.T) {
	cases := []struct {
		input string
		want  TeamRunState
	}{
		{"pending", TeamRunStatePending},
		{"running", TeamRunStateRunning},
		{"success", TeamRunStateSuccess},
		{"failed", TeamRunStateFailed},
		{"cancelled", TeamRunStateCancelled},
		{"waiting_human", TeamRunStateWaitingHuman},
		{"unknown", TeamRunState("unknown")},
	}

	for _, tc := range cases {
		got := ParseTeamRunState(tc.input)
		if got != tc.want {
			t.Errorf("ParseTeamRunState(%q) = %q, want %q", tc.input, string(got), string(tc.want))
		}
	}
}

// ── IsTeamRunTerminal ─────────────────────────────────────────────────────────

func TestIsTeamRunTerminal(t *testing.T) {
	cases := []struct {
		state TeamRunState
		want  bool
	}{
		{TeamRunStateSuccess, true},
		{TeamRunStateFailed, true},
		{TeamRunStateCancelled, true},
		{TeamRunStatePending, false},
		{TeamRunStateRunning, false},
		{TeamRunStateWaitingHuman, false},
	}

	for _, tc := range cases {
		got := IsTeamRunTerminal(tc.state)
		if got != tc.want {
			t.Errorf("IsTeamRunTerminal(%q) = %v, want %v", string(tc.state), got, tc.want)
		}
	}
}

// ── Full lifecycle: Pending → Running → WaitingHuman → Running → Success ─────

func TestTeamRunStateMachine_FullLifecycle(t *testing.T) {
	sm := newTeamRunSM(t)

	steps := []struct {
		from  TeamRunState
		event TeamRunEvent
		want  TeamRunState
	}{
		{TeamRunStatePending, TeamRunEventStart, TeamRunStateRunning},
		{TeamRunStateRunning, TeamRunEventAwaitHuman, TeamRunStateWaitingHuman},
		{TeamRunStateWaitingHuman, TeamRunEventResume, TeamRunStateRunning},
		{TeamRunStateRunning, TeamRunEventSucceed, TeamRunStateSuccess},
	}

	current := TeamRunStatePending
	for _, step := range steps {
		got, err := sm.Transition(step.from, step.event)
		if err != nil {
			t.Fatalf("Transition(%q, %q): unexpected error: %v", string(step.from), string(step.event), err)
		}
		if got != step.want {
			t.Fatalf("Transition(%q, %q) = %q, want %q", string(step.from), string(step.event), string(got), string(step.want))
		}
		current = got
	}

	if !IsTeamRunTerminal(current) {
		t.Fatalf("expected terminal state, got %q", string(current))
	}
}
