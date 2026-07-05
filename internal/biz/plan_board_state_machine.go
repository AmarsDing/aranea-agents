// Package biz — PlanBoard State Machine (AS-FSM-01)
//
// # PlanBoard State Diagram
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Planning
//	Planning --> Executing : execute
//	Planning --> Failed : fail_early
//	Executing --> Completed : complete
//	Executing --> Failed : fail
//	Executing --> PartialFailure : partial
//	Completed --> [*]
//	Failed --> [*]
//	PartialFailure --> [*]
//
// ```
package biz

import (
	"aranea-agents/internal/biz/shared"
)

// ── PlanBoard Event types ────────────────────────────────────────────────────

// PlanBoardEvent enumerates all events that can trigger a PlanBoard state transition.
// Stability:evolving
type PlanBoardEvent string

const (
	PlanBoardEventExecute    PlanBoardEvent = "execute"
	PlanBoardEventFailEarly  PlanBoardEvent = "fail_early"
	PlanBoardEventComplete   PlanBoardEvent = "complete"
	PlanBoardEventFail       PlanBoardEvent = "fail"
	PlanBoardEventPartial    PlanBoardEvent = "partial"
)

// ── Transition rules ─────────────────────────────────────────────────────────

// planBoardTransitionRules defines the legal state transitions for a PlanBoard.
// Terminal states (completed, failed, partial_failure) have no outgoing transitions.
var planBoardTransitionRules = []shared.TransitionRule[PlanStatus, PlanBoardEvent]{
	{From: PlanStatusPlanning, Event: PlanBoardEventExecute, To: PlanStatusExecuting},
	{From: PlanStatusPlanning, Event: PlanBoardEventFailEarly, To: PlanStatusFailed},
	{From: PlanStatusExecuting, Event: PlanBoardEventComplete, To: PlanStatusCompleted},
	{From: PlanStatusExecuting, Event: PlanBoardEventFail, To: PlanStatusFailed},
	{From: PlanStatusExecuting, Event: PlanBoardEventPartial, To: PlanStatusPartialFailure},
}

// ── PlanBoardStateMachine ────────────────────────────────────────────────────

// PlanBoardStateMachine wraps the generic state machine with PlanBoard-specific types.
// It is safe for concurrent use after construction.
// Stability:evolving
type PlanBoardStateMachine struct {
	inner *shared.GenericStateMachine[PlanStatus, PlanBoardEvent]
}

// NewPlanBoardStateMachine creates a PlanBoardStateMachine with the standard transition rules.
func NewPlanBoardStateMachine() *PlanBoardStateMachine {
	return &PlanBoardStateMachine{
		inner: shared.NewGenericStateMachine[PlanStatus, PlanBoardEvent](planBoardTransitionRules),
	}
}

// Transition validates and executes a state transition.
// Returns the new state on success, or an error for illegal transitions.
func (sm *PlanBoardStateMachine) Transition(from PlanStatus, event PlanBoardEvent) (PlanStatus, error) {
	return sm.inner.Transition(from, event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (sm *PlanBoardStateMachine) CanTransition(from, to PlanStatus) bool {
	return sm.inner.CanTransition(from, to)
}

// ValidTargets returns all states reachable from the given state, sorted
// lexicographically.
func (sm *PlanBoardStateMachine) ValidTargets(from PlanStatus) []PlanStatus {
	return sm.inner.ValidTargets(from)
}

// ── Helper functions ─────────────────────────────────────────────────────────

// IsPlanBoardTerminal returns true for terminal states that have no outgoing transitions.
func IsPlanBoardTerminal(status PlanStatus) bool {
	switch status {
	case PlanStatusCompleted, PlanStatusFailed, PlanStatusPartialFailure:
		return true
	default:
		return false
	}
}

// IsActive reports whether the state is an active (non-terminal) state.
// Planning and Executing are active; Completed/Failed/PartialFailure are not.
func (s PlanStatus) IsActive() bool {
	return s == PlanStatusPlanning || s == PlanStatusExecuting
}
