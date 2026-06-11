package biz

import (
	"testing"
)

// ── Valid transitions ─────────────────────────────────────────────────────────

func TestSessionRunPhaseMachine_Transition_Valid(t *testing.T) {
	m := NewSessionRunPhaseMachine()
	tests := []struct {
		from  SessionRunPhase
		event SessionRunPhaseEvent
		want  SessionRunPhase
	}{
		{PhaseInteractive, PhaseEventEscalate, PhaseEscalating},
		{PhaseEscalating, PhaseEventDurable, PhaseDurable},
		{PhaseInteractive, PhaseEventComplete, PhaseCompleted},
		{PhaseInteractive, PhaseEventFail, PhaseFailed},
		{PhaseEscalating, PhaseEventComplete, PhaseCompleted},
		{PhaseEscalating, PhaseEventFail, PhaseFailed},
		{PhaseDurable, PhaseEventComplete, PhaseCompleted},
		{PhaseDurable, PhaseEventFail, PhaseFailed},
	}
	for _, tt := range tests {
		got, err := m.Transition(tt.from, tt.event)
		if err != nil {
			t.Errorf("Transition(%s, %s) unexpected error: %v", tt.from, tt.event, err)
		}
		if got != tt.want {
			t.Errorf("Transition(%s, %s) = %s, want %s", tt.from, tt.event, got, tt.want)
		}
	}
}

// ── Invalid transitions ──────────────────────────────────────────────────────

func TestSessionRunPhaseMachine_Transition_Invalid(t *testing.T) {
	m := NewSessionRunPhaseMachine()
	tests := []struct {
		from  SessionRunPhase
		event SessionRunPhaseEvent
	}{
		// Terminal phases have no outgoing transitions.
		{PhaseCompleted, PhaseEventComplete},
		{PhaseCompleted, PhaseEventFail},
		{PhaseCompleted, PhaseEventEscalate},
		{PhaseCompleted, PhaseEventDurable},
		{PhaseFailed, PhaseEventComplete},
		{PhaseFailed, PhaseEventFail},
		{PhaseFailed, PhaseEventEscalate},
		{PhaseFailed, PhaseEventDurable},
		// Wrong events for the current phase.
		{PhaseInteractive, PhaseEventDurable},
		{PhaseEscalating, PhaseEventEscalate},
		{PhaseDurable, PhaseEventEscalate},
		{PhaseDurable, PhaseEventDurable},
	}
	for _, tt := range tests {
		_, err := m.Transition(tt.from, tt.event)
		if err == nil {
			t.Errorf("Transition(%s, %s) should fail, but succeeded", tt.from, tt.event)
		}
	}
}

// ── CanTransition ─────────────────────────────────────────────────────────────

