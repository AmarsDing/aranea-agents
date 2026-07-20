// Package biz — GraphStage State Machine (AS-FSM-01)
//
// # GraphStage State Diagram
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Running
//	Running --> Completed : complete
//	Running --> Failed : fail
//	Running --> Interrupted : interrupt
//	Completed --> [*]
//	Failed --> [*]
//	Interrupted --> [*]
//
// ```
package biz

import (
	"aranea-agents/internal/biz/shared"
)

// ── GraphStage Event types ───────────────────────────────────────────────────

// GraphStageEvent enumerates all events that can trigger a GraphStage state transition.
// Stability:evolving
type GraphStageEvent string

const (
	GraphStageEventComplete  GraphStageEvent = "complete"
	GraphStageEventFail      GraphStageEvent = "fail"
	GraphStageEventInterrupt GraphStageEvent = "interrupt"
)

// ── Transition rules ─────────────────────────────────────────────────────────

// graphStageTransitionRules defines the legal state transitions for a GraphStage.
// Terminal states (completed, failed, interrupted) have no outgoing transitions.
var graphStageTransitionRules = []shared.TransitionRule[GraphStageStatus, GraphStageEvent]{
	{From: GraphStageStatusRunning, Event: GraphStageEventComplete, To: GraphStageStatusCompleted},
	{From: GraphStageStatusRunning, Event: GraphStageEventFail, To: GraphStageStatusFailed},
	{From: GraphStageStatusRunning, Event: GraphStageEventInterrupt, To: GraphStageStatusInterrupted},
}

// ── GraphStageStateMachine ───────────────────────────────────────────────────

// GraphStageStateMachine wraps the generic state machine with GraphStage-specific types.
// It is safe for concurrent use after construction.
// Stability:evolving
type GraphStageStateMachine struct {
	inner *shared.GenericStateMachine[GraphStageStatus, GraphStageEvent]
}

// NewGraphStageStateMachine creates a GraphStageStateMachine with the standard transition rules.
func NewGraphStageStateMachine() *GraphStageStateMachine {
	return &GraphStageStateMachine{
		inner: shared.NewGenericStateMachine[GraphStageStatus, GraphStageEvent](graphStageTransitionRules),
	}
}

// Transition validates and executes a state transition.
// Returns the new state on success, or an error for illegal transitions.
func (sm *GraphStageStateMachine) Transition(from GraphStageStatus, event GraphStageEvent) (GraphStageStatus, error) {
	return sm.inner.Transition(from, event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (sm *GraphStageStateMachine) CanTransition(from, to GraphStageStatus) bool {
	return sm.inner.CanTransition(from, to)
}

// ValidTargets returns all states reachable from the given state, sorted
// lexicographically.
func (sm *GraphStageStateMachine) ValidTargets(from GraphStageStatus) []GraphStageStatus {
	return sm.inner.ValidTargets(from)
}

// ── Helper functions ─────────────────────────────────────────────────────────

// IsGraphStageTerminal returns true for terminal states that have no outgoing transitions.
func IsGraphStageTerminal(status GraphStageStatus) bool {
	switch status {
	case GraphStageStatusCompleted, GraphStageStatusFailed, GraphStageStatusInterrupted:
		return true
	default:
		return false
	}
}

// IsActive reports whether the state is an active (non-terminal) state.
// Only Running is active; Completed/Failed/Interrupted are not.
func (s GraphStageStatus) IsActive() bool {
	return s == GraphStageStatusRunning
}
