// Package biz — TeamStage State Machine (AS-FSM-01)
//
// # TeamStage State Diagram
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Pending
//	Pending --> Running : start
//	Running --> Completed : complete
//	Running --> Failed : fail
//	Running --> Cancelled : cancel
//	Running --> WaitingHuman : interrupt
//	WaitingHuman --> Running : resume
//	WaitingHuman --> Cancelled : cancel
//	WaitingHuman --> Failed : fail
//	Completed --> [*]
//	Failed --> [*]
//	Cancelled --> [*]
//	WaitingHuman --> [*]
//
// ```
package biz

import (
	"aranea-agents/internal/biz/shared"
)

// ── TeamStage Event types ────────────────────────────────────────────────────

// TeamStageEvent enumerates all events that can trigger a TeamStage state transition.
// Stability:evolving
type TeamStageEvent string

const (
	TeamStageEventStart    TeamStageEvent = "start"
	TeamStageEventComplete TeamStageEvent = "complete"
	TeamStageEventFail     TeamStageEvent = "fail"
	TeamStageEventCancel   TeamStageEvent = "cancel"
	TeamStageEventInterrupt TeamStageEvent = "interrupt"
	TeamStageEventResume   TeamStageEvent = "resume"
)

// ── Transition rules ─────────────────────────────────────────────────────────

// teamStageTransitionRules defines the legal state transitions for a TeamStage.
// Terminal states (completed, failed, cancelled) have no outgoing transitions.
// WaitingHuman is non-terminal (can resume or terminate).
var teamStageTransitionRules = []shared.TransitionRule[TeamStageStatus, TeamStageEvent]{
	{From: TeamStageStatusPending, Event: TeamStageEventStart, To: TeamStageStatusRunning},
	{From: TeamStageStatusRunning, Event: TeamStageEventComplete, To: TeamStageStatusCompleted},
	{From: TeamStageStatusRunning, Event: TeamStageEventFail, To: TeamStageStatusFailed},
	{From: TeamStageStatusRunning, Event: TeamStageEventCancel, To: TeamStageStatusCancelled},
	{From: TeamStageStatusRunning, Event: TeamStageEventInterrupt, To: TeamStageStatusWaitingHuman},
	{From: TeamStageStatusWaitingHuman, Event: TeamStageEventResume, To: TeamStageStatusRunning},
	{From: TeamStageStatusWaitingHuman, Event: TeamStageEventCancel, To: TeamStageStatusCancelled},
	{From: TeamStageStatusWaitingHuman, Event: TeamStageEventFail, To: TeamStageStatusFailed},
}

// ── TeamStageStateMachine ────────────────────────────────────────────────────

// TeamStageStateMachine wraps the generic state machine with TeamStage-specific types.
// It is safe for concurrent use after construction.
// Stability:evolving
type TeamStageStateMachine struct {
	inner *shared.GenericStateMachine[TeamStageStatus, TeamStageEvent]
}

// NewTeamStageStateMachine creates a TeamStageStateMachine with the standard transition rules.
func NewTeamStageStateMachine() *TeamStageStateMachine {
	return &TeamStageStateMachine{
		inner: shared.NewGenericStateMachine[TeamStageStatus, TeamStageEvent](teamStageTransitionRules),
	}
}

// Transition validates and executes a state transition.
// Returns the new state on success, or an error for illegal transitions.
func (sm *TeamStageStateMachine) Transition(from TeamStageStatus, event TeamStageEvent) (TeamStageStatus, error) {
	return sm.inner.Transition(from, event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (sm *TeamStageStateMachine) CanTransition(from, to TeamStageStatus) bool {
	return sm.inner.CanTransition(from, to)
}

// ValidTargets returns all states reachable from the given state, sorted
// lexicographically.
func (sm *TeamStageStateMachine) ValidTargets(from TeamStageStatus) []TeamStageStatus {
	return sm.inner.ValidTargets(from)
}

// ── Helper functions ─────────────────────────────────────────────────────────

// IsTeamStageTerminal returns true for terminal states that have no outgoing transitions.
// WaitingHuman is NOT terminal (can resume or terminate).
func IsTeamStageTerminal(status TeamStageStatus) bool {
	switch status {
	case TeamStageStatusCompleted, TeamStageStatusFailed, TeamStageStatusCancelled:
		return true
	default:
		return false
	}
}

// IsActive reports whether the state is an active (non-terminal) state.
// Pending, Running, and WaitingHuman are active; Completed/Failed/Cancelled are not.
func (s TeamStageStatus) IsActive() bool {
	return s == TeamStageStatusPending || s == TeamStageStatusRunning || s == TeamStageStatusWaitingHuman
}
