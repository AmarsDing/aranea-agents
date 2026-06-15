// SessionRunPhase state machine (AS-FSM-01).
//
// Mermaid state diagram:
//
//	stateDiagram-v2
//	    [*] --> interactive
//	    interactive --> durable     : user_escalate
//	    interactive --> completed   : complete
//	    interactive --> failed      : fail
//	    interactive --> cancelled   : cancel
//	    durable --> completed       : complete
//	    durable --> failed          : fail
//	    durable --> cancelled       : cancel
//	    completed --> [*]
//	    failed --> [*]
//	    cancelled --> [*]
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
	PhaseDurable     SessionRunPhase = "durable"
	PhaseCompleted   SessionRunPhase = "completed"
	PhaseFailed      SessionRunPhase = "failed"
	PhaseCancelled   SessionRunPhase = "cancelled"
)

// ── Event type ────────────────────────────────────────────────────────────────

// SessionRunPhaseEvent represents an event that can trigger a phase transition.
// Stability:stable
type SessionRunPhaseEvent string

const (
	PhaseEventUserEscalate SessionRunPhaseEvent = "user_escalate"
	PhaseEventDurable      SessionRunPhaseEvent = "durable"
	PhaseEventComplete     SessionRunPhaseEvent = "complete"
	PhaseEventFail         SessionRunPhaseEvent = "fail"
	PhaseEventCancel       SessionRunPhaseEvent = "cancel"
)

// ── Transition rules ──────────────────────────────────────────────────────────

var sessionRunPhaseTransitionRules = []shared.TransitionRule[SessionRunPhase, SessionRunPhaseEvent]{
	{From: PhaseInteractive, Event: PhaseEventUserEscalate, To: PhaseDurable},
	{From: PhaseInteractive, Event: PhaseEventComplete, To: PhaseCompleted},
	{From: PhaseInteractive, Event: PhaseEventFail, To: PhaseFailed},
	{From: PhaseInteractive, Event: PhaseEventCancel, To: PhaseCancelled},
	{From: PhaseDurable, Event: PhaseEventComplete, To: PhaseCompleted},
	{From: PhaseDurable, Event: PhaseEventFail, To: PhaseFailed},
	{From: PhaseDurable, Event: PhaseEventCancel, To: PhaseCancelled},
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
// Returns nil for terminal phases (completed, failed, cancelled).
func (m *SessionRunPhaseMachine) ValidTargets(from SessionRunPhase) []SessionRunPhase {
	return m.inner.ValidTargets(from)
}

// ── Helper functions ──────────────────────────────────────────────────────────

// ParseSessionRunPhase converts a raw string to a typed SessionRunPhase.
// Unrecognised values default to PhaseInteractive.
func ParseSessionRunPhase(s string) SessionRunPhase {
	switch SessionRunPhase(s) {
	case PhaseInteractive, PhaseDurable, PhaseCompleted, PhaseFailed, PhaseCancelled:
		return SessionRunPhase(s)
	default:
		// 兼容：DB 中历史 escalating 记录映射为 PhaseDurable
		if SessionRunPhase(s) == "escalating" {
			return PhaseDurable
		}
		return PhaseInteractive
	}
}

// IsSessionRunPhaseTerminal returns true for terminal phases (completed, failed, cancelled)
// that have no further transitions.
func IsSessionRunPhaseTerminal(phase SessionRunPhase) bool {
	return phase == PhaseCompleted || phase == PhaseFailed || phase == PhaseCancelled
}
