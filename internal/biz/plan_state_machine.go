// Package biz — Plan State Machine (AS-FSM-01)
//
// # Plan State Diagram
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Draft
//	Draft --> Approved : approve
//	Approved --> Confirmed : confirm
//	Approved --> Executing : start
//	Confirmed --> Executing : start
//	Executing --> Completed : complete
//	Executing --> Failed : fail
//	Failed --> Draft : retry
//	Completed --> [*]
//
// ```
package biz

import (
	"aranea-agents/internal/biz/shared"
)

// ── Plan Event types ─────────────────────────────────────────────────────────

// PlanEvent enumerates all events that can trigger a Plan state transition.
// Stability:evolving
type PlanEvent string

const (
	PlanEventApprove  PlanEvent = "approve"
	PlanEventConfirm  PlanEvent = "confirm"
	PlanEventStart    PlanEvent = "start"
	PlanEventComplete PlanEvent = "complete"
	PlanEventFail     PlanEvent = "fail"
	PlanEventRetry    PlanEvent = "retry"
)

// ── Transition rules ─────────────────────────────────────────────────────────

// planTransitionRules defines the legal state transitions for a Plan.
// Terminal state (completed) has no outgoing transitions.
var planTransitionRules = []shared.TransitionRule[PlanStatus, PlanEvent]{
	{From: PlanStatusDraft, Event: PlanEventApprove, To: PlanStatusApproved},
	{From: PlanStatusApproved, Event: PlanEventConfirm, To: PlanStatusConfirmed},
	{From: PlanStatusApproved, Event: PlanEventStart, To: PlanStatusExecuting},
	{From: PlanStatusConfirmed, Event: PlanEventStart, To: PlanStatusExecuting},
	{From: PlanStatusExecuting, Event: PlanEventComplete, To: PlanStatusCompleted},
	{From: PlanStatusExecuting, Event: PlanEventFail, To: PlanStatusFailed},
	{From: PlanStatusFailed, Event: PlanEventRetry, To: PlanStatusDraft},
}

// ── PlanStateMachine ─────────────────────────────────────────────────────────

// PlanStateMachine wraps the generic state machine with Plan-specific types.
// It is safe for concurrent use after construction.
// Stability:evolving
type PlanStateMachine struct {
	inner *shared.GenericStateMachine[PlanStatus, PlanEvent]
}

// NewPlanStateMachine creates a PlanStateMachine with the standard transition rules.
func NewPlanStateMachine() *PlanStateMachine {
	return &PlanStateMachine{
		inner: shared.NewGenericStateMachine[PlanStatus, PlanEvent](planTransitionRules),
	}
}

// Transition validates and executes a state transition.
// Returns the new state on success, or an error for illegal transitions.
func (sm *PlanStateMachine) Transition(from PlanStatus, event PlanEvent) (PlanStatus, error) {
	return sm.inner.Transition(from, event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (sm *PlanStateMachine) CanTransition(from, to PlanStatus) bool {
	return sm.inner.CanTransition(from, to)
}

// ValidTargets returns all states reachable from the given state, sorted
// lexicographically.
func (sm *PlanStateMachine) ValidTargets(from PlanStatus) []PlanStatus {
	return sm.inner.ValidTargets(from)
}

// ── Helper functions ─────────────────────────────────────────────────────────

// IsPlanTerminal returns true for terminal states that have no outgoing transitions.
func IsPlanTerminal(state PlanStatus) bool {
	return state == PlanStatusCompleted
}

// planStateMachine is the package-level singleton instance.
// Safe for concurrent use — all internal state is immutable after construction.
var planStateMachine = NewPlanStateMachine()

// PlanTransition is a convenience function that validates and executes a Plan
// state transition using the package-level state machine.
// Returns the new state on success, or an error for illegal transitions.
func PlanTransition(from PlanStatus, event PlanEvent) (PlanStatus, error) {
	return planStateMachine.Transition(from, event)
}

// CanPlanTransition reports whether a direct transition from→to is legal.
func CanPlanTransition(from, to PlanStatus) bool {
	return planStateMachine.CanTransition(from, to)
}
