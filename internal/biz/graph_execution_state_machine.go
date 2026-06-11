// Package biz — GraphExecution State Machine (AS-FSM-01)
//
// # GraphExecution State Diagram
//
// ```mermaid
// stateDiagram-v2
//     [*] --> Running
//     Running --> Completed : finish
//     Running --> Failed : fail
//     Running --> Cancelled : cancel
//     Running --> WaitingHuman : await_human
//     WaitingHuman --> Running : resume
//     WaitingHuman --> Failed : fail
//     WaitingHuman --> Cancelled : cancel
//     Completed --> [*]
//     Failed --> [*]
//     Cancelled --> [*]
// ```
package biz

import (
	"aranea-agents/internal/biz/shared"
)

// ── GraphExec State & Event types ────────────────────────────────────────────

// GraphExecState enumerates all legal states of a GraphExecution entity.
// String values match the raw strings currently used throughout the codebase.
// Stability:stable
type GraphExecState string

const (
	GraphExecStateRunning      GraphExecState = "running"
	GraphExecStateCompleted    GraphExecState = "completed"
	GraphExecStateFailed       GraphExecState = "failed"
	GraphExecStateCancelled    GraphExecState = "cancelled"
	GraphExecStateWaitingHuman GraphExecState = "waiting_human"
)

// GraphExecEvent enumerates all events that can trigger a GraphExecution state transition.
// Stability:stable
type GraphExecEvent string

const (
	GraphExecEventFinish     GraphExecEvent = "finish"
	GraphExecEventFail       GraphExecEvent = "fail"
	GraphExecEventCancel     GraphExecEvent = "cancel"
	GraphExecEventAwaitHuman GraphExecEvent = "await_human"
	GraphExecEventResume     GraphExecEvent = "resume"
)

// ── Transition rules ─────────────────────────────────────────────────────────

// graphExecTransitionRules defines the legal state transitions for a GraphExecution.
// Terminal states (completed, failed, cancelled) have no outgoing transitions.
var graphExecTransitionRules = []shared.TransitionRule[GraphExecState, GraphExecEvent]{
	{From: GraphExecStateRunning, Event: GraphExecEventFinish, To: GraphExecStateCompleted},
	{From: GraphExecStateRunning, Event: GraphExecEventFail, To: GraphExecStateFailed},
	{From: GraphExecStateRunning, Event: GraphExecEventCancel, To: GraphExecStateCancelled},
	{From: GraphExecStateRunning, Event: GraphExecEventAwaitHuman, To: GraphExecStateWaitingHuman},
	{From: GraphExecStateWaitingHuman, Event: GraphExecEventResume, To: GraphExecStateRunning},
	{From: GraphExecStateWaitingHuman, Event: GraphExecEventFail, To: GraphExecStateFailed},
	{From: GraphExecStateWaitingHuman, Event: GraphExecEventCancel, To: GraphExecStateCancelled},
}

// ── GraphExecStateMachine ────────────────────────────────────────────────────

// GraphExecStateMachine wraps the generic state machine with GraphExecution-specific types.
// It is safe for concurrent use after construction.
// Stability:stable
type GraphExecStateMachine struct {
	inner *shared.GenericStateMachine[GraphExecState, GraphExecEvent]
}

// NewGraphExecStateMachine creates a GraphExecStateMachine with the standard transition rules.
func NewGraphExecStateMachine() *GraphExecStateMachine {
	return &GraphExecStateMachine{
		inner: shared.NewGenericStateMachine[GraphExecState, GraphExecEvent](graphExecTransitionRules),
	}
}

// Transition validates and executes a state transition.
// Returns the new state on success, or an error for illegal transitions.
func (sm *GraphExecStateMachine) Transition(from GraphExecState, event GraphExecEvent) (GraphExecState, error) {
	return sm.inner.Transition(from, event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (sm *GraphExecStateMachine) CanTransition(from, to GraphExecState) bool {
	return sm.inner.CanTransition(from, to)
}

// ValidTargets returns all states reachable from the given state, sorted
// lexicographically.
func (sm *GraphExecStateMachine) ValidTargets(from GraphExecState) []GraphExecState {
	return sm.inner.ValidTargets(from)
}

// ── Helper functions ─────────────────────────────────────────────────────────

// defaultGraphExecSM is the singleton GraphExecStateMachine used by convenience functions.
var defaultGraphExecSM = NewGraphExecStateMachine()

// GraphExecStatus constants are string aliases for backward compatibility.
// New code should use GraphExecState typed constants instead.
const (
	GraphExecStatusRunning      = string(GraphExecStateRunning)
	GraphExecStatusCompleted    = string(GraphExecStateCompleted)
	GraphExecStatusFailed       = string(GraphExecStateFailed)
	GraphExecStatusCancelled    = string(GraphExecStateCancelled)
	GraphExecStatusWaitingHuman = string(GraphExecStateWaitingHuman)
)

// IsGraphExecTerminalStatus reports whether the given status is terminal.
func IsGraphExecTerminalStatus(status string) bool {
	switch GraphExecState(status) {
	case GraphExecStateCompleted, GraphExecStateFailed, GraphExecStateCancelled:
		return true
	default:
		return false
	}
}

// ValidateGraphExecTransition reports whether transitioning from one status to another is valid.
func ValidateGraphExecTransition(from, to string) bool {
	if from == to {
		return true
	}
	return defaultGraphExecSM.CanTransition(GraphExecState(from), GraphExecState(to))
}
