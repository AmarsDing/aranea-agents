// Package biz — GraphExecution State Machine (AS-FSM-01)
//
// # GraphExecution State Diagram
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Running
//	Running --> Completed : complete
//	Running --> Failed : fail
//	Running --> Cancelled : cancel
//	Running --> WaitingHuman : interrupt
//	Running --> Running : recover (startup reconcile only)
//	WaitingHuman --> Running : resume
//	WaitingHuman --> Cancelled : cancel
//	WaitingHuman --> Failed : fail
//	Completed --> [*]
//	Failed --> [*]
//	Cancelled --> [*]
//
// ```
package biz

import (
	"aranea-agents/internal/biz/shared"
)

// ── GraphExecution State & Event types ────────────────────────────────────────

// GraphExecutionState enumerates all legal states of a GraphExecution entity.
// String values match the raw strings currently used throughout the codebase.
// Stability:stable
type GraphExecutionState string

const (
	GraphExecRunning      GraphExecutionState = "running"
	GraphExecCompleted    GraphExecutionState = "completed"
	GraphExecFailed       GraphExecutionState = "failed"
	GraphExecCancelled    GraphExecutionState = "cancelled"
	GraphExecWaitingHuman GraphExecutionState = "waiting_human"
)

// GraphExecutionEvent enumerates all events that can trigger a GraphExecution state transition.
// Stability:stable
type GraphExecutionEvent string

const (
	GraphExecEventComplete  GraphExecutionEvent = "complete"
	GraphExecEventFail      GraphExecutionEvent = "fail"
	GraphExecEventCancel    GraphExecutionEvent = "cancel"
	GraphExecEventInterrupt GraphExecutionEvent = "interrupt"
	GraphExecEventResume    GraphExecutionEvent = "resume"
	// GraphExecEventRecover 仅启动对账路径使用（83-长时运行韧性）：孤儿 running
	// 执行被崩溃恢复接管、重建 runtime 的自环事件，DB 状态不变。
	GraphExecEventRecover GraphExecutionEvent = "recover"
)

// ── Transition rules ─────────────────────────────────────────────────────────

// graphExecutionTransitionRules defines the legal state transitions for a GraphExecution.
// Terminal states (completed, failed, cancelled) have no outgoing transitions.
var graphExecutionTransitionRules = []shared.TransitionRule[GraphExecutionState, GraphExecutionEvent]{
	{From: GraphExecRunning, Event: GraphExecEventComplete, To: GraphExecCompleted},
	{From: GraphExecRunning, Event: GraphExecEventFail, To: GraphExecFailed},
	{From: GraphExecRunning, Event: GraphExecEventCancel, To: GraphExecCancelled},
	{From: GraphExecRunning, Event: GraphExecEventInterrupt, To: GraphExecWaitingHuman},
	{From: GraphExecRunning, Event: GraphExecEventRecover, To: GraphExecRunning},
	{From: GraphExecWaitingHuman, Event: GraphExecEventResume, To: GraphExecRunning},
	{From: GraphExecWaitingHuman, Event: GraphExecEventCancel, To: GraphExecCancelled},
	{From: GraphExecWaitingHuman, Event: GraphExecEventFail, To: GraphExecFailed},
}

// ── GraphExecutionStateMachine ────────────────────────────────────────────────

// GraphExecutionStateMachine wraps the generic state machine with GraphExecution-specific types.
// It is safe for concurrent use after construction.
// Stability:stable
type GraphExecutionStateMachine struct {
	inner *shared.GenericStateMachine[GraphExecutionState, GraphExecutionEvent]
}

// NewGraphExecutionStateMachine creates a GraphExecutionStateMachine with the standard transition rules.
func NewGraphExecutionStateMachine() *GraphExecutionStateMachine {
	return &GraphExecutionStateMachine{
		inner: shared.NewGenericStateMachine[GraphExecutionState, GraphExecutionEvent](graphExecutionTransitionRules),
	}
}

// Transition validates and executes a state transition.
// Returns the new state on success, or an error for illegal transitions.
func (sm *GraphExecutionStateMachine) Transition(from GraphExecutionState, event GraphExecutionEvent) (GraphExecutionState, error) {
	return sm.inner.Transition(from, event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (sm *GraphExecutionStateMachine) CanTransition(from, to GraphExecutionState) bool {
	return sm.inner.CanTransition(from, to)
}

// ValidTargets returns all states reachable from the given state, sorted
// lexicographically.
func (sm *GraphExecutionStateMachine) ValidTargets(from GraphExecutionState) []GraphExecutionState {
	return sm.inner.ValidTargets(from)
}

// ── Helper functions ─────────────────────────────────────────────────────────

// ParseGraphExecutionState converts a raw string to a GraphExecutionState constant.
// Unrecognised strings are returned as-is (they will fail transition validation).
func ParseGraphExecutionState(s string) GraphExecutionState {
	switch GraphExecutionState(s) {
	case GraphExecRunning, GraphExecCompleted, GraphExecFailed,
		GraphExecCancelled, GraphExecWaitingHuman:
		return GraphExecutionState(s)
	default:
		return GraphExecutionState(s)
	}
}

// IsGraphExecutionTerminal returns true for terminal states that have no outgoing transitions.
func IsGraphExecutionTerminal(state GraphExecutionState) bool {
	switch state {
	case GraphExecCompleted, GraphExecFailed, GraphExecCancelled:
		return true
	default:
		return false
	}
}

// IsActive reports whether the state is an active (non-terminal, evictable-later) state.
// Running and WaitingHuman are active; Completed/Failed/Cancelled are not.
func (s GraphExecutionState) IsActive() bool {
	return s == GraphExecRunning || s == GraphExecWaitingHuman
}
