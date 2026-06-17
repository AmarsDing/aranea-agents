package biz

import (
	"testing"

	"aranea-agents/pkg/loggateway"
)

// TestApplyExecTransition_LegalTransition_AllowsAndApplies verifies that
// applyExecTransition applies a legal transition and returns nil.
func TestApplyExecTransition_LegalTransition_AllowsAndApplies(t *testing.T) {
	uc := &GraphExecutionUsecase{
		sm: NewGraphExecutionStateMachine(),
		lg: loggateway.NewNoop(),
	}
	exec := NewGraphExecution(nil, "exec-1", "graph-1", "sess-1", string(GraphExecRunning))

	err := uc.applyExecTransition(exec, GraphExecEventComplete)

	if err != nil {
		t.Fatalf("applyExecTransition(running, complete): unexpected error: %v", err)
	}
	if exec.Status != string(GraphExecCompleted) {
		t.Errorf("after legal transition: Status = %q, want %q", exec.Status, GraphExecCompleted)
	}
}

// TestApplyExecTransition_IllegalTransition_RejectsAndPreservesStatus verifies that
// applyExecTransition rejects an illegal transition, returns an error, and
// does NOT modify exec.Status (authoritative mode, not advisory).
func TestApplyExecTransition_IllegalTransition_RejectsAndPreservesStatus(t *testing.T) {
	uc := &GraphExecutionUsecase{
		sm: NewGraphExecutionStateMachine(),
		lg: loggateway.NewNoop(),
	}
	exec := NewGraphExecution(nil, "exec-2", "graph-1", "sess-1", string(GraphExecCompleted))

	err := uc.applyExecTransition(exec, GraphExecEventFail)

	if err == nil {
		t.Fatal("applyExecTransition(completed, fail): expected error for illegal transition, got nil")
	}
	if exec.Status != string(GraphExecCompleted) {
		t.Errorf("after illegal transition: Status = %q, want %q (must be preserved)", exec.Status, GraphExecCompleted)
	}
}

// TestApplyExecTransition_TerminalToFail_Rejected verifies that transitioning
// from a terminal state (completed) to failed is rejected.
func TestApplyExecTransition_TerminalToFail_Rejected(t *testing.T) {
	uc := &GraphExecutionUsecase{
		sm: NewGraphExecutionStateMachine(),
		lg: loggateway.NewNoop(),
	}
	for _, terminal := range []GraphExecutionState{GraphExecCompleted, GraphExecFailed, GraphExecCancelled} {
		exec := NewGraphExecution(nil, "exec-term", "graph-1", "sess-1", string(terminal))
		err := uc.applyExecTransition(exec, GraphExecEventFail)
		if err == nil {
			t.Errorf("applyExecTransition(%q, fail): expected error from terminal state", terminal)
		}
		if exec.Status != string(terminal) {
			t.Errorf("after illegal transition from %q: Status = %q, want %q", terminal, exec.Status, terminal)
		}
	}
}

// TestApplyExecTransition_WaitingHumanToFail_Allowed verifies that the state machine
// allows transitioning from WaitingHuman to Failed (e.g., node error during HITL).
// This transition was previously missing and caused advisory-mode fallbacks.
func TestApplyExecTransition_WaitingHumanToFail_Allowed(t *testing.T) {
	uc := &GraphExecutionUsecase{
		sm: NewGraphExecutionStateMachine(),
		lg: loggateway.NewNoop(),
	}
	exec := NewGraphExecution(nil, "exec-hitl", "graph-1", "sess-1", string(GraphExecWaitingHuman))

	err := uc.applyExecTransition(exec, GraphExecEventFail)

	if err != nil {
		t.Fatalf("applyExecTransition(waiting_human, fail): unexpected error: %v", err)
	}
	if exec.Status != string(GraphExecFailed) {
		t.Errorf("after HITL fail: Status = %q, want %q", exec.Status, GraphExecFailed)
	}
}

// TestApplyExecTransition_RunningToWaitingHuman_Allowed verifies the interrupt transition.
func TestApplyExecTransition_RunningToWaitingHuman_Allowed(t *testing.T) {
	uc := &GraphExecutionUsecase{
		sm: NewGraphExecutionStateMachine(),
		lg: loggateway.NewNoop(),
	}
	exec := NewGraphExecution(nil, "exec-int", "graph-1", "sess-1", string(GraphExecRunning))

	err := uc.applyExecTransition(exec, GraphExecEventInterrupt)

	if err != nil {
		t.Fatalf("applyExecTransition(running, interrupt): unexpected error: %v", err)
	}
	if exec.Status != string(GraphExecWaitingHuman) {
		t.Errorf("after interrupt: Status = %q, want %q", exec.Status, GraphExecWaitingHuman)
	}
}

// TestApplyExecTransition_WaitingHumanToRunning_Allowed verifies the resume transition.
func TestApplyExecTransition_WaitingHumanToRunning_Allowed(t *testing.T) {
	uc := &GraphExecutionUsecase{
		sm: NewGraphExecutionStateMachine(),
		lg: loggateway.NewNoop(),
	}
	exec := NewGraphExecution(nil, "exec-resume", "graph-1", "sess-1", string(GraphExecWaitingHuman))

	err := uc.applyExecTransition(exec, GraphExecEventResume)

	if err != nil {
		t.Fatalf("applyExecTransition(waiting_human, resume): unexpected error: %v", err)
	}
	if exec.Status != string(GraphExecRunning) {
		t.Errorf("after resume: Status = %q, want %q", exec.Status, GraphExecRunning)
	}
}