func TestSessionRunPhaseMachine_CanTransition(t *testing.T) {
	m := NewSessionRunPhaseMachine()
	tests := []struct {
		from  SessionRunPhase
		to    SessionRunPhase
		valid bool
	}{
		{PhaseInteractive, PhaseEscalating, true},
		{PhaseInteractive, PhaseCompleted, true},
		{PhaseInteractive, PhaseFailed, true},
		{PhaseEscalating, PhaseDurable, true},
		{PhaseEscalating, PhaseCompleted, true},
		{PhaseEscalating, PhaseFailed, true},
		{PhaseDurable, PhaseCompleted, true},
		{PhaseDurable, PhaseFailed, true},
		// Invalid
		{PhaseInteractive, PhaseDurable, false},
		{PhaseInteractive, PhaseInteractive, false},
		{PhaseEscalating, PhaseInteractive, false},
		{PhaseEscalating, PhaseEscalating, false},
		{PhaseDurable, PhaseInteractive, false},
		{PhaseDurable, PhaseEscalating, false},
		{PhaseDurable, PhaseDurable, false},
		{PhaseCompleted, PhaseInteractive, false},
		{PhaseCompleted, PhaseEscalating, false},
		{PhaseCompleted, PhaseDurable, false},
		{PhaseCompleted, PhaseCompleted, false},
		{PhaseCompleted, PhaseFailed, false},
		{PhaseFailed, PhaseInteractive, false},
		{PhaseFailed, PhaseEscalating, false},
		{PhaseFailed, PhaseDurable, false},
		{PhaseFailed, PhaseCompleted, false},
		{PhaseFailed, PhaseFailed, false},
	}
	for _, tt := range tests {
		got := m.CanTransition(tt.from, tt.to)
		if got != tt.valid {
			t.Errorf("CanTransition(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.valid)
		}
	}
}

// ── ValidTargets ──────────────────────────────────────────────────────────────

func TestSessionRunPhaseMachine_ValidTargets(t *testing.T) {
	m := NewSessionRunPhaseMachine()
	tests := []struct {
		from       SessionRunPhase
		wantCount  int
		wantSubset []SessionRunPhase
	}{
		{PhaseInteractive, 3, []SessionRunPhase{PhaseEscalating, PhaseCompleted, PhaseFailed}},
		{PhaseEscalating, 3, []SessionRunPhase{PhaseDurable, PhaseCompleted, PhaseFailed}},
		{PhaseDurable, 2, []SessionRunPhase{PhaseCompleted, PhaseFailed}},
		{PhaseCompleted, 0, nil},
		{PhaseFailed, 0, nil},
	}
	for _, tt := range tests {
		targets := m.ValidTargets(tt.from)
		if len(targets) != tt.wantCount {
			t.Errorf("ValidTargets(%s) returned %d targets, want %d", tt.from, len(targets), tt.wantCount)
		}
		// Verify all expected targets are present (order not guaranteed).
		targetSet := make(map[SessionRunPhase]struct{}, len(targets))
		for _, t := range targets {
			targetSet[t] = struct{}{}
		}
		for _, want := range tt.wantSubset {
			if _, ok := targetSet[want]; !ok {
				t.Errorf("ValidTargets(%s) missing expected target %s", tt.from, want)
			}
		}
	}
}

// ── Terminal phases have no valid targets ─────────────────────────────────────

func TestSessionRunPhaseMachine_TerminalPhases_NoValidTargets(t *testing.T) {
	m := NewSessionRunPhaseMachine()
	for _, phase := range []SessionRunPhase{PhaseCompleted, PhaseFailed} {
		targets := m.ValidTargets(phase)
		if targets != nil {
			t.Errorf("ValidTargets(%s) = %v, want nil", phase, targets)
		}
	}
}

// ── ParseSessionRunPhase ─────────────────────────────────────────────────────

func TestParseSessionRunPhase(t *testing.T) {
	tests := []struct {
		input string
		want  SessionRunPhase
	}{
		{"interactive", PhaseInteractive},
		{"escalating", PhaseEscalating},
		{"durable", PhaseDurable},
		{"completed", PhaseCompleted},
		{"failed", PhaseFailed},
		{"unknown", PhaseInteractive},
		{"", PhaseInteractive},
		{"INTERACTIVE", PhaseInteractive},
	}
	for _, tt := range tests {
		got := ParseSessionRunPhase(tt.input)
		if got != tt.want {
			t.Errorf("ParseSessionRunPhase(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ── IsSessionRunPhaseTerminal ─────────────────────────────────────────────────

func TestIsSessionRunPhaseTerminal(t *testing.T) {
	tests := []struct {
		phase SessionRunPhase
		want  bool
	}{
		{PhaseInteractive, false},
		{PhaseEscalating, false},
		{PhaseDurable, false},
		{PhaseCompleted, true},
		{PhaseFailed, true},
	}
	for _, tt := range tests {
		got := IsSessionRunPhaseTerminal(tt.phase)
		if got != tt.want {
			t.Errorf("IsSessionRunPhaseTerminal(%s) = %v, want %v", tt.phase, got, tt.want)
		}
	}
}
