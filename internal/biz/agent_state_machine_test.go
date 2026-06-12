package biz

import (
	"testing"

	"aranea-agents/internal/biz/shared"
)

// Compile-time interface check.
var _ shared.StateMachine[AgentState, AgentEvent] = (*AgentStateMachine)(nil)

func newAgentSM(t *testing.T) *AgentStateMachine {
	t.Helper()
	return NewAgentStateMachine()
}

// ── Valid transitions ─────────────────────────────────────────────────────────

func TestAgentStateMachine_ValidTransitions(t *testing.T) {
	sm := newAgentSM(t)

	cases := []struct {
		from  AgentState
		event AgentEvent
		want  AgentState
	}{
		{AgentStateActive, AgentEventDeactivate, AgentStateInactive},
		{AgentStateInactive, AgentEventActivate, AgentStateActive},
		{AgentStateActive, AgentEventArchive, AgentStateArchived},
		{AgentStateInactive, AgentEventArchive, AgentStateArchived},
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

func TestAgentStateMachine_InvalidTransitions(t *testing.T) {
	sm := newAgentSM(t)

	cases := []struct {
		name  string
		from  AgentState
		event AgentEvent
	}{
		// Archived is terminal — no outgoing transitions
		{"archived→activate", AgentStateArchived, AgentEventActivate},
		{"archived→deactivate", AgentStateArchived, AgentEventDeactivate},
		{"archived→archive", AgentStateArchived, AgentEventArchive},

		// Active cannot activate (already active, no self-transition)
		{"active→activate", AgentStateActive, AgentEventActivate},

		// Inactive cannot deactivate (already inactive, no self-transition)
		{"inactive→deactivate", AgentStateInactive, AgentEventDeactivate},
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

func TestAgentStateMachine_CanTransition(t *testing.T) {
	sm := newAgentSM(t)

	cases := []struct {
		from, to AgentState
		want     bool
	}{
		{AgentStateActive, AgentStateInactive, true},
		{AgentStateActive, AgentStateArchived, true},
		{AgentStateInactive, AgentStateActive, true},
		{AgentStateInactive, AgentStateArchived, true},

		// Cannot reach from terminal states
		{AgentStateArchived, AgentStateActive, false},
		{AgentStateArchived, AgentStateInactive, false},

		// No self-transitions
		{AgentStateActive, AgentStateActive, false},
		{AgentStateInactive, AgentStateInactive, false},
		{AgentStateArchived, AgentStateArchived, false},
	}

	for _, tc := range cases {
		got := sm.CanTransition(tc.from, tc.to)
		if got != tc.want {
			t.Errorf("CanTransition(%q, %q) = %v, want %v", string(tc.from), string(tc.to), got, tc.want)
		}
	}
}

// ── ValidTargets ──────────────────────────────────────────────────────────────

func TestAgentStateMachine_ValidTargets(t *testing.T) {
	sm := newAgentSM(t)

	cases := []struct {
		from AgentState
		want []AgentState
	}{
		{AgentStateActive, []AgentState{AgentStateArchived, AgentStateInactive}},
		{AgentStateInactive, []AgentState{AgentStateActive, AgentStateArchived}},
		{AgentStateArchived, nil},
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

// ── ParseAgentState ───────────────────────────────────────────────────────────

func TestParseAgentState(t *testing.T) {
	cases := []struct {
		input string
		want  AgentState
	}{
		{"active", AgentStateActive},
		{"inactive", AgentStateInactive},
		{"archived", AgentStateArchived},
		{"unknown", AgentState("unknown")},
	}

	for _, tc := range cases {
		got := ParseAgentState(tc.input)
		if got != tc.want {
			t.Errorf("ParseAgentState(%q) = %q, want %q", tc.input, string(got), string(tc.want))
		}
	}
}

// ── IsAgentTerminal ───────────────────────────────────────────────────────────

func TestIsAgentTerminal(t *testing.T) {
	cases := []struct {
		state AgentState
		want  bool
	}{
		{AgentStateArchived, true},
		{AgentStateActive, false},
		{AgentStateInactive, false},
	}

	for _, tc := range cases {
		got := IsAgentTerminal(tc.state)
		if got != tc.want {
			t.Errorf("IsAgentTerminal(%q) = %v, want %v", string(tc.state), got, tc.want)
		}
	}
}

// ── Full lifecycle: Active → Inactive → Active → Archived

func TestAgentStateMachine_FullLifecycle(t *testing.T) {
	sm := newAgentSM(t)

	steps := []struct {
		from  AgentState
		event AgentEvent
		want  AgentState
	}{
		{AgentStateActive, AgentEventDeactivate, AgentStateInactive},
		{AgentStateInactive, AgentEventActivate, AgentStateActive},
		{AgentStateActive, AgentEventArchive, AgentStateArchived},
	}

	current := AgentStateActive
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

	if !IsAgentTerminal(current) {
		t.Fatalf("expected terminal state, got %q", string(current))
	}
}
