package biz

import (
	"aranea-agents/internal/biz/shared"
	"testing"
)

// Compile-time interface check.
var _ shared.StateMachine[GraphExecutionState, GraphExecutionEvent] = (*GraphExecutionStateMachine)(nil)

func newGraphExecSM(t *testing.T) *GraphExecutionStateMachine {
	t.Helper()
	return NewGraphExecutionStateMachine()
}

// ── Valid transitions ─────────────────────────────────────────────────────────

func TestGraphExecutionStateMachine_ValidTransitions(t *testing.T) {
	sm := newGraphExecSM(t)

	cases := []struct {
		from  GraphExecutionState
		event GraphExecutionEvent
		want  GraphExecutionState
	}{
		{GraphExecRunning, GraphExecEventComplete, GraphExecCompleted},
		{GraphExecRunning, GraphExecEventFail, GraphExecFailed},
		{GraphExecRunning, GraphExecEventCancel, GraphExecCancelled},
		{GraphExecRunning, GraphExecEventInterrupt, GraphExecWaitingHuman},
		{GraphExecWaitingHuman, GraphExecEventResume, GraphExecRunning},
		{GraphExecWaitingHuman, GraphExecEventCancel, GraphExecCancelled},
		{GraphExecWaitingHuman, GraphExecEventFail, GraphExecFailed},
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

func TestGraphExecutionStateMachine_InvalidTransitions(t *testing.T) {
	sm := newGraphExecSM(t)

	cases := []struct {
		name  string
		from  GraphExecutionState
		event GraphExecutionEvent
	}{
		// Terminal states should reject all events
		{"completed→complete", GraphExecCompleted, GraphExecEventComplete},
		{"completed→fail", GraphExecCompleted, GraphExecEventFail},
		{"completed→cancel", GraphExecCompleted, GraphExecEventCancel},
		{"completed→interrupt", GraphExecCompleted, GraphExecEventInterrupt},
		{"completed→resume", GraphExecCompleted, GraphExecEventResume},
		{"failed→complete", GraphExecFailed, GraphExecEventComplete},
		{"failed→fail", GraphExecFailed, GraphExecEventFail},
		{"failed→cancel", GraphExecFailed, GraphExecEventCancel},
		{"failed→interrupt", GraphExecFailed, GraphExecEventInterrupt},
		{"failed→resume", GraphExecFailed, GraphExecEventResume},
		{"cancelled→complete", GraphExecCancelled, GraphExecEventComplete},
		{"cancelled→fail", GraphExecCancelled, GraphExecEventFail},
		{"cancelled→cancel", GraphExecCancelled, GraphExecEventCancel},
		{"cancelled→interrupt", GraphExecCancelled, GraphExecEventInterrupt},
		{"cancelled→resume", GraphExecCancelled, GraphExecEventResume},

		// Running cannot resume (only waiting_human can)
		{"running→resume", GraphExecRunning, GraphExecEventResume},

		// WaitingHuman cannot complete directly (must resume first)
		{"waiting_human→complete", GraphExecWaitingHuman, GraphExecEventComplete},

		// WaitingHuman cannot interrupt again
		{"waiting_human→interrupt", GraphExecWaitingHuman, GraphExecEventInterrupt},
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

func TestGraphExecutionStateMachine_CanTransition(t *testing.T) {
	sm := newGraphExecSM(t)

	cases := []struct {
		from, to GraphExecutionState
		want     bool
	}{
		{GraphExecRunning, GraphExecCompleted, true},
		{GraphExecRunning, GraphExecFailed, true},
		{GraphExecRunning, GraphExecCancelled, true},
		{GraphExecRunning, GraphExecWaitingHuman, true},
		{GraphExecWaitingHuman, GraphExecRunning, true},
		{GraphExecWaitingHuman, GraphExecCancelled, true},
		{GraphExecWaitingHuman, GraphExecFailed, true},

		// Cannot reach any state from terminal states
		{GraphExecCompleted, GraphExecRunning, false},
		{GraphExecCompleted, GraphExecFailed, false},
		{GraphExecFailed, GraphExecRunning, false},
		{GraphExecCancelled, GraphExecRunning, false},

		// No self-transitions
		{GraphExecRunning, GraphExecRunning, false},
		{GraphExecWaitingHuman, GraphExecWaitingHuman, false},

		// WaitingHuman cannot reach completed directly (must resume first)
		{GraphExecWaitingHuman, GraphExecCompleted, false},
	}

	for _, tc := range cases {
		got := sm.CanTransition(tc.from, tc.to)
		if got != tc.want {
			t.Errorf("CanTransition(%q, %q) = %v, want %v", string(tc.from), string(tc.to), got, tc.want)
		}
	}
}

// ── ValidTargets ──────────────────────────────────────────────────────────────

func TestGraphExecutionStateMachine_ValidTargets(t *testing.T) {
	sm := newGraphExecSM(t)

	cases := []struct {
		from GraphExecutionState
		want []GraphExecutionState
	}{
		{GraphExecRunning, []GraphExecutionState{GraphExecCancelled, GraphExecCompleted, GraphExecFailed, GraphExecWaitingHuman}},
		{GraphExecWaitingHuman, []GraphExecutionState{GraphExecCancelled, GraphExecFailed, GraphExecRunning}},
		{GraphExecCompleted, nil},
		{GraphExecFailed, nil},
		{GraphExecCancelled, nil},
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

// ── ParseGraphExecutionState ──────────────────────────────────────────────────

func TestParseGraphExecutionState(t *testing.T) {
	cases := []struct {
		input string
		want  GraphExecutionState
	}{
		{"running", GraphExecRunning},
		{"completed", GraphExecCompleted},
		{"failed", GraphExecFailed},
		{"cancelled", GraphExecCancelled},
		{"waiting_human", GraphExecWaitingHuman},
		{"unknown", GraphExecutionState("unknown")},
	}

	for _, tc := range cases {
		got := ParseGraphExecutionState(tc.input)
		if got != tc.want {
			t.Errorf("ParseGraphExecutionState(%q) = %q, want %q", tc.input, string(got), string(tc.want))
		}
	}
}

// ── IsGraphExecutionTerminal ──────────────────────────────────────────────────

func TestIsGraphExecutionTerminal(t *testing.T) {
	cases := []struct {
		state GraphExecutionState
		want  bool
	}{
		{GraphExecCompleted, true},
		{GraphExecFailed, true},
		{GraphExecCancelled, true},
		{GraphExecRunning, false},
		{GraphExecWaitingHuman, false},
	}

	for _, tc := range cases {
		got := IsGraphExecutionTerminal(tc.state)
		if got != tc.want {
			t.Errorf("IsGraphExecutionTerminal(%q) = %v, want %v", string(tc.state), got, tc.want)
		}
	}
}

// ── Terminal states have no outgoing edges ────────────────────────────────────

func TestGraphExecutionStateMachine_TerminalStatesNoOutgoing(t *testing.T) {
	sm := newGraphExecSM(t)

	terminalStates := []GraphExecutionState{GraphExecCompleted, GraphExecFailed, GraphExecCancelled}
	allEvents := []GraphExecutionEvent{
		GraphExecEventComplete, GraphExecEventFail, GraphExecEventCancel,
		GraphExecEventInterrupt, GraphExecEventResume,
	}

	for _, state := range terminalStates {
		for _, event := range allEvents {
			_, err := sm.Transition(state, event)
			if err == nil {
				t.Errorf("terminal state %q should reject event %q", string(state), string(event))
			}
		}
		targets := sm.ValidTargets(state)
		if len(targets) != 0 {
			t.Errorf("terminal state %q should have no valid targets, got %v", string(state), targets)
		}
	}
}

// ── Full lifecycle: Running → WaitingHuman → Running → Completed ─────────────

func TestGraphExecutionStateMachine_FullLifecycle(t *testing.T) {
	sm := newGraphExecSM(t)

	steps := []struct {
		from  GraphExecutionState
		event GraphExecutionEvent
		want  GraphExecutionState
	}{
		{GraphExecRunning, GraphExecEventInterrupt, GraphExecWaitingHuman},
		{GraphExecWaitingHuman, GraphExecEventResume, GraphExecRunning},
		{GraphExecRunning, GraphExecEventComplete, GraphExecCompleted},
	}

	current := GraphExecRunning
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

	if !IsGraphExecutionTerminal(current) {
		t.Fatalf("expected terminal state, got %q", string(current))
	}
}
