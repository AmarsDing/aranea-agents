// Package biz — PlanBoard State Machine (AS-FSM-01)
//
// # PlanBoard State Diagram
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Planning
//	Planning --> Executing : execute
//	Planning --> Failed : fail_early
//	Executing --> Completed : complete
//	Executing --> Failed : fail
//	Executing --> PartialFailure : partial
//	Failed --> Executing : resume  (F2: 失败可恢复——重跑/跳过失败 step 后续跑)
//	Completed --> [*]
//	Failed --> [*]
//	PartialFailure --> [*]
//
// ```
package biz

import (
	"aranea-agents/internal/biz/shared"
)

// ── PlanBoard Event types ────────────────────────────────────────────────────

// PlanBoardEvent enumerates all events that can trigger a PlanBoard state transition.
// Stability:evolving
type PlanBoardEvent string

const (
	PlanBoardEventExecute   PlanBoardEvent = "execute"
	PlanBoardEventFailEarly PlanBoardEvent = "fail_early"
	PlanBoardEventComplete  PlanBoardEvent = "complete"
	PlanBoardEventFail      PlanBoardEvent = "fail"
	PlanBoardEventPartial   PlanBoardEvent = "partial"
	// PlanBoardEventResume（F2，2026-09-03 lbg-verify-planner 复盘 问题2）：
	// Failed 不再是死胡同——ResumePlanBoard 重排队/降级失败 step 后将 board
	// 拉回 Executing 续跑，任务导向而非失败即终局。
	PlanBoardEventResume PlanBoardEvent = "resume"
)

// ── Transition rules ─────────────────────────────────────────────────────────

// planBoardTransitionRules defines the legal state transitions for a PlanBoard.
// Completed/PartialFailure 无出边；Failed 仅可经 resume 回到 Executing。
var planBoardTransitionRules = []shared.TransitionRule[PlanStatus, PlanBoardEvent]{
	{From: PlanStatusPlanning, Event: PlanBoardEventExecute, To: PlanStatusExecuting},
	{From: PlanStatusPlanning, Event: PlanBoardEventFailEarly, To: PlanStatusFailed},
	{From: PlanStatusExecuting, Event: PlanBoardEventComplete, To: PlanStatusCompleted},
	{From: PlanStatusExecuting, Event: PlanBoardEventFail, To: PlanStatusFailed},
	{From: PlanStatusExecuting, Event: PlanBoardEventPartial, To: PlanStatusPartialFailure},
	{From: PlanStatusFailed, Event: PlanBoardEventResume, To: PlanStatusExecuting},
}

// ── PlanBoardStateMachine ────────────────────────────────────────────────────

// PlanBoardStateMachine wraps the generic state machine with PlanBoard-specific types.
// It is safe for concurrent use after construction.
// Stability:evolving
type PlanBoardStateMachine struct {
	inner *shared.GenericStateMachine[PlanStatus, PlanBoardEvent]
}

// NewPlanBoardStateMachine creates a PlanBoardStateMachine with the standard transition rules.
func NewPlanBoardStateMachine() *PlanBoardStateMachine {
	return &PlanBoardStateMachine{
		inner: shared.NewGenericStateMachine[PlanStatus, PlanBoardEvent](planBoardTransitionRules),
	}
}

// Transition validates and executes a state transition.
// Returns the new state on success, or an error for illegal transitions.
func (sm *PlanBoardStateMachine) Transition(from PlanStatus, event PlanBoardEvent) (PlanStatus, error) {
	return sm.inner.Transition(from, event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (sm *PlanBoardStateMachine) CanTransition(from, to PlanStatus) bool {
	return sm.inner.CanTransition(from, to)
}

// ValidTargets returns all states reachable from the given state, sorted
// lexicographically.
func (sm *PlanBoardStateMachine) ValidTargets(from PlanStatus) []PlanStatus {
	return sm.inner.ValidTargets(from)
}

// ── Helper functions ─────────────────────────────────────────────────────────

// IsPlanBoardTerminal returns true for terminal states that have no outgoing transitions.
func IsPlanBoardTerminal(status PlanStatus) bool {
	switch status {
	case PlanStatusCompleted, PlanStatusFailed, PlanStatusPartialFailure:
		return true
	default:
		return false
	}
}

// IsActive reports whether the state is an active (non-terminal) state.
// Planning and Executing are active; Completed/Failed/PartialFailure are not.
func (s PlanStatus) IsActive() bool {
	return s == PlanStatusPlanning || s == PlanStatusExecuting
}
