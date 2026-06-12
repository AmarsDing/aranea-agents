// Package biz — ChannelTurnJob State Machine (AS-FSM-01)
//
// # ChannelTurnJob State Diagram
//
// ```mermaid
// stateDiagram-v2
//     [*] --> Accepted
//     Accepted --> Running : start
//     Accepted --> Queued : queue
//     Accepted --> Cancelled : cancel
//     Accepted --> AsyncQueued : async_queue
//     Running --> Completed : complete
//     Running --> Failed : fail
//     Running --> Timeout : timeout
//     Running --> Cancelled : cancel
//     Running --> AsyncQueued : async_queue
//     Queued --> Running : dequeue
//     Queued --> Cancelled : cancel
//     AsyncQueued --> Running : async_start
//     AsyncQueued --> Failed : async_fail
//     AsyncQueued --> Cancelled : async_cancel
//     AsyncQueued --> Timeout : timeout
//     Completed --> [*]
//     Failed --> [*]
//     Timeout --> [*]
//     Cancelled --> [*]
// ```
package biz

import (
	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
)

// ── ChannelTurnJob State & Event types ────────────────────────────────────────

// ChannelTurnJobState enumerates all legal states of a ChannelTurnJob entity.
// String values match the raw strings currently used throughout the codebase.
// Stability:stable
type ChannelTurnJobState string

const (
	ChannelTurnJobStateAccepted    ChannelTurnJobState = ChannelTurnJobStatusAccepted
	ChannelTurnJobStateRunning     ChannelTurnJobState = ChannelTurnJobStatusRunning
	ChannelTurnJobStateQueued      ChannelTurnJobState = ChannelTurnJobStatusQueued
	ChannelTurnJobStateCompleted   ChannelTurnJobState = ChannelTurnJobStatusCompleted
	ChannelTurnJobStateFailed      ChannelTurnJobState = ChannelTurnJobStatusFailed
	ChannelTurnJobStateTimeout     ChannelTurnJobState = ChannelTurnJobStatusTimeout
	ChannelTurnJobStateCancelled   ChannelTurnJobState = ChannelTurnJobStatusCancelled
	ChannelTurnJobStateAsyncQueued ChannelTurnJobState = ChannelTurnJobStatusAsyncQueued
)

// ChannelTurnJobEvent enumerates all events that can trigger a ChannelTurnJob state transition.
// Stability:stable
type ChannelTurnJobEvent string

const (
	ChannelTurnJobEventStart       ChannelTurnJobEvent = JobEventStart
	ChannelTurnJobEventQueue       ChannelTurnJobEvent = JobEventQueue
	ChannelTurnJobEventDequeue     ChannelTurnJobEvent = JobEventDequeue
	ChannelTurnJobEventComplete    ChannelTurnJobEvent = JobEventComplete
	ChannelTurnJobEventFail        ChannelTurnJobEvent = JobEventFail
	ChannelTurnJobEventTimeout     ChannelTurnJobEvent = JobEventTimeout
	ChannelTurnJobEventCancel      ChannelTurnJobEvent = JobEventCancel
	ChannelTurnJobEventAsyncQueue  ChannelTurnJobEvent = JobEventAsyncQueue
	ChannelTurnJobEventAsyncStart  ChannelTurnJobEvent = JobEventAsyncStart
	ChannelTurnJobEventAsyncFail   ChannelTurnJobEvent = JobEventAsyncFail
	ChannelTurnJobEventAsyncCancel ChannelTurnJobEvent = JobEventAsyncCancel
)

// ── Transition rules ─────────────────────────────────────────────────────────

// channelTurnJobTransitionRules defines the legal state transitions for a ChannelTurnJob.
// Terminal states (completed, failed, timeout, cancelled) have no outgoing transitions.
var channelTurnJobTransitionRules = []shared.TransitionRule[ChannelTurnJobState, ChannelTurnJobEvent]{
	// accepted →
	{From: ChannelTurnJobStateAccepted, Event: ChannelTurnJobEventStart, To: ChannelTurnJobStateRunning},
	{From: ChannelTurnJobStateAccepted, Event: ChannelTurnJobEventQueue, To: ChannelTurnJobStateQueued},
	{From: ChannelTurnJobStateAccepted, Event: ChannelTurnJobEventCancel, To: ChannelTurnJobStateCancelled},
	{From: ChannelTurnJobStateAccepted, Event: ChannelTurnJobEventAsyncQueue, To: ChannelTurnJobStateAsyncQueued},
	// running →
	{From: ChannelTurnJobStateRunning, Event: ChannelTurnJobEventComplete, To: ChannelTurnJobStateCompleted},
	{From: ChannelTurnJobStateRunning, Event: ChannelTurnJobEventFail, To: ChannelTurnJobStateFailed},
	{From: ChannelTurnJobStateRunning, Event: ChannelTurnJobEventTimeout, To: ChannelTurnJobStateTimeout},
	{From: ChannelTurnJobStateRunning, Event: ChannelTurnJobEventCancel, To: ChannelTurnJobStateCancelled},
	{From: ChannelTurnJobStateRunning, Event: ChannelTurnJobEventAsyncQueue, To: ChannelTurnJobStateAsyncQueued},
	// queued →
	{From: ChannelTurnJobStateQueued, Event: ChannelTurnJobEventDequeue, To: ChannelTurnJobStateRunning},
	{From: ChannelTurnJobStateQueued, Event: ChannelTurnJobEventCancel, To: ChannelTurnJobStateCancelled},
	// async_queued →
	{From: ChannelTurnJobStateAsyncQueued, Event: ChannelTurnJobEventAsyncStart, To: ChannelTurnJobStateRunning},
	{From: ChannelTurnJobStateAsyncQueued, Event: ChannelTurnJobEventAsyncFail, To: ChannelTurnJobStateFailed},
	{From: ChannelTurnJobStateAsyncQueued, Event: ChannelTurnJobEventAsyncCancel, To: ChannelTurnJobStateCancelled},
	{From: ChannelTurnJobStateAsyncQueued, Event: ChannelTurnJobEventTimeout, To: ChannelTurnJobStateTimeout},
}

