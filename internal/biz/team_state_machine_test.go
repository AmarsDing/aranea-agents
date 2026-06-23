package biz

import (
	"testing"

	"aranea-agents/internal/biz/shared"
)

// Compile-time interface check.
var _ shared.StateMachine[TeamState, TeamEvent] = (*TeamStateMachine)(nil)

func newTeamSM(t *testing.T) *TeamStateMachine {
	t.Helper()
	return NewTeamStateMachine()
}

// ── Valid transitions ─────────────────────────────────────────────────────────

func TestTeamStateMachine_ValidTransitions(t *testing.T) {
	sm := newTeamSM(t)

	cases := []struct {
		from  TeamState
		event TeamEvent
		want  TeamState
	}{
		{TeamStatePending, TeamEventStart, TeamStateRunning},
		{TeamStatePending, TeamEventCancel, TeamStateCancelled},
		{TeamStatePending, TeamEventFail, TeamStateFailed},
		{TeamStateRunning, TeamEventComplete, TeamStateCompleted},
		{TeamStateRunning, TeamEventFail, TeamStateFailed},
		{TeamStateRunning, TeamEventCancel, TeamStateCancelled},
		{TeamStateRunning, TeamEventInterrupt, TeamStateInterrupted},
		{TeamStateRunning, TeamEventRework, TeamStatePending},
		{TeamStateInterrupted, TeamEventRecover, TeamStateRunning},
		{TeamStateCompleted, TeamEventArchive, TeamStateArchived},
		{TeamStateFailed, TeamEventArchive, TeamStateArchived},
		{TeamStateFailed, TeamEventRecover, TeamStatePending},
		{TeamStateCancelled, TeamEventArchive, TeamStateArchived},
		{TeamStateCancelled, TeamEventRecover, TeamStatePending},
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

func TestTeamStateMachine_InvalidTransitions(t *testing.T) {
	sm := newTeamSM(t)

	cases := []struct {
		name  string
		from  TeamState
		event TeamEvent
	}{
		// Archived is terminal — no outgoing transitions
		{"archived→start", TeamStateArchived, TeamEventStart},
		{"archived→recover", TeamStateArchived, TeamEventRecover},
		{"archived→archive", TeamStateArchived, TeamEventArchive},

		// Completed can only archive
		{"completed→start", TeamStateCompleted, TeamEventStart},
		{"completed→fail", TeamStateCompleted, TeamEventFail},
		{"completed→cancel", TeamStateCompleted, TeamEventCancel},
		{"completed→recover", TeamStateCompleted, TeamEventRecover},

		// Running cannot recover (only interrupted can)
		{"running→recover", TeamStateRunning, TeamEventRecover},
		{"running→archive", TeamStateRunning, TeamEventArchive},

		// Pending cannot complete/interrupt directly (can fail via B-01 fix)
		{"pending→complete", TeamStatePending, TeamEventComplete},
		{"pending→interrupt", TeamStatePending, TeamEventInterrupt},
		{"pending→archive", TeamStatePending, TeamEventArchive},
		{"pending→recover", TeamStatePending, TeamEventRecover},

		// Interrupted can only recover
		{"interrupted→start", TeamStateInterrupted, TeamEventStart},
		{"interrupted→complete", TeamStateInterrupted, TeamEventComplete},
		{"interrupted→fail", TeamStateInterrupted, TeamEventFail},
		{"interrupted→cancel", TeamStateInterrupted, TeamEventCancel},
		{"interrupted→archive", TeamStateInterrupted, TeamEventArchive},

		// Blocked has no transitions
		{"blocked→start", TeamStateBlocked, TeamEventStart},
		{"blocked→recover", TeamStateBlocked, TeamEventRecover},
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

func TestTeamStateMachine_CanTransition(t *testing.T) {
	sm := newTeamSM(t)

	cases := []struct {
		from, to TeamState
		want     bool
	}{
		{TeamStatePending, TeamStateRunning, true},
		{TeamStatePending, TeamStateCancelled, true},
		{TeamStatePending, TeamStateFailed, true},
		{TeamStateRunning, TeamStateCompleted, true},
		{TeamStateRunning, TeamStateFailed, true},
		{TeamStateRunning, TeamStateCancelled, true},
		{TeamStateRunning, TeamStateInterrupted, true},
		{TeamStateRunning, TeamStatePending, true},
		{TeamStateInterrupted, TeamStateRunning, true},
		{TeamStateCompleted, TeamStateArchived, true},
		{TeamStateFailed, TeamStateArchived, true},
		{TeamStateFailed, TeamStatePending, true},
		{TeamStateCancelled, TeamStateArchived, true},
		{TeamStateCancelled, TeamStatePending, true},

		// Cannot reach from terminal states
		{TeamStateArchived, TeamStatePending, false},
		{TeamStateArchived, TeamStateRunning, false},
		{TeamStateCompleted, TeamStateRunning, false},
		{TeamStateCompleted, TeamStateFailed, false},

		// No self-transitions
		{TeamStateRunning, TeamStateRunning, false},
		{TeamStatePending, TeamStatePending, false},
	}

	for _, tc := range cases {
		got := sm.CanTransition(tc.from, tc.to)
		if got != tc.want {
			t.Errorf("CanTransition(%q, %q) = %v, want %v", string(tc.from), string(tc.to), got, tc.want)
		}
	}
}

// ── ValidTargets ──────────────────────────────────────────────────────────────

func TestTeamStateMachine_ValidTargets(t *testing.T) {
	sm := newTeamSM(t)

	cases := []struct {
		from TeamState
		want []TeamState
	}{
		{TeamStatePending, []TeamState{TeamStateCancelled, TeamStateFailed, TeamStateRunning}},
		{TeamStateRunning, []TeamState{TeamStateCancelled, TeamStateCompleted, TeamStateFailed, TeamStateInterrupted, TeamStatePending}},
		{TeamStateInterrupted, []TeamState{TeamStateRunning}},
		{TeamStateCompleted, []TeamState{TeamStateArchived}},
		{TeamStateFailed, []TeamState{TeamStateArchived, TeamStatePending}},
		{TeamStateCancelled, []TeamState{TeamStateArchived, TeamStatePending}},
		{TeamStateArchived, nil},
		{TeamStateBlocked, nil},
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

// ── ParseTeamState ────────────────────────────────────────────────────────────

func TestParseTeamState(t *testing.T) {
	cases := []struct {
		input string
		want  TeamState
	}{
		{"pending", TeamStatePending},
		{"running", TeamStateRunning},
		{"completed", TeamStateCompleted},
		{"failed", TeamStateFailed},
		{"cancelled", TeamStateCancelled},
		{"interrupted", TeamStateInterrupted},
		{"archived", TeamStateArchived},
		{"blocked", TeamStateBlocked},
		{"unknown", TeamState("unknown")},
	}

	for _, tc := range cases {
		got := ParseTeamState(tc.input)
		if got != tc.want {
			t.Errorf("ParseTeamState(%q) = %q, want %q", tc.input, string(got), string(tc.want))
		}
	}
}

// ── IsTeamTerminal ────────────────────────────────────────────────────────────

func TestIsTeamTerminal(t *testing.T) {
	cases := []struct {
		state TeamState
		want  bool
	}{
		{TeamStateArchived, true},
		{TeamStateCompleted, false},
		{TeamStateFailed, false},
		{TeamStateCancelled, false},
		{TeamStatePending, false},
		{TeamStateRunning, false},
		{TeamStateInterrupted, false},
	}

	for _, tc := range cases {
		got := IsTeamTerminal(tc.state)
		if got != tc.want {
			t.Errorf("IsTeamTerminal(%q) = %v, want %v", string(tc.state), got, tc.want)
		}
	}
}

// ── Full lifecycle: Pending → Running → Interrupted → Running → Completed → Archived

func TestTeamStateMachine_FullLifecycle(t *testing.T) {
	sm := newTeamSM(t)

	steps := []struct {
		from  TeamState
		event TeamEvent
		want  TeamState
	}{
		{TeamStatePending, TeamEventStart, TeamStateRunning},
		{TeamStateRunning, TeamEventInterrupt, TeamStateInterrupted},
		{TeamStateInterrupted, TeamEventRecover, TeamStateRunning},
		{TeamStateRunning, TeamEventComplete, TeamStateCompleted},
		{TeamStateCompleted, TeamEventArchive, TeamStateArchived},
	}

	current := TeamStatePending
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

	if !IsTeamTerminal(current) {
		t.Fatalf("expected terminal state, got %q", string(current))
	}
}
