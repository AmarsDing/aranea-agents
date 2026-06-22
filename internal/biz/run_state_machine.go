// Package biz — Run State Machine (AS-FSM-01)
//
// # Run State Diagram
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> None
//	None --> Running : start
//	Running --> Completed : complete
//	Running --> Failed : fail
//	Running --> Cancelled : cancel
//	Running --> AwaitingUser : await
//	AwaitingUser --> Running : resume
//	AwaitingUser --> Cancelled : cancel
//	AwaitingUser --> Failed : fail
//	Completed --> [*]
//	Failed --> [*]
//	Cancelled --> [*]
//
// ```
package biz

import (
	"aranea-agents/internal/biz/shared"
)

// ── Run State & Event types ──────────────────────────────────────────────────

// RunState enumerates all legal states of a Run entity.
// String values match the raw strings currently used throughout the codebase.
// Stability:stable
type RunState string

const (
	RunStateNone         RunState = ""
	RunStateRunning      RunState = "running"
	RunStateCompleted    RunState = "completed"
	RunStateFailed       RunState = "failed"
	RunStateCancelled    RunState = "cancelled"
	RunStateAwaitingUser RunState = "awaiting_user"
)

// RunEvent enumerates all events that can trigger a Run state transition.
// Stability:stable
type RunEvent string

const (
	RunEventStart    RunEvent = "start"
	RunEventComplete RunEvent = "complete"
	RunEventFail     RunEvent = "fail"
	RunEventCancel   RunEvent = "cancel"
	RunEventAwait    RunEvent = "await"
	RunEventResume   RunEvent = "resume"
)

// ── Transition rules ─────────────────────────────────────────────────────────

// runTransitionRules defines the legal state transitions for a Run.
// Terminal states (completed, failed, cancelled) have no outgoing transitions.
var runTransitionRules = []shared.TransitionRule[RunState, RunEvent]{
	{From: RunStateNone, Event: RunEventStart, To: RunStateRunning},
	{From: RunStateRunning, Event: RunEventComplete, To: RunStateCompleted},
	{From: RunStateRunning, Event: RunEventFail, To: RunStateFailed},
	{From: RunStateRunning, Event: RunEventCancel, To: RunStateCancelled},
	{From: RunStateRunning, Event: RunEventAwait, To: RunStateAwaitingUser},
	{From: RunStateAwaitingUser, Event: RunEventResume, To: RunStateRunning},
	{From: RunStateAwaitingUser, Event: RunEventCancel, To: RunStateCancelled},
	{From: RunStateAwaitingUser, Event: RunEventFail, To: RunStateFailed},
}

// ── RunStateMachine ──────────────────────────────────────────────────────────

// RunStateMachine wraps the generic state machine with Run-specific types.
// It is safe for concurrent use after construction.
// Stability:stable
type RunStateMachine struct {
	inner *shared.GenericStateMachine[RunState, RunEvent]
}

// NewRunStateMachine creates a RunStateMachine with the standard transition rules.
func NewRunStateMachine() *RunStateMachine {
	return &RunStateMachine{
		inner: shared.NewGenericStateMachine[RunState, RunEvent](runTransitionRules),
	}
}

// Transition validates and executes a state transition.
// Returns the new state on success, or an error for illegal transitions.
func (sm *RunStateMachine) Transition(from RunState, event RunEvent) (RunState, error) {
	return sm.inner.Transition(from, event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (sm *RunStateMachine) CanTransition(from, to RunState) bool {
	return sm.inner.CanTransition(from, to)
}

// ValidTargets returns all states reachable from the given state, sorted
// lexicographically.
func (sm *RunStateMachine) ValidTargets(from RunState) []RunState {
	return sm.inner.ValidTargets(from)
}

// ── Helper functions ─────────────────────────────────────────────────────────

// ParseRunState converts a raw string to a RunState constant.
// Unrecognised strings are returned as-is (they will fail transition validation).
func ParseRunState(s string) RunState {
	switch RunState(s) {
	case RunStateNone, RunStateRunning, RunStateCompleted,
		RunStateFailed, RunStateCancelled, RunStateAwaitingUser:
		return RunState(s)
	default:
		return RunState(s)
	}
}

// IsRunTerminal returns true for terminal states that have no outgoing transitions.
func IsRunTerminal(state RunState) bool {
	switch state {
	case RunStateCompleted, RunStateFailed, RunStateCancelled:
		return true
	default:
		return false
	}
}
