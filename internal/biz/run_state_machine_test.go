package biz

import (
	"testing"
)

func newSM(t *testing.T) *RunStateMachine {
	t.Helper()
	return NewRunStateMachine()
}

// ── Valid transitions ─────────────────────────────────────────────────────────

func TestRunStateMachine_ValidTransitions(t *testing.T) {
	sm := newSM(t)

	cases := []struct {
		from  RunState
		event RunEvent
		want  RunState
	}{
		{RunStateNone, RunEventStart, RunStateRunning},
		{RunStateRunning, RunEventComplete, RunStateCompleted},
		{RunStateRunning, RunEventFail, RunStateFailed},
		{RunStateRunning, RunEventCancel, RunStateCancelled},
		{RunStateRunning, RunEventAwait, RunStateAwaitingUser},
		{RunStateAwaitingUser, RunEventResume, RunStateRunning},
		{RunStateAwaitingUser, RunEventCancel, RunStateCancelled},
		{RunStateAwaitingUser, RunEventFail, RunStateFailed},
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

func TestRunStateMachine_InvalidTransitions(t *testing.T) {
	sm := newSM(t)

	cases := []struct {
		name  string
		from  RunState
		event RunEvent
	}{
		// Terminal states should reject all events
		{"completed→running", RunStateCompleted, RunEventStart},
		{"completed→complete", RunStateCompleted, RunEventComplete},
		{"completed→fail", RunStateCompleted, RunEventFail},
		{"completed→cancel", RunStateCompleted, RunEventCancel},
		{"failed→running", RunStateFailed, RunEventStart},
		{"failed→complete", RunStateFailed, RunEventComplete},
		{"cancelled→running", RunStateCancelled, RunEventStart},
		{"cancelled→complete", RunStateCancelled, RunEventComplete},

		// None can only start
		{"none→complete", RunStateNone, RunEventComplete},
		{"none→fail", RunStateNone, RunEventFail},
		{"none→cancel", RunStateNone, RunEventCancel},
		{"none→await", RunStateNone, RunEventAwait},
		{"none→resume", RunStateNone, RunEventResume},

		// Running cannot resume (only awaiting_user can)
		{"running→resume", RunStateRunning, RunEventResume},

		// AwaitingUser cannot start (only None can)
		{"awaiting→start", RunStateAwaitingUser, RunEventStart},

		// AwaitingUser cannot await again
		{"awaiting→await", RunStateAwaitingUser, RunEventAwait},

		// AwaitingUser cannot complete directly (must resume first)
		{"awaiting→complete", RunStateAwaitingUser, RunEventComplete},
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

func TestRunStateMachine_CanTransition(t *testing.T) {
	sm := newSM(t)

	cases := []struct {
		from, to RunState
		want     bool
	}{
		{RunStateNone, RunStateRunning, true},
		{RunStateRunning, RunStateCompleted, true},
		{RunStateRunning, RunStateFailed, true},
		{RunStateRunning, RunStateCancelled, true},
		{RunStateRunning, RunStateAwaitingUser, true},
		{RunStateAwaitingUser, RunStateRunning, true},
		{RunStateAwaitingUser, RunStateCancelled, true},
		{RunStateAwaitingUser, RunStateFailed, true},

		// Cannot reach terminal states from other terminal states
		{RunStateCompleted, RunStateRunning, false},
		{RunStateCompleted, RunStateFailed, false},
		{RunStateFailed, RunStateRunning, false},
		{RunStateCancelled, RunStateRunning, false},

		// None can only go to Running
		{RunStateNone, RunStateCompleted, false},
		{RunStateNone, RunStateFailed, false},

		// No self-transitions
		{RunStateRunning, RunStateRunning, false},
		{RunStateNone, RunStateNone, false},
	}

	for _, tc := range cases {
		got := sm.CanTransition(tc.from, tc.to)
		if got != tc.want {
			t.Errorf("CanTransition(%q, %q) = %v, want %v", string(tc.from), string(tc.to), got, tc.want)
		}
	}
}

// ── ValidTargets ──────────────────────────────────────────────────────────────

func TestRunStateMachine_ValidTargets(t *testing.T) {
	sm := newSM(t)

	cases := []struct {
		from RunState
		want []RunState
	}{
		{RunStateNone, []RunState{RunStateRunning}},
		{RunStateRunning, []RunState{RunStateAwaitingUser, RunStateCancelled, RunStateCompleted, RunStateFailed}},
		{RunStateAwaitingUser, []RunState{RunStateCancelled, RunStateFailed, RunStateRunning}},
		{RunStateCompleted, nil},
		{RunStateFailed, nil},
		{RunStateCancelled, nil},
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

// ── ParseRunState ─────────────────────────────────────────────────────────────

func TestParseRunState(t *testing.T) {
	cases := []struct {
		input string
		want  RunState
	}{
		{"", RunStateNone},
		{"running", RunStateRunning},
		{"completed", RunStateCompleted},
		{"failed", RunStateFailed},
		{"cancelled", RunStateCancelled},
		{"awaiting_user", RunStateAwaitingUser},
		{"unknown", RunState("unknown")},
	}

	for _, tc := range cases {
		got := ParseRunState(tc.input)
		if got != tc.want {
			t.Errorf("ParseRunState(%q) = %q, want %q", tc.input, string(got), string(tc.want))
		}
	}
}

// ── IsRunTerminal ─────────────────────────────────────────────────────────────

func TestIsRunTerminal(t *testing.T) {
	cases := []struct {
		state RunState
		want  bool
	}{
		{RunStateCompleted, true},
		{RunStateFailed, true},
		{RunStateCancelled, true},
		{RunStateNone, false},
		{RunStateRunning, false},
		{RunStateAwaitingUser, false},
	}

	for _, tc := range cases {
		got := IsRunTerminal(tc.state)
		if got != tc.want {
			t.Errorf("IsRunTerminal(%q) = %v, want %v", string(tc.state), got, tc.want)
		}
	}
}

// ── RunStateNone can only transition to Running ──────────────────────────────

func TestRunStateMachine_NoneOnlyGoesToRunning(t *testing.T) {
	sm := newSM(t)

	// All events except start should fail from None
	events := []RunEvent{RunEventComplete, RunEventFail, RunEventCancel, RunEventAwait, RunEventResume}
	for _, ev := range events {
		_, err := sm.Transition(RunStateNone, ev)
		if err == nil {
			t.Errorf("Transition(None, %q): expected error", string(ev))
		}
	}

	// Start should succeed
	got, err := sm.Transition(RunStateNone, RunEventStart)
	if err != nil {
		t.Fatalf("Transition(None, start): unexpected error: %v", err)
	}
	if got != RunStateRunning {
		t.Fatalf("Transition(None, start) = %q, want %q", string(got), string(RunStateRunning))
	}
}

// ── Full lifecycle: None → Running → AwaitingUser → Running → Completed ─────

func TestRunStateMachine_FullLifecycle(t *testing.T) {
	sm := newSM(t)

	steps := []struct {
		from  RunState
		event RunEvent
		want  RunState
	}{
		{RunStateNone, RunEventStart, RunStateRunning},
		{RunStateRunning, RunEventAwait, RunStateAwaitingUser},
		{RunStateAwaitingUser, RunEventResume, RunStateRunning},
		{RunStateRunning, RunEventComplete, RunStateCompleted},
	}

	current := RunStateNone
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

	if !IsRunTerminal(current) {
		t.Fatalf("expected terminal state, got %q", string(current))
	}
}
