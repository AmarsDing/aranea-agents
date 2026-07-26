// Package biz — UnifiedEvolutionSuggestion State Machine (AS-FSM-01)
//
// This file defines the state machine for UnifiedEvolutionSuggestion.Status
// (pending/approved/rejected/applied/rolled_back/expired).
//
// # UnifiedEvolution State Diagram
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Pending
//	Pending --> Approved : approve
//	Pending --> Rejected : reject
//	Pending --> Expired : expire
//	Approved --> Applied : apply
//	Applied --> RolledBack : rollback
//	Rejected --> [*]
//	RolledBack --> [*]
//	Expired --> [*]
//
// ```
package biz

import (
	"aranea-agents/internal/biz/shared"
)

// ── UnifiedEvolution State & Event types ─────────────────────────────────────

// UnifiedEvolutionState enumerates all legal states of a UnifiedEvolutionSuggestion.
// String values match the raw strings used in UnifiedEvolutionSuggestion.Status.
// Stability:stable
type UnifiedEvolutionState string

const (
	UnifiedEvolutionStatePending    UnifiedEvolutionState = "pending"
	UnifiedEvolutionStateApproved   UnifiedEvolutionState = "approved"
	UnifiedEvolutionStateRejected   UnifiedEvolutionState = "rejected"
	UnifiedEvolutionStateApplied    UnifiedEvolutionState = "applied"
	UnifiedEvolutionStateExpired    UnifiedEvolutionState = "expired"
	UnifiedEvolutionStateRolledBack UnifiedEvolutionState = "rolled_back"
)

// UnifiedEvolutionEvent enumerates events that trigger a UnifiedEvolutionSuggestion state transition.
// Stability:stable
type UnifiedEvolutionEvent string

const (
	UnifiedEvolutionEventApprove  UnifiedEvolutionEvent = "approve"
	UnifiedEvolutionEventReject   UnifiedEvolutionEvent = "reject"
	UnifiedEvolutionEventApply    UnifiedEvolutionEvent = "apply"
	UnifiedEvolutionEventExpire   UnifiedEvolutionEvent = "expire"
	UnifiedEvolutionEventRollback UnifiedEvolutionEvent = "rollback"
)

// ── UnifiedEvolution transition rules ────────────────────────────────────────

// unifiedEvolutionTransitionRules defines legal state transitions for a UnifiedEvolutionSuggestion.
// Terminal states (rejected, rolled_back, expired) have no outgoing transitions.
// Note: the L1 skill-proposal legacy status 'registered' is not a unified state;
// it is stored verbatim and interpreted by the L1 view layer only.
var unifiedEvolutionTransitionRules = []shared.TransitionRule[UnifiedEvolutionState, UnifiedEvolutionEvent]{
	{From: UnifiedEvolutionStatePending, Event: UnifiedEvolutionEventApprove, To: UnifiedEvolutionStateApproved},
	{From: UnifiedEvolutionStatePending, Event: UnifiedEvolutionEventReject, To: UnifiedEvolutionStateRejected},
	{From: UnifiedEvolutionStatePending, Event: UnifiedEvolutionEventExpire, To: UnifiedEvolutionStateExpired},
	{From: UnifiedEvolutionStateApproved, Event: UnifiedEvolutionEventApply, To: UnifiedEvolutionStateApplied},
	{From: UnifiedEvolutionStateApplied, Event: UnifiedEvolutionEventRollback, To: UnifiedEvolutionStateRolledBack},
}

// ── UnifiedEvolutionStateMachine ─────────────────────────────────────────────

// UnifiedEvolutionStateMachine wraps the generic state machine with
// UnifiedEvolutionSuggestion-specific types. Safe for concurrent use after construction.
// Stability:stable
type UnifiedEvolutionStateMachine struct {
	inner *shared.GenericStateMachine[UnifiedEvolutionState, UnifiedEvolutionEvent]
}

// NewUnifiedEvolutionStateMachine creates a UnifiedEvolutionStateMachine with standard rules.
func NewUnifiedEvolutionStateMachine() *UnifiedEvolutionStateMachine {
	return &UnifiedEvolutionStateMachine{
		inner: shared.NewGenericStateMachine[UnifiedEvolutionState, UnifiedEvolutionEvent](unifiedEvolutionTransitionRules),
	}
}

// Transition validates and executes a state transition.
func (sm *UnifiedEvolutionStateMachine) Transition(from UnifiedEvolutionState, event UnifiedEvolutionEvent) (UnifiedEvolutionState, error) {
	return sm.inner.Transition(from, event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (sm *UnifiedEvolutionStateMachine) CanTransition(from, to UnifiedEvolutionState) bool {
	return sm.inner.CanTransition(from, to)
}

// ValidTargets returns all states reachable from the given state.
func (sm *UnifiedEvolutionStateMachine) ValidTargets(from UnifiedEvolutionState) []UnifiedEvolutionState {
	return sm.inner.ValidTargets(from)
}

// ── Helper functions ─────────────────────────────────────────────────────────

// ParseUnifiedEvolutionState converts a raw string to a UnifiedEvolutionState constant.
func ParseUnifiedEvolutionState(s string) UnifiedEvolutionState {
	return UnifiedEvolutionState(s)
}

// IsUnifiedEvolutionTerminal returns true for terminal states with no outgoing transitions.
func IsUnifiedEvolutionTerminal(state UnifiedEvolutionState) bool {
	switch state {
	case UnifiedEvolutionStateRejected, UnifiedEvolutionStateRolledBack, UnifiedEvolutionStateExpired:
		return true
	default:
		return false
	}
}
