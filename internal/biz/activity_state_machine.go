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
//	Running --> Paused : pause
//	Running --> Completed : done
//	Running --> Failed : fail
//	Running --> Cancelled : cancel
//	Running --> Interrupted : interrupt
//	Running --> PartialFailure : partial
//	ToolRunning --> Running : tool_end
//	ToolRunning --> Completed : done
//	ToolRunning --> Failed : fail
//	ToolRunning --> Cancelled : cancel
//	ToolBlocked --> Running : tool_unblock
//	ToolBlocked --> Completed : done
//	ToolBlocked --> Failed : fail
//	ToolBlocked --> Cancelled : cancel
//	Paused --> Running : unpause
//	Paused --> Completed : done
//	Paused --> Failed : fail
//	Paused --> Cancelled : cancel
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

// ── Activity Transition Event types ──────────────────────────────────────────

// ActivityTransitionEvent enumerates all events that can trigger an Activity
// state transition. These are internal state-machine triggers, distinct from
// the transport-level ActivityEvent (biz.ActivityEvent) published to EventBus.
// Stability:evolving
type ActivityTransitionEvent string

const (
	ActivityTransitionStart       ActivityTransitionEvent = "start"
	ActivityTransitionToolStart   ActivityTransitionEvent = "tool_start"
	ActivityTransitionToolEnd     ActivityTransitionEvent = "tool_end"
	ActivityTransitionToolBlock   ActivityTransitionEvent = "tool_block"
	ActivityTransitionToolUnblock ActivityTransitionEvent = "tool_unblock"
	ActivityTransitionDone        ActivityTransitionEvent = "done"
	ActivityTransitionFail        ActivityTransitionEvent = "fail"
	ActivityTransitionCancel      ActivityTransitionEvent = "cancel"
	ActivityTransitionInterrupt   ActivityTransitionEvent = "interrupt"
	ActivityTransitionPartial     ActivityTransitionEvent = "partial"
	ActivityTransitionPause       ActivityTransitionEvent = "pause"
	ActivityTransitionUnpause     ActivityTransitionEvent = "unpause"
)

// ── Transition rules ─────────────────────────────────────────────────────────

// activityTransitionRules defines the legal state transitions for an Activity.
// Terminal states (Completed/Failed/Cancelled/Interrupted/PartialFailure) have
// no outgoing transitions. Paused is non-terminal (resumable).
var activityTransitionRules = []shared.TransitionRule[ActivityStatus, ActivityTransitionEvent]{
	// Pending → *
	{From: ActivityStatusPending, Event: ActivityTransitionStart, To: ActivityStatusRunning},
	// Running → *
	{From: ActivityStatusRunning, Event: ActivityTransitionToolStart, To: ActivityStatusToolRunning},
	{From: ActivityStatusRunning, Event: ActivityTransitionToolBlock, To: ActivityStatusToolBlocked},
	{From: ActivityStatusRunning, Event: ActivityTransitionPause, To: ActivityStatusPaused},
	{From: ActivityStatusRunning, Event: ActivityTransitionDone, To: ActivityStatusCompleted},
	{From: ActivityStatusRunning, Event: ActivityTransitionFail, To: ActivityStatusFailed},
	{From: ActivityStatusRunning, Event: ActivityTransitionCancel, To: ActivityStatusCancelled},
	{From: ActivityStatusRunning, Event: ActivityTransitionInterrupt, To: ActivityStatusInterrupted},
	{From: ActivityStatusRunning, Event: ActivityTransitionPartial, To: ActivityStatusPartialFailure},
	// ToolRunning → *
	{From: ActivityStatusToolRunning, Event: ActivityTransitionToolEnd, To: ActivityStatusRunning},
	{From: ActivityStatusToolRunning, Event: ActivityTransitionDone, To: ActivityStatusCompleted},
	{From: ActivityStatusToolRunning, Event: ActivityTransitionFail, To: ActivityStatusFailed},
	{From: ActivityStatusToolRunning, Event: ActivityTransitionCancel, To: ActivityStatusCancelled},
	// ToolBlocked → *
	{From: ActivityStatusToolBlocked, Event: ActivityTransitionToolUnblock, To: ActivityStatusRunning},
	{From: ActivityStatusToolBlocked, Event: ActivityTransitionDone, To: ActivityStatusCompleted},
	{From: ActivityStatusToolBlocked, Event: ActivityTransitionFail, To: ActivityStatusFailed},
	{From: ActivityStatusToolBlocked, Event: ActivityTransitionCancel, To: ActivityStatusCancelled},
	// Paused → * (resumable; non-terminal)
	{From: ActivityStatusPaused, Event: ActivityTransitionUnpause, To: ActivityStatusRunning},
	{From: ActivityStatusPaused, Event: ActivityTransitionDone, To: ActivityStatusCompleted},
	{From: ActivityStatusPaused, Event: ActivityTransitionFail, To: ActivityStatusFailed},
	{From: ActivityStatusPaused, Event: ActivityTransitionCancel, To: ActivityStatusCancelled},
}

// ── ActivityStateMachine ─────────────────────────────────────────────────────

// activityStateMachine is the singleton ActivityStatus state machine.
var activityStateMachine = shared.NewGenericStateMachine(activityTransitionRules)

// ActivityStateMachine returns the singleton ActivityStatus state machine.
// Stability:evolving
func ActivityStateMachine() shared.StateMachine[ActivityStatus, ActivityTransitionEvent] {
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
func TransitionActivityStatus(from ActivityStatus, event ActivityTransitionEvent) (ActivityStatus, error) {
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
