package biz

import "aranea-agents/pkg/apierror"

// 保留理由（2026-08-20 P2-2 盘点）：本状态机不迁移到 biz/shared.GenericStateMachine，
// 原因：① 非法转换须返回 apierror.BadRequest(DomainTeam) 以保证 service 层 400 映射，
// shared 仅返回 sentinel 错误会导致 400→500 回归；② from=="" 归一化为 pending、
// 终态表、Validate 便捷变体均为 shared 未覆盖特性；③ 与 team_run_v2_state_machine.go
// 是不同实体（v1 七态含 waiting_human），非重复实现。AS-FSM-01 合规：显式转换表。

// TeamRun status constants for the TeamRun lifecycle state machine.
// These are intentionally kept as string constants for JSON/DB compatibility.
// Stability:internal
const (
	TeamRunStatusPending      = "pending"
	TeamRunStatusRunning      = "running"
	TeamRunStatusSuccess      = "success"
	TeamRunStatusFailed       = "failed"
	TeamRunStatusCancelled    = "cancelled"
	TeamRunStatusWaitingHuman = "waiting_human"
	TeamRunStatusPaused       = "paused"
)

// TeamRunEvent represents an event that can trigger a TeamRun status transition.
// Stability:internal
type TeamRunEvent string

const (
	TeamRunEventStart     TeamRunEvent = "start"
	TeamRunEventComplete  TeamRunEvent = "complete"
	TeamRunEventFail      TeamRunEvent = "fail"
	TeamRunEventCancel    TeamRunEvent = "cancel"
	TeamRunEventWaitHuman TeamRunEvent = "wait_human"
	TeamRunEventResume    TeamRunEvent = "resume"
	TeamRunEventPause     TeamRunEvent = "pause"
	TeamRunEventUnpause   TeamRunEvent = "unpause"
)

// teamRunTransitions defines the legal TeamRun status transitions.
// Each (from, event) maps to exactly one target status.
// Stability:internal
var teamRunTransitions = map[string]map[TeamRunEvent]string{
	TeamRunStatusPending: {
		TeamRunEventStart:  TeamRunStatusRunning,
		TeamRunEventCancel: TeamRunStatusCancelled,
	},
	TeamRunStatusRunning: {
		TeamRunEventWaitHuman: TeamRunStatusWaitingHuman,
		TeamRunEventPause:     TeamRunStatusPaused,
		TeamRunEventComplete:  TeamRunStatusSuccess,
		TeamRunEventFail:      TeamRunStatusFailed,
		TeamRunEventCancel:    TeamRunStatusCancelled,
	},
	TeamRunStatusWaitingHuman: {
		TeamRunEventResume:   TeamRunStatusRunning,
		TeamRunEventComplete: TeamRunStatusSuccess,
		TeamRunEventFail:     TeamRunStatusFailed,
		TeamRunEventCancel:   TeamRunStatusCancelled,
	},
	TeamRunStatusPaused: {
		TeamRunEventUnpause:  TeamRunStatusRunning,
		TeamRunEventComplete: TeamRunStatusSuccess,
		TeamRunEventFail:     TeamRunStatusFailed,
		TeamRunEventCancel:   TeamRunStatusCancelled,
	},
	TeamRunStatusSuccess:   {},
	TeamRunStatusFailed:    {},
	TeamRunStatusCancelled: {},
}

// teamRunTerminalStatuses lists statuses that cannot be left without creating a new run.
var teamRunTerminalStatuses = map[string]bool{
	TeamRunStatusSuccess:   true,
	TeamRunStatusFailed:    true,
	TeamRunStatusCancelled: true,
}

// IsTeamRunTerminalStatus reports whether the given TeamRun status is terminal.
func IsTeamRunTerminalStatus(status string) bool {
	return teamRunTerminalStatuses[status]
}

// TeamRunState is a typed alias for TeamRun status used by the state machine.
// Stability:internal
type TeamRunState string

// NewTeamRunStateMachine returns a TeamRunStateMachine ready for transition checks.
// Stability:internal
func NewTeamRunStateMachine() *TeamRunStateMachine {
	return &TeamRunStateMachine{}
}

// TeamRunStateMachine validates TeamRun status transitions.
// Stability:internal
type TeamRunStateMachine struct{}

// CanTransition reports whether transitioning from "from" to "to" is legal.
func (m *TeamRunStateMachine) CanTransition(from, to TeamRunState) bool {
	for _, target := range teamRunTransitions[string(from)] {
		if target == string(to) {
			return true
		}
	}
	return false
}

// TransitionTeamRunStatus returns the target status for a (from, event) pair.
// It returns an error if the transition is not defined.
// Stability:internal
func TransitionTeamRunStatus(from string, event TeamRunEvent) (string, error) {
	if from == "" {
		from = TeamRunStatusPending
	}
	events, ok := teamRunTransitions[from]
	if !ok {
		return "", apierror.BadRequest(apierror.DomainTeam, "unknown team run status %q", from)
	}
	to, ok := events[event]
	if !ok {
		return "", apierror.BadRequest(apierror.DomainTeam, "invalid team run transition from %q on event %q", from, event)
	}
	return to, nil
}

// ValidateTeamRunStatusTransition returns nil if the transition is legal.
// It is a convenience wrapper for callers that already know the target status.
// Stability:internal
func ValidateTeamRunStatusTransition(from, to string) error {
	if from == "" {
		from = TeamRunStatusPending
	}
	events, ok := teamRunTransitions[from]
	if !ok {
		return apierror.BadRequest(apierror.DomainTeam, "unknown team run status %q", from)
	}
	for _, target := range events {
		if target == to {
			return nil
		}
	}
	return apierror.BadRequest(apierror.DomainTeam, "invalid team run transition from %q to %q", from, to)
}
