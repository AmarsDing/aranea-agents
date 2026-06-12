package biz

import (
	"testing"

	"aranea-agents/internal/biz/shared"
)

// Compile-time interface check.
var _ shared.StateMachine[EvolutionState, EvolutionEvent] = (*EvolutionStateMachine)(nil)

func newEvolutionSM(t *testing.T) *EvolutionStateMachine {
	t.Helper()
	return NewEvolutionStateMachine()
}

// ── Valid transitions ─────────────────────────────────────────────────────────

func TestEvolutionStateMachine_ValidTransitions(t *testing.T) {
	sm := newEvolutionSM(t)

	cases := []struct {
		from  EvolutionState
		event EvolutionEvent
		want  EvolutionState
	}{
		{EvolutionStatePending, EvolutionEventApply, EvolutionStateApplied},
		{EvolutionStatePending, EvolutionEventReject, EvolutionStateRejected},
		{EvolutionStateApplied, EvolutionEventRollback, EvolutionStateRolledBack},
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

func TestEvolutionStateMachine_InvalidTransitions(t *testing.T) {
	sm := newEvolutionSM(t)

	cases := []struct {
		name  string
		from  EvolutionState
		event EvolutionEvent
	}{
		// Rejected is terminal — no outgoing transitions
		{"rejected→apply", EvolutionStateRejected, EvolutionEventApply},
		{"rejected→reject", EvolutionStateRejected, EvolutionEventReject},
		{"rejected→rollback", EvolutionStateRejected, EvolutionEventRollback},

		// RolledBack is terminal — no outgoing transitions
		{"rolled_back→apply", EvolutionStateRolledBack, EvolutionEventApply},
		{"rolled_back→reject", EvolutionStateRolledBack, EvolutionEventReject},
		{"rolled_back→rollback", EvolutionStateRolledBack, EvolutionEventRollback},

		// Applied cannot apply (no self-transition)
		{"applied→apply", EvolutionStateApplied, EvolutionEventApply},

		// Pending cannot rollback (not applied yet)
		{"pending→rollback", EvolutionStatePending, EvolutionEventRollback},
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

func TestEvolutionStateMachine_CanTransition(t *testing.T) {
	sm := newEvolutionSM(t)

	cases := []struct {
		from, to EvolutionState
		want     bool
	}{
		{EvolutionStatePending, EvolutionStateApplied, true},
		{EvolutionStatePending, EvolutionStateRejected, true},
		{EvolutionStateApplied, EvolutionStateRolledBack, true},

		// Cannot reach from terminal states
		{EvolutionStateRejected, EvolutionStatePending, false},
		{EvolutionStateRejected, EvolutionStateApplied, false},
		{EvolutionStateRolledBack, EvolutionStatePending, false},
		{EvolutionStateRolledBack, EvolutionStateApplied, false},

		// No self-transitions
		{EvolutionStatePending, EvolutionStatePending, false},
		{EvolutionStateApplied, EvolutionStateApplied, false},
		{EvolutionStateRejected, EvolutionStateRejected, false},
		{EvolutionStateRolledBack, EvolutionStateRolledBack, false},
	}

	for _, tc := range cases {
		got := sm.CanTransition(tc.from, tc.to)
		if got != tc.want {
			t.Errorf("CanTransition(%q, %q) = %v, want %v", string(tc.from), string(tc.to), got, tc.want)
		}
	}
}

// ── ValidTargets ──────────────────────────────────────────────────────────────

func TestEvolutionStateMachine_ValidTargets(t *testing.T) {
	sm := newEvolutionSM(t)

	cases := []struct {
		from EvolutionState
		want []EvolutionState
	}{
		{EvolutionStatePending, []EvolutionState{EvolutionStateApplied, EvolutionStateRejected}},
		{EvolutionStateApplied, []EvolutionState{EvolutionStateRolledBack}},
		{EvolutionStateRejected, nil},
		{EvolutionStateRolledBack, nil},
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

// ── ParseEvolutionState ──────────────────────────────────────────────────────

func TestParseEvolutionState(t *testing.T) {
	cases := []struct {
		input string
		want  EvolutionState
	}{
		{"pending", EvolutionStatePending},
		{"applied", EvolutionStateApplied},
		{"rejected", EvolutionStateRejected},
		{"rolled_back", EvolutionStateRolledBack},
		{"unknown", EvolutionState("unknown")},
	}

	for _, tc := range cases {
		got := ParseEvolutionState(tc.input)
		if got != tc.want {
			t.Errorf("ParseEvolutionState(%q) = %q, want %q", tc.input, string(got), string(tc.want))
		}
	}
}

// ── IsEvolutionTerminal ──────────────────────────────────────────────────────

func TestIsEvolutionTerminal(t *testing.T) {
	cases := []struct {
		state EvolutionState
		want  bool
	}{
		{EvolutionStateRejected, true},
		{EvolutionStateRolledBack, true},
		{EvolutionStatePending, false},
		{EvolutionStateApplied, false},
	}

	for _, tc := range cases {
		got := IsEvolutionTerminal(tc.state)
		if got != tc.want {
			t.Errorf("IsEvolutionTerminal(%q) = %v, want %v", string(tc.state), got, tc.want)
		}
	}
}

// ── Full lifecycle: Pending → Applied → RolledBack; and Pending → Rejected

func TestEvolutionStateMachine_FullLifecycle(t *testing.T) {
	sm := newEvolutionSM(t)

	t.Run("apply_and_rollback", func(t *testing.T) {
		steps := []struct {
			from  EvolutionState
			event EvolutionEvent
			want  EvolutionState
		}{
			{EvolutionStatePending, EvolutionEventApply, EvolutionStateApplied},
			{EvolutionStateApplied, EvolutionEventRollback, EvolutionStateRolledBack},
		}

		current := EvolutionStatePending
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

		if !IsEvolutionTerminal(current) {
			t.Fatalf("expected terminal state, got %q", string(current))
		}
	})

	t.Run("reject", func(t *testing.T) {
		got, err := sm.Transition(EvolutionStatePending, EvolutionEventReject)
		if err != nil {
			t.Fatalf("Transition(%q, %q): unexpected error: %v", string(EvolutionStatePending), string(EvolutionEventReject), err)
		}
		if got != EvolutionStateRejected {
			t.Fatalf("Transition(%q, %q) = %q, want %q", string(EvolutionStatePending), string(EvolutionEventReject), string(got), string(EvolutionStateRejected))
		}
		if !IsEvolutionTerminal(got) {
			t.Fatalf("expected terminal state, got %q", string(got))
		}
	})
}