// ── ChannelTurnJobStateMachine ────────────────────────────────────────────────

// ChannelTurnJobStateMachine wraps the generic state machine with ChannelTurnJob-specific types.
// It is safe for concurrent use after construction.
// Stability:stable
type ChannelTurnJobStateMachine struct {
	inner *shared.GenericStateMachine[ChannelTurnJobState, ChannelTurnJobEvent]
}

// NewChannelTurnJobStateMachine creates a ChannelTurnJobStateMachine with the standard transition rules.
func NewChannelTurnJobStateMachine() *ChannelTurnJobStateMachine {
	return &ChannelTurnJobStateMachine{
		inner: shared.NewGenericStateMachine[ChannelTurnJobState, ChannelTurnJobEvent](channelTurnJobTransitionRules),
	}
}

// Transition validates and executes a state transition.
// Returns the new state on success, or an error for illegal transitions.
func (sm *ChannelTurnJobStateMachine) Transition(from ChannelTurnJobState, event ChannelTurnJobEvent) (ChannelTurnJobState, error) {
	return sm.inner.Transition(from, event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (sm *ChannelTurnJobStateMachine) CanTransition(from, to ChannelTurnJobState) bool {
	return sm.inner.CanTransition(from, to)
}

// ValidTargets returns all states reachable from the given state, sorted lexicographically.
func (sm *ChannelTurnJobStateMachine) ValidTargets(from ChannelTurnJobState) []ChannelTurnJobState {
	return sm.inner.ValidTargets(from)
}

// ── Backward-compatible convenience functions ─────────────────────────────────

// defaultChannelTurnJobSM is the singleton used by backward-compatible convenience functions.
var defaultChannelTurnJobSM = NewChannelTurnJobStateMachine()

// TransitionChannelTurnJob validates and returns the target status for a transition.
// Returns an error if the transition is illegal.
// Stability:stable
func TransitionChannelTurnJob(fromStatus, event string) (string, error) {
	from := ChannelTurnJobState(NormalizeChannelTurnJobStatus(fromStatus))
	to, err := defaultChannelTurnJobSM.Transition(from, ChannelTurnJobEvent(event))
	if err != nil {
		return string(from), apierror.BadRequest("CHANNEL_TURN_JOB", "illegal transition from "+string(from)+" via "+event)
	}
	return string(to), nil
}

// CanTransitionChannelTurnJob reports whether a transition is legal.
// Stability:stable
func CanTransitionChannelTurnJob(fromStatus, event string) bool {
	from := ChannelTurnJobState(NormalizeChannelTurnJobStatus(fromStatus))
	evt := ChannelTurnJobEvent(event)
	to, err := defaultChannelTurnJobSM.Transition(from, evt)
	if err != nil {
		return false
	}
	return defaultChannelTurnJobSM.CanTransition(from, to)
}

// ChannelTurnJobStatusFromEvent returns the target status for a given event.
// Returns an error for unknown events to prevent silent misrouting.
// Stability:stable
func ChannelTurnJobStatusFromEvent(event string) (string, error) {
	switch event {
	case JobEventStart:
		return ChannelTurnJobStatusRunning, nil
	case JobEventQueue:
		return ChannelTurnJobStatusQueued, nil
	case JobEventDequeue:
		return ChannelTurnJobStatusRunning, nil
	case JobEventComplete:
		return ChannelTurnJobStatusCompleted, nil
	case JobEventFail:
		return ChannelTurnJobStatusFailed, nil
	case JobEventTimeout:
		return ChannelTurnJobStatusTimeout, nil
	case JobEventCancel:
		return ChannelTurnJobStatusCancelled, nil
	case JobEventAsyncQueue:
		return ChannelTurnJobStatusAsyncQueued, nil
	case JobEventAsyncStart:
		return ChannelTurnJobStatusRunning, nil
	case JobEventAsyncFail:
		return ChannelTurnJobStatusFailed, nil
	case JobEventAsyncCancel:
		return ChannelTurnJobStatusCancelled, nil
	default:
		return "", apierror.BadRequest("CHANNEL_TURN_JOB", "unknown event: "+event)
	}
}

// ChannelTurnJobEventFromStatus derives the canonical state machine event that leads
// to the given target status. Used by markTurnJob to route through the state machine
// even when callers specify a target status rather than an event.
// Returns an error for ambiguous or unknown statuses.
func ChannelTurnJobEventFromStatus(status string) (string, error) {
	switch NormalizeChannelTurnJobStatus(status) {
	case ChannelTurnJobStatusRunning:
		return JobEventStart, nil
	case ChannelTurnJobStatusQueued:
		return JobEventQueue, nil
	case ChannelTurnJobStatusCompleted:
		return JobEventComplete, nil
	case ChannelTurnJobStatusFailed:
		return JobEventFail, nil
	case ChannelTurnJobStatusTimeout:
		return JobEventTimeout, nil
	case ChannelTurnJobStatusCancelled:
		return JobEventCancel, nil
	case ChannelTurnJobStatusAsyncQueued:
		return JobEventAsyncQueue, nil
	default:
		return "", apierror.BadRequest("CHANNEL_TURN_JOB", "cannot derive event from status: "+status)
	}
}
