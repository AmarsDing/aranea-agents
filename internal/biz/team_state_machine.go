// Package biz — Team State Machine (AS-FSM-01)
//
// # Team State Diagram
//
// ```mermaid
// stateDiagram-v2
//     [*] --> Pending
//     Pending --> Running : start
//     Pending --> Cancelled : cancel
//     Running --> Completed : complete
//     Running --> Failed : fail
//     Running --> Cancelled : cancel
//     Running --> Interrupted : interrupt
//     Interrupted --> Running : recover
//     Completed --> Archived : archive
//     Failed --> Archived : archive
//     Failed --> Pending : recover
//     Cancelled --> Archived : archive
//     Cancelled --> Pending : recover
//     Archived --> [*]
// ```
package biz

import (
	"aranea-agents/internal/biz/shared"
)

// ── Team State & Event types ─────────────────────────────────────────────────

// TeamState enumerates all legal states of a Team entity.
// String values match the raw strings currently used throughout the codebase.
// Stability:stable
type TeamState string

const (
	TeamStatePending     TeamState = "pending"
	TeamStateRunning     TeamState = "running"
	TeamStateCompleted   TeamState = "completed"
	TeamStateFailed      TeamState = "failed"
	TeamStateCancelled   TeamState = "cancelled"
	TeamStateInterrupted TeamState = "interrupted"
	TeamStateArchived    TeamState = "archived"

	// TeamStateBlocked is a virtual state used only in cascade blocked results.
	// It is never persisted and has no transitions.
	TeamStateBlocked TeamState = "blocked"
)

// TeamEvent enumerates all events that can trigger a Team state transition.
// Stability:stable
type TeamEvent string

const (
	TeamEventStart    TeamEvent = "start"
	TeamEventComplete TeamEvent = "complete"
	TeamEventFail     TeamEvent = "fail"
	TeamEventCancel   TeamEvent = "cancel"
	TeamEventInterrupt TeamEvent = "interrupt"
	TeamEventRecover  TeamEvent = "recover"
	TeamEventArchive  TeamEvent = "archive"
)

// ── Transition rules ─────────────────────────────────────────────────────────

// teamTransitionRules defines the legal state transitions for a Team.
// Terminal states (archived) have no outgoing transitions.
var teamTransitionRules = []shared.TransitionRule[TeamState, TeamEvent]{
	{From: TeamStatePending, Event: TeamEventStart, To: TeamStateRunning},
	{From: TeamStatePending, Event: TeamEventCancel, To: TeamStateCancelled},
	{From: TeamStateRunning, Event: TeamEventComplete, To: TeamStateCompleted},
	{From: TeamStateRunning, Event: TeamEventFail, To: TeamStateFailed},
	{From: TeamStateRunning, Event: TeamEventCancel, To: TeamStateCancelled},
	{From: TeamStateRunning, Event: TeamEventInterrupt, To: TeamStateInterrupted},
	{From: TeamStateInterrupted, Event: TeamEventRecover, To: TeamStateRunning},
	{From: TeamStateCompleted, Event: TeamEventArchive, To: TeamStateArchived},
	{From: TeamStateFailed, Event: TeamEventArchive, To: TeamStateArchived},
	{From: TeamStateFailed, Event: TeamEventRecover, To: TeamStatePending},
	{From: TeamStateCancelled, Event: TeamEventArchive, To: TeamStateArchived},
	{From: TeamStateCancelled, Event: TeamEventRecover, To: TeamStatePending},
}

// ── TeamStateMachine ─────────────────────────────────────────────────────────

// TeamStateMachine wraps the generic state machine with Team-specific types.
// It is safe for concurrent use after construction.
// Stability:stable
type TeamStateMachine struct {
	inner *shared.GenericStateMachine[TeamState, TeamEvent]
}

// NewTeamStateMachine creates a TeamStateMachine with the standard transition rules.
func NewTeamStateMachine() *TeamStateMachine {
	return &TeamStateMachine{
		inner: shared.NewGenericStateMachine[TeamState, TeamEvent](teamTransitionRules),
	}
}

// Transition validates and executes a state transition.
// Returns the new state on success, or an error for illegal transitions.
func (sm *TeamStateMachine) Transition(from TeamState, event TeamEvent) (TeamState, error) {
	return sm.inner.Transition(from, event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (sm *TeamStateMachine) CanTransition(from, to TeamState) bool {
	return sm.inner.CanTransition(from, to)
}

// ValidTargets returns all states reachable from the given state, sorted
// lexicographically.
func (sm *TeamStateMachine) ValidTargets(from TeamState) []TeamState {
	return sm.inner.ValidTargets(from)
}

// ── Helper functions ─────────────────────────────────────────────────────────

// ParseTeamState converts a raw string to a TeamState constant.
// Unrecognised strings are returned as-is (they will fail transition validation).
func ParseTeamState(s string) TeamState {
	switch TeamState(s) {
	case TeamStatePending, TeamStateRunning, TeamStateCompleted,
		TeamStateFailed, TeamStateCancelled, TeamStateInterrupted,
		TeamStateArchived, TeamStateBlocked:
		return TeamState(s)
	default:
		return TeamState(s)
	}
}

// IsTeamTerminal returns true for terminal states that have no outgoing transitions.
func IsTeamTerminal(state TeamState) bool {
	switch state {
	case TeamStateArchived:
		return true
	default:
		return false
	}
}

// IsTeamStateActive returns true if the team state means the team is
// considered "active" (i.e. not terminal and not deleted).
func IsTeamStateActive(state TeamState) bool {
	return state == TeamStatePending || state == TeamStateRunning || state == TeamStateInterrupted
}
