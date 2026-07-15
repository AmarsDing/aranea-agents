package agent

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// TestTransitionOrchestrationStatus_IllegalRejected (C-19) verifies fail-closed
// FSM: illegal transitions return an error and do NOT mutate handle.Status.
func TestTransitionOrchestrationStatus_IllegalRejected(t *testing.T) {
	o := &TaskOrchestratorImpl{lg: loggateway.NewNoop()}
	handle := &biz.OrchestrationHandle{
		ID:     "orch-fsm-1",
		Status: biz.OrchestrationStatusCompleted,
	}

	err := o.transitionOrchestrationStatus(context.Background(), handle, biz.OrchestrationStatusFailed)
	if err == nil {
		t.Fatal("expected error for illegal completed → failed transition")
	}
	if handle.Status != biz.OrchestrationStatusCompleted {
		t.Fatalf("status = %q, want preserved completed", handle.Status)
	}
}

// TestTransitionOrchestrationStatus_LegalApplies (C-19) verifies a legal
// transition updates status and returns nil.
func TestTransitionOrchestrationStatus_LegalApplies(t *testing.T) {
	o := &TaskOrchestratorImpl{lg: loggateway.NewNoop()}
	handle := &biz.OrchestrationHandle{
		ID:     "orch-fsm-2",
		Status: biz.OrchestrationStatusPending,
	}

	err := o.transitionOrchestrationStatus(context.Background(), handle, biz.OrchestrationStatusRunning)
	if err != nil {
		t.Fatalf("unexpected error for legal pending → running: %v", err)
	}
	if handle.Status != biz.OrchestrationStatusRunning {
		t.Fatalf("status = %q, want running", handle.Status)
	}
}

// TestTransitionOrchestrationStatus_SameStatusNoop (C-19) verifies from==to is a no-op.
func TestTransitionOrchestrationStatus_SameStatusNoop(t *testing.T) {
	o := &TaskOrchestratorImpl{lg: loggateway.NewNoop()}
	handle := &biz.OrchestrationHandle{
		ID:     "orch-fsm-3",
		Status: biz.OrchestrationStatusRunning,
	}
	if err := o.transitionOrchestrationStatus(context.Background(), handle, biz.OrchestrationStatusRunning); err != nil {
		t.Fatalf("same-status transition: %v", err)
	}
}
