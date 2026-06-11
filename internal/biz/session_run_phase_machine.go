// SessionRunPhase state machine (AS-FSM-01).
//
// Mermaid state diagram:
//
//	stateDiagram-v2
//	    [*] --> interactive
//	    interactive --> escalating : escalate
//	    interactive --> completed  : complete
//	    interactive --> failed     : fail
//	    escalating --> durable     : durable
//	    escalating --> completed   : complete
//	    escalating --> failed      : fail
//	    durable --> completed      : complete
//	    durable --> failed         : fail
//	    completed --> [*]
//	    failed --> [*]
package biz

import (
	"aranea-agents/internal/biz/shared"
)

// ── Phase type ────────────────────────────────────────────────────────────────

// SessionRunPhase represents the lifecycle phase of a SessionRun.
// Stability:stable
type SessionRunPhase string

const (
	PhaseInteractive SessionRunPhase = "interactive"
	PhaseEscalating  SessionRunPhase = "escalating"
	PhaseDurable     SessionRunPhase = "durable"
	PhaseCompleted   SessionRunPhase = "completed"
	PhaseFailed      SessionRunPhase = "failed"
)

// ── Event type ────────────────────────────────────────────────────────────────

// SessionRunPhaseEvent represents an event that can trigger a phase transition.
// Stability:stable
type SessionRunPhaseEvent string

const (
	PhaseEventEscalate SessionRunPhaseEvent = "escalate"
	PhaseEventDurable  SessionRunPhaseEvent = "durable"
	PhaseEventComplete SessionRunPhaseEvent = "complete"
	PhaseEventFail     SessionRunPhaseEvent = "fail"
)

// ── Transition rules ──────────────────────────────────────────────────────────

var sessionRunPhaseTransitionRules = []shared.TransitionRule[SessionRunPhase, SessionRunPhaseEvent]{
	{From: PhaseInteractive, Event: PhaseEventEscalate, To: PhaseEscalating},
	{From: PhaseEscalating, Event: PhaseEventDurable, To: PhaseDurable},
	{From: PhaseInteractive, Event: PhaseEventComplete, To: PhaseCompleted},
	{From: PhaseInteractive, Event: PhaseEventFail, To: PhaseFailed},
	{From: PhaseEscalating, Event: PhaseEventComplete, To: PhaseCompleted},
	{From: PhaseEscalating, Event: PhaseEventFail, To: PhaseFailed},
	{From: PhaseDurable, Event: PhaseEventComplete, To: PhaseCompleted},
	{From: PhaseDurable, Event: PhaseEventFail, To: PhaseFailed},
}

// ── Machine ───────────────────────────────────────────────────────────────────

// SessionRunPhaseMachine validates and executes phase transitions for SessionRun.
// Stability:stable
type SessionRunPhaseMachine struct {
	inner *shared.GenericStateMachine[SessionRunPhase, SessionRunPhaseEvent]
}

// NewSessionRunPhaseMachine creates a new machine with the standard transition rules.
func NewSessionRunPhaseMachine() *SessionRunPhaseMachine {
	return &SessionRunPhaseMachine{
		inner: shared.NewGenericStateMachine[SessionRunPhase, SessionRunPhaseEvent](sessionRunPhaseTransitionRules),
	}
}

// Transition returns the target phase for the given (from, event) pair.
// Returns an error if the transition is illegal.
func (m *SessionRunPhaseMachine) Transition(from SessionRunPhase, event SessionRunPhaseEvent) (SessionRunPhase, error) {
	return m.inner.Transition(from, event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (m *SessionRunPhaseMachine) CanTransition(from SessionRunPhase, to SessionRunPhase) bool {
	return m.inner.CanTransition(from, to)
}

// ValidTargets returns all phases reachable from the given phase.
// Returns nil for terminal phases (completed, failed).
func (m *SessionRunPhaseMachine) ValidTargets(from SessionRunPhase) []SessionRunPhase {
	return m.inner.ValidTargets(from)
}

// ── Helper functions ──────────────────────────────────────────────────────────

// ParseSessionRunPhase converts a raw string to a typed SessionRunPhase.
// Unrecognised values default to PhaseInteractive.
func ParseSessionRunPhase(s string) SessionRunPhase {
	switch SessionRunPhase(s) {
	case PhaseInteractive, PhaseEscalating, PhaseDurable, PhaseCompleted, PhaseFailed:
		return SessionRunPhase(s)
	default:
		return PhaseInteractive
	}
}

// IsSessionRunPhaseTerminal returns true for terminal phases (completed, failed)
// that have no further transitions.
func IsSessionRunPhaseTerminal(phase SessionRunPhase) bool {
	return phase == PhaseCompleted || phase == PhaseFailed
}
