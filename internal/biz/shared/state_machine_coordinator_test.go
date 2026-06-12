package shared

import (
	"context"
	"testing"
)

func TestDefaultStateMachineCoordinator_RunAwaitingUser(t *testing.T) {
	c := NewDefaultStateMachineCoordinator()
	actions, err := c.OnStateChange(context.Background(), StateChange{
		EntityType: "run",
		EntityID:   "run-1",
		FromState:  "running",
		ToState:    "awaiting_user",
		Event:      "await",
		SessionID:  "session-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0].EntityType != "session" {
		t.Errorf("expected entity type 'session', got %s", actions[0].EntityType)
	}
	if actions[0].EntityID != "session-1" {
		t.Errorf("expected entity ID 'session-1', got %s", actions[0].EntityID)
	}
	if actions[0].TargetState != "awaiting_confirmation" {
		t.Errorf("expected target state 'awaiting_confirmation', got %s", actions[0].TargetState)
	}
}

func TestDefaultStateMachineCoordinator_RunResumed(t *testing.T) {
	c := NewDefaultStateMachineCoordinator()
	actions, err := c.OnStateChange(context.Background(), StateChange{
		EntityType: "run",
		EntityID:   "run-1",
		FromState:  "awaiting_user",
		ToState:    "running",
		Event:      "resume",
		SessionID:  "session-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0].TargetState != "running" {
		t.Errorf("expected target state 'running', got %s", actions[0].TargetState)
	}
}

func TestDefaultStateMachineCoordinator_RunCompleted_NoAction(t *testing.T) {
	c := NewDefaultStateMachineCoordinator()
	actions, err := c.OnStateChange(context.Background(), StateChange{
		EntityType: "run",
		EntityID:   "run-1",
		FromState:  "running",
		ToState:    "completed",
		Event:      "complete",
		SessionID:  "session-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 0 {
		t.Fatalf("expected 0 actions, got %d", len(actions))
	}
}

func TestDefaultStateMachineCoordinator_TeamRun_NoAction(t *testing.T) {
	c := NewDefaultStateMachineCoordinator()
	actions, err := c.OnStateChange(context.Background(), StateChange{
		EntityType: "team_run",
		EntityID:   "tr-1",
		FromState:  "running",
		ToState:    "waiting_human",
		Event:      "await_human",
		SessionID:  "session-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 0 {
		t.Fatalf("expected 0 actions (TeamRun cascade is domain-specific), got %d", len(actions))
	}
}

func TestDefaultStateMachineCoordinator_RunAwaitingUser_NoSessionID(t *testing.T) {
	c := NewDefaultStateMachineCoordinator()
	actions, err := c.OnStateChange(context.Background(), StateChange{
		EntityType: "run",
		EntityID:   "run-1",
		FromState:  "running",
		ToState:    "awaiting_user",
		Event:      "await",
		SessionID:  "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 0 {
		t.Fatalf("expected 0 actions (no session ID), got %d", len(actions))
	}
}

// Compile-time interface check
func TestDefaultStateMachineCoordinator_ImplementsInterface(t *testing.T) {
	var _ StateMachineCoordinator = (*DefaultStateMachineCoordinator)(nil)
}
