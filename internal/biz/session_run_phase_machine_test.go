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
		{PhaseInteractive, PhaseEventUserEscalate, PhaseDurable},
		{PhaseInteractive, PhaseEventComplete, PhaseCompleted},
		{PhaseInteractive, PhaseEventFail, PhaseFailed},
		{PhaseInteractive, PhaseEventCancel, PhaseCancelled},
		{PhaseDurable, PhaseEventComplete, PhaseCompleted},
		{PhaseDurable, PhaseEventFail, PhaseFailed},
		{PhaseDurable, PhaseEventCancel, PhaseCancelled},
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
		{PhaseCompleted, PhaseEventUserEscalate},
		{PhaseCompleted, PhaseEventDurable},
		{PhaseCompleted, PhaseEventCancel},
		{PhaseFailed, PhaseEventComplete},
		{PhaseFailed, PhaseEventFail},
		{PhaseFailed, PhaseEventUserEscalate},
		{PhaseFailed, PhaseEventDurable},
		{PhaseFailed, PhaseEventCancel},
		{PhaseCancelled, PhaseEventComplete},
		{PhaseCancelled, PhaseEventFail},
		{PhaseCancelled, PhaseEventUserEscalate},
		{PhaseCancelled, PhaseEventDurable},
		{PhaseCancelled, PhaseEventCancel},
		// Wrong events for the current phase.
		{PhaseInteractive, PhaseEventDurable},
		{PhaseDurable, PhaseEventUserEscalate},
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
		{PhaseInteractive, PhaseDurable, true},
		{PhaseInteractive, PhaseCompleted, true},
		{PhaseInteractive, PhaseFailed, true},
		{PhaseInteractive, PhaseCancelled, true},
		{PhaseDurable, PhaseCompleted, true},
		{PhaseDurable, PhaseFailed, true},
		{PhaseDurable, PhaseCancelled, true},
		// Invalid
		{PhaseInteractive, PhaseInteractive, false},
		{PhaseDurable, PhaseInteractive, false},
		{PhaseDurable, PhaseDurable, false},
		{PhaseCompleted, PhaseInteractive, false},
		{PhaseCompleted, PhaseDurable, false},
		{PhaseCompleted, PhaseCompleted, false},
		{PhaseCompleted, PhaseFailed, false},
		{PhaseCompleted, PhaseCancelled, false},
		{PhaseFailed, PhaseInteractive, false},
		{PhaseFailed, PhaseDurable, false},
		{PhaseFailed, PhaseCompleted, false},
		{PhaseFailed, PhaseFailed, false},
		{PhaseFailed, PhaseCancelled, false},
		{PhaseCancelled, PhaseInteractive, false},
		{PhaseCancelled, PhaseDurable, false},
		{PhaseCancelled, PhaseCompleted, false},
		{PhaseCancelled, PhaseFailed, false},
		{PhaseCancelled, PhaseCancelled, false},
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
		{PhaseInteractive, 4, []SessionRunPhase{PhaseDurable, PhaseCompleted, PhaseFailed, PhaseCancelled}},
		{PhaseDurable, 3, []SessionRunPhase{PhaseCompleted, PhaseFailed, PhaseCancelled}},
		{PhaseCompleted, 0, nil},
		{PhaseFailed, 0, nil},
		{PhaseCancelled, 0, nil},
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
	for _, phase := range []SessionRunPhase{PhaseCompleted, PhaseFailed, PhaseCancelled} {
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
		{"escalating", PhaseDurable}, // legacy: escalating maps to durable
		{"durable", PhaseDurable},
		{"completed", PhaseCompleted},
		{"failed", PhaseFailed},
		{"cancelled", PhaseCancelled},
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
		{PhaseDurable, false},
		{PhaseCompleted, true},
		{PhaseFailed, true},
		{PhaseCancelled, true},
	}
	for _, tt := range tests {
		got := IsSessionRunPhaseTerminal(tt.phase)
		if got != tt.want {
			t.Errorf("IsSessionRunPhaseTerminal(%s) = %v, want %v", tt.phase, got, tt.want)
		}
	}
}
