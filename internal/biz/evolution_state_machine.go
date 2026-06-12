// Package biz — EvolutionSuggestion State Machine (AS-FSM-01)
//
// # EvolutionSuggestion State Diagram
//
// ```mermaid
// stateDiagram-v2
//     [*] --> Pending
//     Pending --> Applied : apply
//     Pending --> Rejected : reject
//     Applied --> RolledBack : rollback
//     Rejected --> [*]
//     RolledBack --> [*]
// ```
package biz

import (
	"aranea-agents/internal/biz/shared"
)

// ── Evolution State & Event types ─────────────────────────────────────────────

// EvolutionState enumerates all legal states of an EvolutionSuggestion entity.
// String values match the raw strings currently used throughout the codebase.
// Stability:stable
type EvolutionState string

const (
	EvolutionStatePending    EvolutionState = "pending"
	EvolutionStateApplied    EvolutionState = "applied"
	EvolutionStateRejected   EvolutionState = "rejected"
	EvolutionStateRolledBack EvolutionState = "rolled_back"
)

// EvolutionEvent enumerates all events that can trigger an EvolutionSuggestion state transition.
// Stability:stable
type EvolutionEvent string

const (
	EvolutionEventApply    EvolutionEvent = "apply"
	EvolutionEventReject   EvolutionEvent = "reject"
	EvolutionEventRollback EvolutionEvent = "rollback"
)

// ── Transition rules ─────────────────────────────────────────────────────────

// evolutionTransitionRules defines the legal state transitions for an EvolutionSuggestion.
// Terminal states (rejected, rolled_back) have no outgoing transitions.
var evolutionTransitionRules = []shared.TransitionRule[EvolutionState, EvolutionEvent]{
	{From: EvolutionStatePending, Event: EvolutionEventApply, To: EvolutionStateApplied},
	{From: EvolutionStatePending, Event: EvolutionEventReject, To: EvolutionStateRejected},
	{From: EvolutionStateApplied, Event: EvolutionEventRollback, To: EvolutionStateRolledBack},
}

// ── EvolutionStateMachine ─────────────────────────────────────────────────────

// EvolutionStateMachine wraps the generic state machine with EvolutionSuggestion-specific types.
// It is safe for concurrent use after construction.
// Stability:stable
type EvolutionStateMachine struct {
	inner *shared.GenericStateMachine[EvolutionState, EvolutionEvent]
}

// NewEvolutionStateMachine creates an EvolutionStateMachine with the standard transition rules.
func NewEvolutionStateMachine() *EvolutionStateMachine {
	return &EvolutionStateMachine{
		inner: shared.NewGenericStateMachine[EvolutionState, EvolutionEvent](evolutionTransitionRules),
	}
}

// Transition validates and executes a state transition.
// Returns the new state on success, or an error for illegal transitions.
func (sm *EvolutionStateMachine) Transition(from EvolutionState, event EvolutionEvent) (EvolutionState, error) {
	return sm.inner.Transition(from, event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (sm *EvolutionStateMachine) CanTransition(from, to EvolutionState) bool {
	return sm.inner.CanTransition(from, to)
}

// ValidTargets returns all states reachable from the given state, sorted
// lexicographically.
func (sm *EvolutionStateMachine) ValidTargets(from EvolutionState) []EvolutionState {
	return sm.inner.ValidTargets(from)
}

// ── Helper functions ─────────────────────────────────────────────────────────

// ParseEvolutionState converts a raw string to an EvolutionState constant.
// Unrecognised strings are returned as-is (they will fail transition validation).
func ParseEvolutionState(s string) EvolutionState {
	switch EvolutionState(s) {
	case EvolutionStatePending, EvolutionStateApplied, EvolutionStateRejected, EvolutionStateRolledBack:
		return EvolutionState(s)
	default:
		return EvolutionState(s)
	}
}

// IsEvolutionTerminal returns true for terminal states that have no outgoing transitions.
func IsEvolutionTerminal(state EvolutionState) bool {
	switch state {
	case EvolutionStateRejected, EvolutionStateRolledBack:
		return true
	default:
		return false
	}
}
