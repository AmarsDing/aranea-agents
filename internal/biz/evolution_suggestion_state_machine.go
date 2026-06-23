// Package biz — SkillEvolutionSuggestion State Machine (AS-FSM-01)
//
// This file defines TWO related state machines:
//  1. EvolutionSuggestionStateMachine — for SkillEvolutionSuggestion.Status
//     (pending/approved/rejected/applied)
//  2. UnifiedEvolutionStateMachine — for UnifiedEvolutionSuggestion.Status
//     (pending/approved/rejected/applied/expired)
//
// # EvolutionSuggestion State Diagram
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Pending
//	Pending --> Approved : approve
//	Pending --> Rejected : reject
//	Approved --> Applied : apply
//	Rejected --> [*]
//	Applied --> [*]
//
// ```
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
//	Rejected --> [*]
//	Applied --> [*]
//	Expired --> [*]
//
// ```
package biz

import (
	"aranea-agents/internal/biz/shared"
)

// ── EvolutionSuggestion Event type ───────────────────────────────────────────

// EvolutionSuggestionEvent enumerates events that trigger an EvolutionSuggestion state transition.
// Stability:stable
type EvolutionSuggestionEvent string

const (
	EvoSuggestionEventApprove EvolutionSuggestionEvent = "approve"
	EvoSuggestionEventReject  EvolutionSuggestionEvent = "reject"
	EvoSuggestionEventApply   EvolutionSuggestionEvent = "apply"
)

// ── EvolutionSuggestion transition rules ─────────────────────────────────────

// evoSuggestionTransitionRules defines legal state transitions for a SkillEvolutionSuggestion.
// Terminal states (rejected, applied) have no outgoing transitions.
var evoSuggestionTransitionRules = []shared.TransitionRule[EvolutionSuggestionStatus, EvolutionSuggestionEvent]{
	{From: EvoSuggestionPending, Event: EvoSuggestionEventApprove, To: EvoSuggestionApproved},
	{From: EvoSuggestionPending, Event: EvoSuggestionEventReject, To: EvoSuggestionRejected},
	{From: EvoSuggestionApproved, Event: EvoSuggestionEventApply, To: EvoSuggestionApplied},
}

// ── EvolutionSuggestionStateMachine ──────────────────────────────────────────

// EvolutionSuggestionStateMachine wraps the generic state machine with
// SkillEvolutionSuggestion-specific types. Safe for concurrent use after construction.
// Stability:stable
type EvolutionSuggestionStateMachine struct {
	inner *shared.GenericStateMachine[EvolutionSuggestionStatus, EvolutionSuggestionEvent]
}

// NewEvolutionSuggestionStateMachine creates an EvolutionSuggestionStateMachine with standard rules.
func NewEvolutionSuggestionStateMachine() *EvolutionSuggestionStateMachine {
	return &EvolutionSuggestionStateMachine{
		inner: shared.NewGenericStateMachine[EvolutionSuggestionStatus, EvolutionSuggestionEvent](evoSuggestionTransitionRules),
	}
}

// Transition validates and executes a state transition.
func (sm *EvolutionSuggestionStateMachine) Transition(from EvolutionSuggestionStatus, event EvolutionSuggestionEvent) (EvolutionSuggestionStatus, error) {
	return sm.inner.Transition(from, event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (sm *EvolutionSuggestionStateMachine) CanTransition(from, to EvolutionSuggestionStatus) bool {
	return sm.inner.CanTransition(from, to)
}

// ValidTargets returns all states reachable from the given state.
func (sm *EvolutionSuggestionStateMachine) ValidTargets(from EvolutionSuggestionStatus) []EvolutionSuggestionStatus {
	return sm.inner.ValidTargets(from)
}

// ── UnifiedEvolution State & Event types ─────────────────────────────────────

// UnifiedEvolutionState enumerates all legal states of a UnifiedEvolutionSuggestion.
// String values match the raw strings used in UnifiedEvolutionSuggestion.Status.
// Stability:stable
type UnifiedEvolutionState string

const (
	UnifiedEvolutionStatePending  UnifiedEvolutionState = "pending"
	UnifiedEvolutionStateApproved UnifiedEvolutionState = "approved"
	UnifiedEvolutionStateRejected UnifiedEvolutionState = "rejected"
	UnifiedEvolutionStateApplied  UnifiedEvolutionState = "applied"
	UnifiedEvolutionStateExpired  UnifiedEvolutionState = "expired"
)

// UnifiedEvolutionEvent enumerates events that trigger a UnifiedEvolutionSuggestion state transition.
// Stability:stable
type UnifiedEvolutionEvent string

const (
	UnifiedEvolutionEventApprove UnifiedEvolutionEvent = "approve"
	UnifiedEvolutionEventReject  UnifiedEvolutionEvent = "reject"
	UnifiedEvolutionEventApply   UnifiedEvolutionEvent = "apply"
	UnifiedEvolutionEventExpire  UnifiedEvolutionEvent = "expire"
)

// ── UnifiedEvolution transition rules ────────────────────────────────────────

// unifiedEvolutionTransitionRules defines legal state transitions for a UnifiedEvolutionSuggestion.
// Terminal states (rejected, applied, expired) have no outgoing transitions.
var unifiedEvolutionTransitionRules = []shared.TransitionRule[UnifiedEvolutionState, UnifiedEvolutionEvent]{
	{From: UnifiedEvolutionStatePending, Event: UnifiedEvolutionEventApprove, To: UnifiedEvolutionStateApproved},
	{From: UnifiedEvolutionStatePending, Event: UnifiedEvolutionEventReject, To: UnifiedEvolutionStateRejected},
	{From: UnifiedEvolutionStatePending, Event: UnifiedEvolutionEventExpire, To: UnifiedEvolutionStateExpired},
	{From: UnifiedEvolutionStateApproved, Event: UnifiedEvolutionEventApply, To: UnifiedEvolutionStateApplied},
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
	case UnifiedEvolutionStateRejected, UnifiedEvolutionStateApplied, UnifiedEvolutionStateExpired:
		return true
	default:
		return false
	}
}
