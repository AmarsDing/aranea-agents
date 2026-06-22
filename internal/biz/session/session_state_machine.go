// Package session — Session State Machine (AS-FSM-01)
//
// # Session State Diagram
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Idle
//	Idle --> Running : start
//	Running --> Completed : complete
//	Running --> Interrupted : interrupt
//	Running --> AwaitingConfirmation : await_confirmation
//	Completed --> Running : start
//	Interrupted --> Running : resume
//	AwaitingConfirmation --> Running : resume
//	AwaitingConfirmation --> Interrupted : cancel
//	Completed --> [*]
//	Interrupted --> [*]
//
// ```
package session

import (
	"aranea-agents/internal/biz/shared"
)

// ── Session State & Event types ──────────────────────────────────────────────

// SessionState enumerates all legal states of a Session entity.
// String values match the SessionStatus constants used throughout the codebase.
// Stability:evolving
type SessionState string

const (
	SessionStateIdle                 SessionState = "idle"
	SessionStateRunning              SessionState = "running"
	SessionStateCompleted            SessionState = "completed"
	SessionStateInterrupted          SessionState = "interrupted"
	SessionStateAwaitingConfirmation SessionState = "awaiting_confirmation"
)

// SessionEvent enumerates all events that can trigger a Session state transition.
// Stability:evolving
type SessionEvent string

const (
	SessionEventStart             SessionEvent = "start"
	SessionEventComplete          SessionEvent = "complete"
	SessionEventInterrupt         SessionEvent = "interrupt"
	SessionEventAwaitConfirmation SessionEvent = "await_confirmation"
	SessionEventResume            SessionEvent = "resume"
	SessionEventCancel            SessionEvent = "cancel"
)

// ── Transition rules ─────────────────────────────────────────────────────────

// sessionTransitionRules defines the legal state transitions for a Session.
// Terminal states (completed, interrupted) have no outgoing transitions,
// except completed → running (re-run).
var sessionTransitionRules = []shared.TransitionRule[SessionState, SessionEvent]{
	{From: SessionStateIdle, Event: SessionEventStart, To: SessionStateRunning},
	{From: SessionStateRunning, Event: SessionEventComplete, To: SessionStateCompleted},
	{From: SessionStateRunning, Event: SessionEventInterrupt, To: SessionStateInterrupted},
	{From: SessionStateRunning, Event: SessionEventAwaitConfirmation, To: SessionStateAwaitingConfirmation},
	{From: SessionStateCompleted, Event: SessionEventStart, To: SessionStateRunning},
	{From: SessionStateInterrupted, Event: SessionEventResume, To: SessionStateRunning},
	{From: SessionStateAwaitingConfirmation, Event: SessionEventResume, To: SessionStateRunning},
	{From: SessionStateAwaitingConfirmation, Event: SessionEventCancel, To: SessionStateInterrupted},
}

// ── SessionStateMachine ──────────────────────────────────────────────────────

// SessionStateMachine wraps the generic state machine with Session-specific types.
// It is safe for concurrent use after construction.
// Stability:evolving
type SessionStateMachine struct {
	inner *shared.GenericStateMachine[SessionState, SessionEvent]
}

// NewSessionStateMachine creates a SessionStateMachine with the standard transition rules.
func NewSessionStateMachine() *SessionStateMachine {
	return &SessionStateMachine{
		inner: shared.NewGenericStateMachine[SessionState, SessionEvent](sessionTransitionRules),
	}
}

// Transition validates and executes a state transition.
// Returns the new state on success, or an error for illegal transitions.
func (sm *SessionStateMachine) Transition(from SessionState, event SessionEvent) (SessionState, error) {
	return sm.inner.Transition(from, event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (sm *SessionStateMachine) CanTransition(from, to SessionState) bool {
	return sm.inner.CanTransition(from, to)
}

// ValidTargets returns all states reachable from the given state, sorted
// lexicographically.
func (sm *SessionStateMachine) ValidTargets(from SessionState) []SessionState {
	return sm.inner.ValidTargets(from)
}

// Compile-time interface check.
var _ shared.StateMachine[SessionState, SessionEvent] = (*SessionStateMachine)(nil)

// ── Helper functions ─────────────────────────────────────────────────────────

// ParseSessionState converts a raw string to a SessionState constant.
// Unrecognised strings are returned as-is (they will fail transition validation).
func ParseSessionState(s string) SessionState {
	switch SessionState(s) {
	case SessionStateIdle, SessionStateRunning, SessionStateCompleted,
		SessionStateInterrupted, SessionStateAwaitingConfirmation:
		return SessionState(s)
	default:
		return SessionState(s)
	}
}

// IsSessionTerminal returns true for terminal states that have no outgoing transitions.
// Note: completed is considered terminal even though it can transition back to running
// via a re-run event — the terminal check is used for "active session" guards.
func IsSessionTerminal(state SessionState) bool {
	switch state {
	case SessionStateCompleted, SessionStateInterrupted:
		return true
	default:
		return false
	}
}
