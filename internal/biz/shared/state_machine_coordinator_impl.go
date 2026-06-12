package shared

import "context"

// DefaultStateMachineCoordinator implements StateMachineCoordinator with
// rule-based cross-entity coordination.
//
// The coordination rules are:
// 1. Run → awaiting_user ⇒ Session → awaiting_confirmation
// 2. Run → running (from awaiting_user) ⇒ Session → running (if Session was awaiting_confirmation)
// 3. TeamRun → waiting_human ⇒ (no automatic Run cascade — handled by TeamGraphRunCoordinator)
// 4. GraphExecution → interrupted ⇒ (no automatic Run cascade — handled by GraphInterruptAdapter)
//
// Rules 3 and 4 are intentionally NOT automated here because they require
// domain-specific context (which Runs belong to which TeamRun, which Graph node
// was interrupted) that the coordinator should not possess. Those cascades are
// handled by their respective domain coordinators.
//
// Stability:evolving
type DefaultStateMachineCoordinator struct{}

// NewDefaultStateMachineCoordinator creates a new coordinator.
func NewDefaultStateMachineCoordinator() *DefaultStateMachineCoordinator {
	return &DefaultStateMachineCoordinator{}
}

// OnStateChange evaluates cross-entity consequences.
func (c *DefaultStateMachineCoordinator) OnStateChange(ctx context.Context, change StateChange) ([]CrossEntityAction, error) {
	var actions []CrossEntityAction

	switch change.EntityType {
	case "run":
		actions = c.onRunStateChange(change)
	case "team_run":
		actions = c.onTeamRunStateChange(change)
	case "session":
		// Session state changes don't currently cascade to other entities
	case "graph_execution":
		// Graph execution state changes are handled by domain-specific coordinators
	}

	return actions, nil
}

func (c *DefaultStateMachineCoordinator) onRunStateChange(change StateChange) []CrossEntityAction {
	switch change.ToState {
	case "awaiting_user":
		// When a Run enters awaiting_user, the Session should enter awaiting_confirmation
		if change.SessionID != "" {
			return []CrossEntityAction{{
				EntityType:  "session",
				EntityID:    change.SessionID,
				TargetState: "awaiting_confirmation",
				Reason:      "run_awaiting_user",
			}}
		}
	case "running":
		// When a Run resumes from awaiting_user, the Session should resume too
		if change.FromState == "awaiting_user" && change.SessionID != "" {
			return []CrossEntityAction{{
				EntityType:  "session",
				EntityID:    change.SessionID,
				TargetState: "running",
				Reason:      "run_resumed",
			}}
		}
	}
	return nil
}

func (c *DefaultStateMachineCoordinator) onTeamRunStateChange(change StateChange) []CrossEntityAction {
	// TeamRun state changes are handled by TeamGraphRunCoordinator.
	// The coordinator does not auto-cascade TeamRun → Run because it lacks
	// the context of which Runs belong to which TeamRun.
	return nil
}
