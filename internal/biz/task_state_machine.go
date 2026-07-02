// Package biz — Task State Machine (AS-FSM-01)
//
// # Task State Diagram
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Pending
//	Pending --> PendingAssignment : assign_dynamic_no_agent
//	Pending --> Claimed : claim
//	PendingAssignment --> Pending : reassign
//	PendingAssignment --> Claimed : claim
//	Claimed --> Complete : complete
//	Claimed --> ReviewRequired : complete_need_review
//	Claimed --> Blocked : block
//	Claimed --> TimedOut : timeout
//	Blocked --> Pending : unblock
//	ReviewRequired --> Complete : approve
//	ReviewRequired --> Claimed : reject
//	TimedOut --> Pending : retry
//	Complete --> [*]
//	Failed --> [*]
//	Cancelled --> [*]
//	Crashed --> [*]
//
// ```
package biz

import (
	"aranea-agents/internal/biz/shared"
)

// ── Task Event types ─────────────────────────────────────────────────────────

// TaskTransitionEvent enumerates all events that can trigger a Task state transition.
// Named TaskTransitionEvent to avoid collision with the TaskEvent struct in task.go.
// Stability:evolving
type TaskTransitionEvent string

const (
	TaskEventClaim              TaskTransitionEvent = "claim"
	TaskEventComplete           TaskTransitionEvent = "complete"
	TaskEventCompleteNeedReview TaskTransitionEvent = "complete_need_review"
	TaskEventBlock              TaskTransitionEvent = "block"
	TaskEventUnblock            TaskTransitionEvent = "unblock"
	TaskEventApprove            TaskTransitionEvent = "approve"
	TaskEventReject             TaskTransitionEvent = "reject"
	TaskEventTimeout            TaskTransitionEvent = "timeout"
	TaskEventRetry              TaskTransitionEvent = "retry"
	TaskEventAssignDynamic      TaskTransitionEvent = "assign_dynamic_no_agent"
	TaskEventReassign           TaskTransitionEvent = "reassign"
)

// ── Transition rules ─────────────────────────────────────────────────────────

// taskTransitionRules defines the legal state transitions for a Task.
// Terminal states (complete, failed, cancelled, crashed) have no outgoing transitions.
var taskTransitionRules = []shared.TransitionRule[GraphTaskStatus, TaskTransitionEvent]{
	{From: GraphTaskStatusPending, Event: TaskEventClaim, To: GraphTaskStatusClaimed},
	{From: GraphTaskStatusPending, Event: TaskEventAssignDynamic, To: GraphTaskStatusPendingAssignment},
	{From: GraphTaskStatusPendingAssignment, Event: TaskEventReassign, To: GraphTaskStatusPending},
	{From: GraphTaskStatusPendingAssignment, Event: TaskEventClaim, To: GraphTaskStatusClaimed},
	{From: GraphTaskStatusClaimed, Event: TaskEventComplete, To: GraphTaskStatusComplete},
	{From: GraphTaskStatusClaimed, Event: TaskEventCompleteNeedReview, To: GraphTaskStatusReviewRequired},
	{From: GraphTaskStatusClaimed, Event: TaskEventBlock, To: GraphTaskStatusBlocked},
	{From: GraphTaskStatusClaimed, Event: TaskEventTimeout, To: GraphTaskStatusTimedOut},
	{From: GraphTaskStatusBlocked, Event: TaskEventUnblock, To: GraphTaskStatusPending},
	{From: GraphTaskStatusReviewRequired, Event: TaskEventApprove, To: GraphTaskStatusComplete},
	{From: GraphTaskStatusReviewRequired, Event: TaskEventReject, To: GraphTaskStatusClaimed},
	{From: GraphTaskStatusTimedOut, Event: TaskEventRetry, To: GraphTaskStatusPending},
}

// ── TaskStateMachine ─────────────────────────────────────────────────────────

// TaskStateMachine wraps the generic state machine with Task-specific types.
// It is safe for concurrent use after construction.
// Stability:evolving
type TaskStateMachine struct {
	inner *shared.GenericStateMachine[GraphTaskStatus, TaskTransitionEvent]
}

// NewTaskStateMachine creates a TaskStateMachine with the standard transition rules.
func NewTaskStateMachine() *TaskStateMachine {
	return &TaskStateMachine{
		inner: shared.NewGenericStateMachine[GraphTaskStatus, TaskTransitionEvent](taskTransitionRules),
	}
}

// Transition validates and executes a state transition.
// Returns the new state on success, or an error for illegal transitions.
func (sm *TaskStateMachine) Transition(from GraphTaskStatus, event TaskTransitionEvent) (GraphTaskStatus, error) {
	return sm.inner.Transition(from, event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (sm *TaskStateMachine) CanTransition(from, to GraphTaskStatus) bool {
	return sm.inner.CanTransition(from, to)
}

// ValidTargets returns all states reachable from the given state, sorted
// lexicographically.
func (sm *TaskStateMachine) ValidTargets(from GraphTaskStatus) []GraphTaskStatus {
	return sm.inner.ValidTargets(from)
}

// ── Helper functions ─────────────────────────────────────────────────────────

// IsTaskTerminal returns true for terminal states that have no outgoing transitions.
func IsTaskTerminal(state GraphTaskStatus) bool {
	switch state {
	case GraphTaskStatusComplete, GraphTaskStatusFailed, GraphTaskStatusCancelled, GraphTaskStatusCrashed:
		return true
	default:
		return false
	}
}

// taskStateMachine is the package-level singleton instance.
// Safe for concurrent use — all internal state is immutable after construction.
var taskStateMachine = NewTaskStateMachine()

// TaskTransition is a convenience function that validates and executes a Task
// state transition using the package-level state machine.
// Returns the new state on success, or an error for illegal transitions.
func TaskTransition(from GraphTaskStatus, event TaskTransitionEvent) (GraphTaskStatus, error) {
	return taskStateMachine.Transition(from, event)
}

// CanTaskTransition reports whether a direct transition from→to is legal.
func CanTaskTransition(from, to GraphTaskStatus) bool {
	return taskStateMachine.CanTransition(from, to)
}
