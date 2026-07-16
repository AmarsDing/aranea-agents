// Package biz — TeamRun v2 State Machine (AS-FSM-01)
//
// # TeamRun v2 State Diagram
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Running
//	Running --> Completed : complete
//	Running --> Failed : fail
//	Running --> Cancelled : cancel
//	Running --> Paused : pause
//	Paused --> Running : unpause
//	Paused --> Cancelled : cancel
//	Paused --> Failed : fail
//	Completed --> [*]
//	Failed --> [*]
//	Cancelled --> [*]
//
// ```
//
// 注意：本文件是 v2 TeamRun 的状态机。v1 TeamRun 状态机在 team_run_state_machine.go，
// 使用无类型字符串常量（TeamRunStatusRunning 等），与 v2 的 TeamRunV2Status 类型不兼容。
// 命名加上 V2 后缀以避免冲突。
package biz

import (
	"aranea-agents/internal/biz/shared"
)

// ── TeamRun v2 Event types ───────────────────────────────────────────────────

// TeamRunV2Event enumerates all events that can trigger a TeamRun v2 state transition.
// Stability:evolving
type TeamRunV2Event string

const (
	TeamRunV2EventComplete TeamRunV2Event = "complete"
	TeamRunV2EventFail     TeamRunV2Event = "fail"
	TeamRunV2EventCancel   TeamRunV2Event = "cancel"
	TeamRunV2EventPause    TeamRunV2Event = "pause"
	TeamRunV2EventUnpause  TeamRunV2Event = "unpause"
)

// ── Transition rules ─────────────────────────────────────────────────────────

// teamRunV2TransitionRules defines the legal state transitions for a TeamRun v2.
// Terminal states (completed, failed, cancelled) have no outgoing transitions.
// Pause/unpause are non-terminal (MVP cancel+marker semantics).
var teamRunV2TransitionRules = []shared.TransitionRule[TeamRunV2Status, TeamRunV2Event]{
	{From: TeamRunV2StatusRunning, Event: TeamRunV2EventComplete, To: TeamRunV2StatusCompleted},
	{From: TeamRunV2StatusRunning, Event: TeamRunV2EventFail, To: TeamRunV2StatusFailed},
	{From: TeamRunV2StatusRunning, Event: TeamRunV2EventCancel, To: TeamRunV2StatusCancelled},
	{From: TeamRunV2StatusRunning, Event: TeamRunV2EventPause, To: TeamRunV2StatusPaused},
	{From: TeamRunV2StatusPaused, Event: TeamRunV2EventUnpause, To: TeamRunV2StatusRunning},
	{From: TeamRunV2StatusPaused, Event: TeamRunV2EventCancel, To: TeamRunV2StatusCancelled},
	{From: TeamRunV2StatusPaused, Event: TeamRunV2EventFail, To: TeamRunV2StatusFailed},
}

// ── TeamRunV2StateMachine ────────────────────────────────────────────────────

// TeamRunV2StateMachine wraps the generic state machine with TeamRun v2-specific types.
// It is safe for concurrent use after construction.
// Stability:evolving
type TeamRunV2StateMachine struct {
	inner *shared.GenericStateMachine[TeamRunV2Status, TeamRunV2Event]
}

// NewTeamRunV2StateMachine creates a TeamRunV2StateMachine with the standard transition rules.
func NewTeamRunV2StateMachine() *TeamRunV2StateMachine {
	return &TeamRunV2StateMachine{
		inner: shared.NewGenericStateMachine[TeamRunV2Status, TeamRunV2Event](teamRunV2TransitionRules),
	}
}

// Transition validates and executes a state transition.
// Returns the new state on success, or an error for illegal transitions.
func (sm *TeamRunV2StateMachine) Transition(from TeamRunV2Status, event TeamRunV2Event) (TeamRunV2Status, error) {
	return sm.inner.Transition(from, event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (sm *TeamRunV2StateMachine) CanTransition(from, to TeamRunV2Status) bool {
	return sm.inner.CanTransition(from, to)
}

// ValidTargets returns all states reachable from the given state, sorted
// lexicographically.
func (sm *TeamRunV2StateMachine) ValidTargets(from TeamRunV2Status) []TeamRunV2Status {
	return sm.inner.ValidTargets(from)
}

// ── Helper functions ─────────────────────────────────────────────────────────

// IsTeamRunV2Terminal returns true for terminal states that have no outgoing transitions.
func IsTeamRunV2Terminal(status TeamRunV2Status) bool {
	switch status {
	case TeamRunV2StatusCompleted, TeamRunV2StatusFailed, TeamRunV2StatusCancelled:
		return true
	default:
		return false
	}
}

// IsActive reports whether the state is an active (non-terminal) state.
// Only Running is active; Completed/Failed/Cancelled are not.
func (s TeamRunV2Status) IsActive() bool {
	return s == TeamRunV2StatusRunning
}
