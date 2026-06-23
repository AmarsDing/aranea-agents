// Package biz — Activity State Machine (AS-FSM-01)
//
// # Activity State Diagram
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Pending
//	Pending --> Running : start
//	Running --> ToolRunning : tool_start
//	Running --> ToolBlocked : tool_block
//	Running --> Completed : done
//	Running --> Failed : fail
//	Running --> Cancelled : cancel
//	Running --> Interrupted : interrupt
//	Running --> PartialFailure : partial
//	ToolRunning --> Running : tool_end
//	ToolRunning --> Completed : done
//	ToolRunning --> Failed : fail
//	ToolBlocked --> Running : tool_unblock
//	ToolBlocked --> Failed : fail
//	Completed --> [*]
//	Failed --> [*]
//	Cancelled --> [*]
//	Interrupted --> [*]
//	PartialFailure --> [*]
//
// ```
package biz

import (
	"aranea-agents/internal/biz/shared"
)

// ── Activity Event types ─────────────────────────────────────────────────────

// ActivityEvent enumerates all events that can trigger an Activity state
// transition.
// Stability:evolving
type ActivityEvent string

const (
	ActivityEventStart       ActivityEvent = "start"
	ActivityEventToolStart   ActivityEvent = "tool_start"
	ActivityEventToolEnd     ActivityEvent = "tool_end"
	ActivityEventToolBlock   ActivityEvent = "tool_block"
	ActivityEventToolUnblock ActivityEvent = "tool_unblock"
	ActivityEventDone        ActivityEvent = "done"
	ActivityEventFail        ActivityEvent = "fail"
	ActivityEventCancel      ActivityEvent = "cancel"
	ActivityEventInterrupt   ActivityEvent = "interrupt"
	ActivityEventPartial     ActivityEvent = "partial"
)

// ── Transition rules ─────────────────────────────────────────────────────────

// activityTransitionRules defines the legal state transitions for an Activity.
// Terminal states (Completed/Failed/Cancelled/Interrupted/PartialFailure) have
// no outgoing transitions.
var activityTransitionRules = []shared.TransitionRule[ActivityStatus, ActivityEvent]{
	// Pending → *
	{From: ActivityStatusPending, Event: ActivityEventStart, To: ActivityStatusRunning},
	// Running → *
	{From: ActivityStatusRunning, Event: ActivityEventToolStart, To: ActivityStatusToolRunning},
	{From: ActivityStatusRunning, Event: ActivityEventToolBlock, To: ActivityStatusToolBlocked},
	{From: ActivityStatusRunning, Event: ActivityEventDone, To: ActivityStatusCompleted},
	{From: ActivityStatusRunning, Event: ActivityEventFail, To: ActivityStatusFailed},
	{From: ActivityStatusRunning, Event: ActivityEventCancel, To: ActivityStatusCancelled},
	{From: ActivityStatusRunning, Event: ActivityEventInterrupt, To: ActivityStatusInterrupted},
	{From: ActivityStatusRunning, Event: ActivityEventPartial, To: ActivityStatusPartialFailure},
	// ToolRunning → *
	{From: ActivityStatusToolRunning, Event: ActivityEventToolEnd, To: ActivityStatusRunning},
	{From: ActivityStatusToolRunning, Event: ActivityEventDone, To: ActivityStatusCompleted},
	{From: ActivityStatusToolRunning, Event: ActivityEventFail, To: ActivityStatusFailed},
	// ToolBlocked → *
	{From: ActivityStatusToolBlocked, Event: ActivityEventToolUnblock, To: ActivityStatusRunning},
	{From: ActivityStatusToolBlocked, Event: ActivityEventFail, To: ActivityStatusFailed},
}

// ── ActivityStateMachine ─────────────────────────────────────────────────────

// activityStateMachine is the singleton ActivityStatus state machine.
var activityStateMachine = shared.NewGenericStateMachine(activityTransitionRules)

// ActivityStateMachine returns the singleton ActivityStatus state machine.
// Stability:evolving
func ActivityStateMachine() shared.StateMachine[ActivityStatus, ActivityEvent] {
	return activityStateMachine
}

// CanTransitionActivityStatus reports whether a direct transition from one
// ActivityStatus to another is valid according to the state machine.
func CanTransitionActivityStatus(from, to ActivityStatus) bool {
	return activityStateMachine.CanTransition(from, to)
}

// TransitionActivityStatus validates and executes a state transition triggered
// by the given event. Returns the new state on success, or an error for illegal
// transitions.
func TransitionActivityStatus(from ActivityStatus, event ActivityEvent) (ActivityStatus, error) {
	return activityStateMachine.Transition(from, event)
}

// IsActivityTerminal returns true for terminal states that have no outgoing
// transitions.
func IsActivityTerminal(state ActivityStatus) bool {
	switch state {
	case ActivityStatusCompleted, ActivityStatusFailed, ActivityStatusCancelled, ActivityStatusInterrupted, ActivityStatusPartialFailure:
		return true
	default:
		return false
	}
}
