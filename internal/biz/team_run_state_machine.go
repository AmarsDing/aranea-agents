// Package biz — TeamRun State Machine (AS-FSM-01)
//
// # TeamRun State Diagram
//
// ```mermaid
// stateDiagram-v2
//     [*] --> Pending
//     Pending --> Running : start
//     Pending --> Cancelled : cancel
//     Running --> WaitingHuman : await_human
//     Running --> Success : succeed
//     Running --> Failed : fail
//     Running --> Cancelled : cancel
//     WaitingHuman --> Running : resume
//     WaitingHuman --> Success : succeed
//     WaitingHuman --> Failed : fail
//     WaitingHuman --> Cancelled : cancel
//     Success --> [*]
//     Failed --> [*]
//     Cancelled --> [*]
// ```
package biz

import (
	"aranea-agents/internal/biz/shared"
)

// ── TeamRun State & Event types ──────────────────────────────────────────────

// TeamRunState enumerates all legal states of a TeamRun entity.
// String values match the raw strings currently used throughout the codebase.
// Stability:stable
type TeamRunState string

const (
	TeamRunStatePending      TeamRunState = "pending"
	TeamRunStateRunning      TeamRunState = "running"
	TeamRunStateSuccess      TeamRunState = "success"
	TeamRunStateFailed       TeamRunState = "failed"
	TeamRunStateCancelled    TeamRunState = "cancelled"
	TeamRunStateWaitingHuman TeamRunState = "waiting_human"
)

// TeamRunEvent enumerates all events that can trigger a TeamRun state transition.
// Stability:stable
type TeamRunEvent string

const (
	TeamRunEventStart       TeamRunEvent = "start"
	TeamRunEventSucceed     TeamRunEvent = "succeed"
	TeamRunEventFail        TeamRunEvent = "fail"
	TeamRunEventCancel      TeamRunEvent = "cancel"
	TeamRunEventAwaitHuman  TeamRunEvent = "await_human"
	TeamRunEventResume      TeamRunEvent = "resume"
)

// ── Transition rules ─────────────────────────────────────────────────────────

// teamRunTransitionRules defines the legal state transitions for a TeamRun.
// Terminal states (success, failed, cancelled) have no outgoing transitions.
var teamRunTransitionRules = []shared.TransitionRule[TeamRunState, TeamRunEvent]{
	{From: TeamRunStatePending, Event: TeamRunEventStart, To: TeamRunStateRunning},
	{From: TeamRunStatePending, Event: TeamRunEventCancel, To: TeamRunStateCancelled},
	{From: TeamRunStateRunning, Event: TeamRunEventAwaitHuman, To: TeamRunStateWaitingHuman},
	{From: TeamRunStateRunning, Event: TeamRunEventSucceed, To: TeamRunStateSuccess},
	{From: TeamRunStateRunning, Event: TeamRunEventFail, To: TeamRunStateFailed},
	{From: TeamRunStateRunning, Event: TeamRunEventCancel, To: TeamRunStateCancelled},
	{From: TeamRunStateWaitingHuman, Event: TeamRunEventResume, To: TeamRunStateRunning},
	{From: TeamRunStateWaitingHuman, Event: TeamRunEventSucceed, To: TeamRunStateSuccess},
	{From: TeamRunStateWaitingHuman, Event: TeamRunEventFail, To: TeamRunStateFailed},
	{From: TeamRunStateWaitingHuman, Event: TeamRunEventCancel, To: TeamRunStateCancelled},
}

// ── TeamRunStateMachine ──────────────────────────────────────────────────────

// TeamRunStateMachine wraps the generic state machine with TeamRun-specific types.
// It is safe for concurrent use after construction.
// Stability:stable
type TeamRunStateMachine struct {
	inner *shared.GenericStateMachine[TeamRunState, TeamRunEvent]
}

// NewTeamRunStateMachine creates a TeamRunStateMachine with the standard transition rules.
func NewTeamRunStateMachine() *TeamRunStateMachine {
	return &TeamRunStateMachine{
		inner: shared.NewGenericStateMachine[TeamRunState, TeamRunEvent](teamRunTransitionRules),
	}
}

// Transition validates and executes a state transition.
// Returns the new state on success, or an error for illegal transitions.
func (sm *TeamRunStateMachine) Transition(from TeamRunState, event TeamRunEvent) (TeamRunState, error) {
	return sm.inner.Transition(from, event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (sm *TeamRunStateMachine) CanTransition(from, to TeamRunState) bool {
	return sm.inner.CanTransition(from, to)
}

// ValidTargets returns all states reachable from the given state, sorted
// lexicographically.
func (sm *TeamRunStateMachine) ValidTargets(from TeamRunState) []TeamRunState {
	return sm.inner.ValidTargets(from)
}

// ── Helper functions ─────────────────────────────────────────────────────────

// ParseTeamRunState converts a raw string to a TeamRunState constant.
// Unrecognised strings are returned as-is (they will fail transition validation).
func ParseTeamRunState(s string) TeamRunState {
	switch TeamRunState(s) {
	case TeamRunStatePending, TeamRunStateRunning, TeamRunStateSuccess,
		TeamRunStateFailed, TeamRunStateCancelled, TeamRunStateWaitingHuman:
		return TeamRunState(s)
	default:
		return TeamRunState(s)
	}
}

// IsTeamRunTerminal returns true for terminal states that have no outgoing transitions.
func IsTeamRunTerminal(state TeamRunState) bool {
	switch state {
	case TeamRunStateSuccess, TeamRunStateFailed, TeamRunStateCancelled:
		return true
	default:
		return false
	}
}
