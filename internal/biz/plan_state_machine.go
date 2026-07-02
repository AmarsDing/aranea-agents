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
var planTransitionRules = []shared.TransitionRule[LegacyPlanStatus, PlanEvent]{
	{From: LegacyPlanStatusDraft, Event: PlanEventApprove, To: LegacyPlanStatusApproved},
	{From: LegacyPlanStatusApproved, Event: PlanEventConfirm, To: LegacyPlanStatusConfirmed},
	{From: LegacyPlanStatusApproved, Event: PlanEventStart, To: LegacyPlanStatusExecuting},
	{From: LegacyPlanStatusConfirmed, Event: PlanEventStart, To: LegacyPlanStatusExecuting},
	{From: LegacyPlanStatusExecuting, Event: PlanEventComplete, To: LegacyPlanStatusCompleted},
	{From: LegacyPlanStatusExecuting, Event: PlanEventFail, To: LegacyPlanStatusFailed},
	{From: LegacyPlanStatusFailed, Event: PlanEventRetry, To: LegacyPlanStatusDraft},
}

// ── PlanStateMachine ─────────────────────────────────────────────────────────

// PlanStateMachine wraps the generic state machine with Plan-specific types.
// It is safe for concurrent use after construction.
// Stability:evolving
type PlanStateMachine struct {
	inner *shared.GenericStateMachine[LegacyPlanStatus, PlanEvent]
}

// NewPlanStateMachine creates a PlanStateMachine with the standard transition rules.
func NewPlanStateMachine() *PlanStateMachine {
	return &PlanStateMachine{
		inner: shared.NewGenericStateMachine[LegacyPlanStatus, PlanEvent](planTransitionRules),
	}
}

// Transition validates and executes a state transition.
// Returns the new state on success, or an error for illegal transitions.
func (sm *PlanStateMachine) Transition(from LegacyPlanStatus, event PlanEvent) (LegacyPlanStatus, error) {
	return sm.inner.Transition(from, event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (sm *PlanStateMachine) CanTransition(from, to LegacyPlanStatus) bool {
	return sm.inner.CanTransition(from, to)
}

// ValidTargets returns all states reachable from the given state, sorted
// lexicographically.
func (sm *PlanStateMachine) ValidTargets(from LegacyPlanStatus) []LegacyPlanStatus {
	return sm.inner.ValidTargets(from)
}

// ── Helper functions ─────────────────────────────────────────────────────────

// IsPlanTerminal returns true for terminal states that have no outgoing transitions.
func IsPlanTerminal(state LegacyPlanStatus) bool {
	return state == LegacyPlanStatusCompleted
}

// planStateMachine is the package-level singleton instance.
// Safe for concurrent use — all internal state is immutable after construction.
var planStateMachine = NewPlanStateMachine()

// PlanTransition is a convenience function that validates and executes a Plan
// state transition using the package-level state machine.
// Returns the new state on success, or an error for illegal transitions.
func PlanTransition(from LegacyPlanStatus, event PlanEvent) (LegacyPlanStatus, error) {
	return planStateMachine.Transition(from, event)
}

// CanPlanTransition reports whether a direct transition from→to is legal.
func CanPlanTransition(from, to LegacyPlanStatus) bool {
	return planStateMachine.CanTransition(from, to)
}
