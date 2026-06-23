// Package biz — SkillProposal State Machine (AS-FSM-01)
//
// # SkillProposal State Diagram
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Pending
//	Pending --> Approved : approve
//	Pending --> Rejected : reject
//	Approved --> Registered : register
//	Rejected --> [*]
//	Registered --> [*]
//
// ```
package biz

import (
	"aranea-agents/internal/biz/shared"
)

// ── SkillProposal Event type ─────────────────────────────────────────────────

// SkillProposalEvent enumerates events that trigger a SkillProposal state transition.
// Stability:stable
type SkillProposalEvent string

const (
	SkillProposalEventApprove  SkillProposalEvent = "approve"
	SkillProposalEventReject   SkillProposalEvent = "reject"
	SkillProposalEventRegister SkillProposalEvent = "register"
)

// ── Transition rules ─────────────────────────────────────────────────────────

// skillProposalTransitionRules defines legal state transitions for a SkillProposal.
// Terminal states (rejected, registered) have no outgoing transitions.
var skillProposalTransitionRules = []shared.TransitionRule[SkillProposalStatus, SkillProposalEvent]{
	{From: SkillProposalStatusPending, Event: SkillProposalEventApprove, To: SkillProposalStatusApproved},
	{From: SkillProposalStatusPending, Event: SkillProposalEventReject, To: SkillProposalStatusRejected},
	{From: SkillProposalStatusApproved, Event: SkillProposalEventRegister, To: SkillProposalStatusRegistered},
}

// ── SkillProposalStateMachine ────────────────────────────────────────────────

// SkillProposalStateMachine wraps the generic state machine with SkillProposal-specific types.
// Safe for concurrent use after construction.
// Stability:stable
type SkillProposalStateMachine struct {
	inner *shared.GenericStateMachine[SkillProposalStatus, SkillProposalEvent]
}

// NewSkillProposalStateMachine creates a SkillProposalStateMachine with standard transition rules.
func NewSkillProposalStateMachine() *SkillProposalStateMachine {
	return &SkillProposalStateMachine{
		inner: shared.NewGenericStateMachine[SkillProposalStatus, SkillProposalEvent](skillProposalTransitionRules),
	}
}

// Transition validates and executes a state transition.
// Returns the new state on success, or an error for illegal transitions.
func (sm *SkillProposalStateMachine) Transition(from SkillProposalStatus, event SkillProposalEvent) (SkillProposalStatus, error) {
	return sm.inner.Transition(from, event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (sm *SkillProposalStateMachine) CanTransition(from, to SkillProposalStatus) bool {
	return sm.inner.CanTransition(from, to)
}

// ValidTargets returns all states reachable from the given state, sorted lexicographically.
func (sm *SkillProposalStateMachine) ValidTargets(from SkillProposalStatus) []SkillProposalStatus {
	return sm.inner.ValidTargets(from)
}

// ── Helper functions ─────────────────────────────────────────────────────────

// ParseSkillProposalStatus converts a raw string to a SkillProposalStatus constant.
// Unrecognised strings are returned as-is (they will fail transition validation).
func ParseSkillProposalStatus(s string) SkillProposalStatus {
	return SkillProposalStatus(s)
}

// IsSkillProposalTerminal returns true for terminal states that have no outgoing transitions.
func IsSkillProposalTerminal(state SkillProposalStatus) bool {
	switch state {
	case SkillProposalStatusRejected, SkillProposalStatusRegistered:
		return true
	default:
		return false
	}
}
