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
var taskTransitionRules = []shared.TransitionRule[TaskStatus, TaskTransitionEvent]{
	{From: TaskStatusPending, Event: TaskEventClaim, To: TaskStatusClaimed},
	{From: TaskStatusPending, Event: TaskEventAssignDynamic, To: TaskStatusPendingAssignment},
	{From: TaskStatusPendingAssignment, Event: TaskEventReassign, To: TaskStatusPending},
	{From: TaskStatusPendingAssignment, Event: TaskEventClaim, To: TaskStatusClaimed},
	{From: TaskStatusClaimed, Event: TaskEventComplete, To: TaskStatusComplete},
	{From: TaskStatusClaimed, Event: TaskEventCompleteNeedReview, To: TaskStatusReviewRequired},
	{From: TaskStatusClaimed, Event: TaskEventBlock, To: TaskStatusBlocked},
	{From: TaskStatusClaimed, Event: TaskEventTimeout, To: TaskStatusTimedOut},
	{From: TaskStatusBlocked, Event: TaskEventUnblock, To: TaskStatusPending},
	{From: TaskStatusReviewRequired, Event: TaskEventApprove, To: TaskStatusComplete},
	{From: TaskStatusReviewRequired, Event: TaskEventReject, To: TaskStatusClaimed},
	{From: TaskStatusTimedOut, Event: TaskEventRetry, To: TaskStatusPending},
}

// ── TaskStateMachine ─────────────────────────────────────────────────────────

// TaskStateMachine wraps the generic state machine with Task-specific types.
// It is safe for concurrent use after construction.
// Stability:evolving
type TaskStateMachine struct {
	inner *shared.GenericStateMachine[TaskStatus, TaskTransitionEvent]
}

// NewTaskStateMachine creates a TaskStateMachine with the standard transition rules.
func NewTaskStateMachine() *TaskStateMachine {
	return &TaskStateMachine{
		inner: shared.NewGenericStateMachine[TaskStatus, TaskTransitionEvent](taskTransitionRules),
	}
}

// Transition validates and executes a state transition.
// Returns the new state on success, or an error for illegal transitions.
func (sm *TaskStateMachine) Transition(from TaskStatus, event TaskTransitionEvent) (TaskStatus, error) {
	return sm.inner.Transition(from, event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (sm *TaskStateMachine) CanTransition(from, to TaskStatus) bool {
	return sm.inner.CanTransition(from, to)
}

// ValidTargets returns all states reachable from the given state, sorted
// lexicographically.
func (sm *TaskStateMachine) ValidTargets(from TaskStatus) []TaskStatus {
	return sm.inner.ValidTargets(from)
}

// ── Helper functions ─────────────────────────────────────────────────────────

// IsTaskTerminal returns true for terminal states that have no outgoing transitions.
func IsTaskTerminal(state TaskStatus) bool {
	switch state {
	case TaskStatusComplete, TaskStatusFailed, TaskStatusCancelled, TaskStatusCrashed:
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
func TaskTransition(from TaskStatus, event TaskTransitionEvent) (TaskStatus, error) {
	return taskStateMachine.Transition(from, event)
}

// CanTaskTransition reports whether a direct transition from→to is legal.
func CanTaskTransition(from, to TaskStatus) bool {
	return taskStateMachine.CanTransition(from, to)
}
