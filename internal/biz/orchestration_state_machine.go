// Package biz — Orchestration State Machine (AS-FSM-01)
//
// # Orchestration State Diagram
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Pending
//	Pending --> Running : start
//	Pending --> Completed : direct_strategy
//	Pending --> Failed : setup_failed
//	Pending --> Cancelled : cancel
//	Running --> Completed : synthesize
//	Running --> Failed : execute_failed
//	Running --> Cancelled : cancel
//	Running --> Interrupted : interrupt
//	Interrupted --> Running : recover
//	Interrupted --> Failed : recover_failed
//	Interrupted --> Cancelled : cancel
//	Completed --> [*]
//	Failed --> [*]
//	Cancelled --> [*]
//
// ```
package biz

import (
	"aranea-agents/internal/biz/shared"
)

// ── Orchestration Event types ────────────────────────────────────────────────

// OrchestrationEvent enumerates all events that can trigger an Orchestration
// state transition.
// Stability:evolving
type OrchestrationEvent string

const (
	OrchestrationEventStart       OrchestrationEvent = "start"
	OrchestrationEventDirectDone  OrchestrationEvent = "direct_strategy"
	OrchestrationEventSetupFailed OrchestrationEvent = "setup_failed"
	OrchestrationEventExecuteFail OrchestrationEvent = "execute_failed"
	OrchestrationEventSynthesize  OrchestrationEvent = "synthesize"
	OrchestrationEventInterrupt   OrchestrationEvent = "interrupt"
	OrchestrationEventRecover     OrchestrationEvent = "recover"
	OrchestrationEventRecoverFail OrchestrationEvent = "recover_failed"
	OrchestrationEventCancel      OrchestrationEvent = "cancel"
)

// ── Transition rules ─────────────────────────────────────────────────────────

// orchestrationTransitionRules defines the legal state transitions for an
// OrchestrationHandle. Terminal states (Completed/Failed/Cancelled) have no
// outgoing transitions.
var orchestrationTransitionRules = []shared.TransitionRule[OrchestrationStatus, OrchestrationEvent]{
	// Pending → *
	{From: OrchestrationStatusPending, Event: OrchestrationEventStart, To: OrchestrationStatusRunning},
	{From: OrchestrationStatusPending, Event: OrchestrationEventDirectDone, To: OrchestrationStatusCompleted},
	{From: OrchestrationStatusPending, Event: OrchestrationEventSetupFailed, To: OrchestrationStatusFailed},
	{From: OrchestrationStatusPending, Event: OrchestrationEventCancel, To: OrchestrationStatusCancelled},
	// Running → *
	{From: OrchestrationStatusRunning, Event: OrchestrationEventSynthesize, To: OrchestrationStatusCompleted},
	{From: OrchestrationStatusRunning, Event: OrchestrationEventExecuteFail, To: OrchestrationStatusFailed},
	{From: OrchestrationStatusRunning, Event: OrchestrationEventCancel, To: OrchestrationStatusCancelled},
	{From: OrchestrationStatusRunning, Event: OrchestrationEventInterrupt, To: OrchestrationStatusInterrupted},
	// Interrupted → *
	{From: OrchestrationStatusInterrupted, Event: OrchestrationEventRecover, To: OrchestrationStatusRunning},
	{From: OrchestrationStatusInterrupted, Event: OrchestrationEventRecoverFail, To: OrchestrationStatusFailed},
	{From: OrchestrationStatusInterrupted, Event: OrchestrationEventCancel, To: OrchestrationStatusCancelled},
}

// ── OrchestrationStateMachine ────────────────────────────────────────────────

// orchestrationStateMachine is the singleton OrchestrationStatus state machine.
var orchestrationStateMachine = shared.NewGenericStateMachine(orchestrationTransitionRules)

// OrchestrationStateMachine returns the singleton OrchestrationStatus state machine.
// Stability:evolving
func OrchestrationStateMachine() shared.StateMachine[OrchestrationStatus, OrchestrationEvent] {
	return orchestrationStateMachine
}

// CanTransitionOrchestrationStatus reports whether a direct transition from
// one OrchestrationStatus to another is valid according to the state machine.
func CanTransitionOrchestrationStatus(from, to OrchestrationStatus) bool {
	return orchestrationStateMachine.CanTransition(from, to)
}

// TransitionOrchestrationStatus validates and executes a state transition
// triggered by the given event. Returns the new state on success, or an error
// for illegal transitions.
func TransitionOrchestrationStatus(from OrchestrationStatus, event OrchestrationEvent) (OrchestrationStatus, error) {
	return orchestrationStateMachine.Transition(from, event)
}

// orchestrationEventForTarget returns the event that transitions from the
// given source state to the given target state. Returns ok=false if no such
// transition exists. Used by callers that need to validate a target state
// without knowing the specific event name.
func orchestrationEventForTarget(from, to OrchestrationStatus) (OrchestrationEvent, bool) {
	switch {
	// Pending → *
	case from == OrchestrationStatusPending && to == OrchestrationStatusRunning:
		return OrchestrationEventStart, true
	case from == OrchestrationStatusPending && to == OrchestrationStatusCompleted:
		return OrchestrationEventDirectDone, true
	case from == OrchestrationStatusPending && to == OrchestrationStatusFailed:
		return OrchestrationEventSetupFailed, true
	case from == OrchestrationStatusPending && to == OrchestrationStatusCancelled:
		return OrchestrationEventCancel, true
	// Running → *
	case from == OrchestrationStatusRunning && to == OrchestrationStatusCompleted:
		return OrchestrationEventSynthesize, true
	case from == OrchestrationStatusRunning && to == OrchestrationStatusFailed:
		return OrchestrationEventExecuteFail, true
	case from == OrchestrationStatusRunning && to == OrchestrationStatusCancelled:
		return OrchestrationEventCancel, true
	case from == OrchestrationStatusRunning && to == OrchestrationStatusInterrupted:
		return OrchestrationEventInterrupt, true
	// Interrupted → *
	case from == OrchestrationStatusInterrupted && to == OrchestrationStatusRunning:
		return OrchestrationEventRecover, true
	case from == OrchestrationStatusInterrupted && to == OrchestrationStatusFailed:
		return OrchestrationEventRecoverFail, true
	case from == OrchestrationStatusInterrupted && to == OrchestrationStatusCancelled:
		return OrchestrationEventCancel, true
	default:
		return "", false
	}
}

// IsOrchestrationTerminal returns true for terminal states that have no
// outgoing transitions.
func IsOrchestrationTerminal(state OrchestrationStatus) bool {
	switch state {
	case OrchestrationStatusCompleted, OrchestrationStatusFailed, OrchestrationStatusCancelled:
		return true
	default:
		return false
	}
}
