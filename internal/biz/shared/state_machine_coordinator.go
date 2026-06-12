// Package shared — State Machine Coordinator (AS-FSM-01 cross-entity coordination)
//
// StateMachineCoordinator defines the contract for cross-entity state coordination.
// When one entity's state changes, related entities may need to transition as well.
// This replaces the manual status-pairing scattered across service/biz layers.
package shared

import "context"

// CrossEntityAction describes a state transition that should be applied to a related entity
// as a consequence of a source entity's state change.
type CrossEntityAction struct {
	EntityType  string // "session", "run", "team_run", "graph_execution"
	EntityID    string // ID of the target entity
	TargetState string // The state the target entity should transition to
	Reason      string // Why this transition is happening
}

// StateChange represents a state transition that has occurred on a source entity.
type StateChange struct {
	EntityType string // "run", "team_run", "session", "graph_execution"
	EntityID   string
	FromState  string
	ToState    string
	Event      string // The event that triggered the transition
	SessionID  string // The session this entity belongs to (for correlation)
}

// StateMachineCoordinator coordinates cross-entity state transitions.
//
// Implementations must be idempotent — if the target entity is already in the
// desired state, the action should be a no-op. Implementations must also handle
// the case where the target entity is not found (log warning, don't error).
//
// Stability:evolving
type StateMachineCoordinator interface {
	// OnStateChange evaluates cross-entity consequences of a state change
	// and returns a list of actions that should be applied.
	// This is a pure function — it does NOT execute the actions, only computes them.
	// The caller is responsible for executing the actions (typically in the service layer).
	OnStateChange(ctx context.Context, change StateChange) ([]CrossEntityAction, error)
}
