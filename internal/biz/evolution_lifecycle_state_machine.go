// Package biz — EvolutionLifecycle State Machine (AS-FSM-01)
//
// # EvolutionLifecycle State Diagram
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Draft
//	Draft --> Validating : start_validation
//	Validating --> Ready : validation_passed
//	Validating --> Draft : validation_failed
//	Ready --> Applied : apply
//	Applied --> [*]
//
// ```
package biz

import (
	"aranea-agents/internal/biz/shared"
)

// ── EvolutionLifecycle Event type ────────────────────────────────────────────

// EvolutionLifecycleEvent enumerates events that trigger an EvolutionLifecycle state transition.
// Stability:stable
type EvolutionLifecycleEvent string

const (
	EvoLifecycleEventStartValidation  EvolutionLifecycleEvent = "start_validation"
	EvoLifecycleEventValidationPassed EvolutionLifecycleEvent = "validation_passed"
	EvoLifecycleEventValidationFailed EvolutionLifecycleEvent = "validation_failed"
	EvoLifecycleEventApply            EvolutionLifecycleEvent = "apply"
)

// ── Transition rules ─────────────────────────────────────────────────────────

// evolutionLifecycleTransitionRules defines legal state transitions for EvolutionLifecycleStatus.
// Terminal state (applied) has no outgoing transitions.
var evolutionLifecycleTransitionRules = []shared.TransitionRule[EvolutionLifecycleStatus, EvolutionLifecycleEvent]{
	{From: EvoLifecycleDraft, Event: EvoLifecycleEventStartValidation, To: EvoLifecycleValidating},
	{From: EvoLifecycleValidating, Event: EvoLifecycleEventValidationPassed, To: EvoLifecycleReady},
	{From: EvoLifecycleValidating, Event: EvoLifecycleEventValidationFailed, To: EvoLifecycleDraft},
	{From: EvoLifecycleReady, Event: EvoLifecycleEventApply, To: EvoLifecycleApplied},
}

// ── EvolutionLifecycleStateMachine ───────────────────────────────────────────

// EvolutionLifecycleStateMachine wraps the generic state machine with
// EvolutionLifecycle-specific types. Safe for concurrent use after construction.
// Stability:stable
type EvolutionLifecycleStateMachine struct {
	inner *shared.GenericStateMachine[EvolutionLifecycleStatus, EvolutionLifecycleEvent]
}

// NewEvolutionLifecycleStateMachine creates an EvolutionLifecycleStateMachine with standard rules.
func NewEvolutionLifecycleStateMachine() *EvolutionLifecycleStateMachine {
	return &EvolutionLifecycleStateMachine{
		inner: shared.NewGenericStateMachine[EvolutionLifecycleStatus, EvolutionLifecycleEvent](evolutionLifecycleTransitionRules),
	}
}

// Transition validates and executes a state transition.
func (sm *EvolutionLifecycleStateMachine) Transition(from EvolutionLifecycleStatus, event EvolutionLifecycleEvent) (EvolutionLifecycleStatus, error) {
	return sm.inner.Transition(from, event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (sm *EvolutionLifecycleStateMachine) CanTransition(from, to EvolutionLifecycleStatus) bool {
	return sm.inner.CanTransition(from, to)
}

// ValidTargets returns all states reachable from the given state.
func (sm *EvolutionLifecycleStateMachine) ValidTargets(from EvolutionLifecycleStatus) []EvolutionLifecycleStatus {
	return sm.inner.ValidTargets(from)
}

// ── Helper functions ─────────────────────────────────────────────────────────

// ParseEvolutionLifecycleStatus converts a raw string to an EvolutionLifecycleStatus constant.
func ParseEvolutionLifecycleStatus(s string) EvolutionLifecycleStatus {
	return EvolutionLifecycleStatus(s)
}

// IsEvolutionLifecycleTerminal returns true for terminal states with no outgoing transitions.
func IsEvolutionLifecycleTerminal(state EvolutionLifecycleStatus) bool {
	return state == EvoLifecycleApplied
}
