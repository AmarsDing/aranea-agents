// Package biz — Agent State Machine (AS-FSM-01)
//
// # Agent State Diagram
//
// ```mermaid
// stateDiagram-v2
//     [*] --> Active
//     Active --> Inactive : deactivate
//     Inactive --> Active : activate
//     Active --> Archived : archive
//     Inactive --> Archived : archive
//     Archived --> [*]
// ```
package biz

import (
	"aranea-agents/internal/biz/shared"
)

// ── Agent State & Event types ─────────────────────────────────────────────────

// AgentState enumerates all legal states of an Agent entity.
// String values match the raw strings currently used throughout the codebase.
// Stability:stable
type AgentState string

const (
	AgentStateActive   AgentState = "active"
	AgentStateInactive AgentState = "inactive"
	AgentStateArchived AgentState = "archived"
)

// AgentEvent enumerates all events that can trigger an Agent state transition.
// Stability:stable
type AgentEvent string

const (
	AgentEventDeactivate AgentEvent = "deactivate"
	AgentEventActivate   AgentEvent = "activate"
	AgentEventArchive    AgentEvent = "archive"
)

// ── Transition rules ─────────────────────────────────────────────────────────

// agentTransitionRules defines the legal state transitions for an Agent.
// Terminal states (archived) have no outgoing transitions.
var agentTransitionRules = []shared.TransitionRule[AgentState, AgentEvent]{
	{From: AgentStateActive, Event: AgentEventDeactivate, To: AgentStateInactive},
	{From: AgentStateInactive, Event: AgentEventActivate, To: AgentStateActive},
	{From: AgentStateActive, Event: AgentEventArchive, To: AgentStateArchived},
	{From: AgentStateInactive, Event: AgentEventArchive, To: AgentStateArchived},
}

// ── AgentStateMachine ─────────────────────────────────────────────────────────

// AgentStateMachine wraps the generic state machine with Agent-specific types.
// It is safe for concurrent use after construction.
// Stability:stable
type AgentStateMachine struct {
	inner *shared.GenericStateMachine[AgentState, AgentEvent]
}

// NewAgentStateMachine creates an AgentStateMachine with the standard transition rules.
func NewAgentStateMachine() *AgentStateMachine {
	return &AgentStateMachine{
		inner: shared.NewGenericStateMachine[AgentState, AgentEvent](agentTransitionRules),
	}
}

// Transition validates and executes a state transition.
// Returns the new state on success, or an error for illegal transitions.
func (sm *AgentStateMachine) Transition(from AgentState, event AgentEvent) (AgentState, error) {
	return sm.inner.Transition(from, event)
}

// CanTransition reports whether a direct transition from→to is legal.
func (sm *AgentStateMachine) CanTransition(from, to AgentState) bool {
	return sm.inner.CanTransition(from, to)
}

// ValidTargets returns all states reachable from the given state, sorted
// lexicographically.
func (sm *AgentStateMachine) ValidTargets(from AgentState) []AgentState {
	return sm.inner.ValidTargets(from)
}

// ── Helper functions ─────────────────────────────────────────────────────────

// ParseAgentState converts a raw string to an AgentState constant.
// Unrecognised strings are returned as-is (they will fail transition validation).
func ParseAgentState(s string) AgentState {
	switch AgentState(s) {
	case AgentStateActive, AgentStateInactive, AgentStateArchived:
		return AgentState(s)
	default:
		return AgentState(s)
	}
}

// IsAgentTerminal returns true for terminal states that have no outgoing transitions.
func IsAgentTerminal(state AgentState) bool {
	switch state {
	case AgentStateArchived:
		return true
	default:
		return false
	}
}

// IsAgentStateActive returns true if the agent state means the agent is
// considered "active" (i.e. operational and usable).
func IsAgentStateActive(state AgentState) bool {
	return state == AgentStateActive
}
