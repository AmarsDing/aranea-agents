package tools

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// TestExecuteOrchestratePhase_UsesPlanBoardID (C-18) verifies the orchestration
// handle ID is the PlanBoard.ID — not a minted orch_* UUID.
func TestExecuteOrchestratePhase_UsesPlanBoardID(t *testing.T) {
	plan := &biz.TaskPlan{
		ID:              "tp-1",
		SpiritSessionID: "sess-1",
		Strategy:        biz.StrategyDAG,
		SubTasks:        []biz.SubTask{{ID: "st-1", Name: "step"}},
	}
	alloc := &biz.AllocationPlan{ID: "alloc-1"}
	deps := planAndExecuteDeps{lg: loggateway.NewNoop()}

	handle, step, err := executeOrchestratePhase(context.Background(), plan, alloc, "pb_canonical_123", deps)
	if err != nil {
		t.Fatalf("executeOrchestratePhase: %v", err)
	}
	if handle.ID != "pb_canonical_123" {
		t.Fatalf("handle.ID = %q, want pb_canonical_123", handle.ID)
	}
	if step.Status != "running" {
		t.Fatalf("step.Status = %q, want running", step.Status)
	}
}

// TestExecuteOrchestratePhase_EmptyBoardIDFails (C-18) verifies missing PlanBoard
// ID fails closed instead of minting orch_*.
func TestExecuteOrchestratePhase_EmptyBoardIDFails(t *testing.T) {
	plan := &biz.TaskPlan{ID: "tp-1", SpiritSessionID: "sess-1"}
	alloc := &biz.AllocationPlan{ID: "alloc-1"}
	deps := planAndExecuteDeps{lg: loggateway.NewNoop()}

	handle, step, err := executeOrchestratePhase(context.Background(), plan, alloc, "", deps)
	if err == nil {
		t.Fatal("expected error for empty planBoardID")
	}
	if handle != nil {
		t.Fatalf("handle = %+v, want nil", handle)
	}
	if step.Status != "failed" {
		t.Fatalf("step.Status = %q, want failed", step.Status)
	}
}

type stubOrch struct {
	progress  []biz.TaskProgress
	progErr   error
	cancelErr error
}

func (s *stubOrch) Orchestrate(context.Context, *biz.TaskPlan, *biz.AllocationPlan) (*biz.OrchestrationHandle, error) {
	return nil, nil
}
func (s *stubOrch) CheckProgress(context.Context, string) ([]biz.TaskProgress, error) {
	return s.progress, s.progErr
}
func (s *stubOrch) Cancel(context.Context, string) error { return s.cancelErr }
func (s *stubOrch) Synthesize(context.Context, string) (*biz.SynthesisOutput, error) {
	return nil, nil
}
func (s *stubOrch) Recover(context.Context, string) error       { return nil }
func (s *stubOrch) RecoverAllInterrupted(context.Context) error { return nil }

var _ biz.TaskOrchestratorPort = (*stubOrch)(nil)

type stubBoardFallback struct {
	progress    []biz.TaskProgress
	progErr     error
	cancelErr   error
	cancelCalls int
}

func (s *stubBoardFallback) CheckPlanBoardProgress(context.Context, string) ([]biz.TaskProgress, error) {
	return s.progress, s.progErr
}
func (s *stubBoardFallback) CancelPlanBoard(context.Context, string) error {
	s.cancelCalls++
	return s.cancelErr
}

var _ PlanBoardOrchFallback = (*stubBoardFallback)(nil)

// TestCheckProgress_FallsBackToPlanBoard (C-18) verifies check_progress uses
// PlanBoard when orchestrator lookup fails.
func TestCheckProgress_FallsBackToPlanBoard(t *testing.T) {
	orch := &stubOrch{progErr: errors.New("orchestration not found")}
	boards := &stubBoardFallback{
		progress: []biz.TaskProgress{{SubTaskID: "st-1", Status: "running", Progress: 0.5}},
	}
	tool := NewCheckOrchestrationProgressTool(orch, boards, loggateway.NewNoop())
	raw, err := tool.Call(context.Background(), []byte(`{"orchestration_id":"pb_abc"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	out, ok := raw.(CheckOrchestrationProgressOutput)
	if !ok {
		t.Fatalf("got %T", raw)
	}
	if out.OrchestrationID != "pb_abc" {
		t.Fatalf("OrchestrationID = %q, want pb_abc", out.OrchestrationID)
	}
	if out.Status != "running" {
		t.Fatalf("Status = %q, want running", out.Status)
	}
	if len(out.Tasks) != 1 || out.Tasks[0].SubTaskID != "st-1" {
		t.Fatalf("Tasks = %+v", out.Tasks)
	}
}

// TestCancel_FallsBackToPlanBoard (C-18) verifies cancel uses PlanBoard when
// orchestrator.Cancel fails.
func TestCancel_FallsBackToPlanBoard(t *testing.T) {
	orch := &stubOrch{cancelErr: errors.New("orchestration not found")}
	boards := &stubBoardFallback{}
	tool := NewCancelOrchestrationTool(orch, boards, loggateway.NewNoop())
	raw, err := tool.Call(context.Background(), []byte(`{"orchestration_id":"pb_cancel"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if boards.cancelCalls != 1 {
		t.Fatalf("cancelCalls = %d, want 1", boards.cancelCalls)
	}
	out, ok := raw.(CancelOrchestrationOutput)
	if !ok {
		t.Fatalf("got %T", raw)
	}
	if out.Status != "cancelled" || out.OrchestrationID != "pb_cancel" {
		t.Fatalf("out = %+v", out)
	}
}
