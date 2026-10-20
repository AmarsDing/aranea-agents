package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// TestCascadeSkip_NilDepStep ensures a nil entry in stepsByID does not panic
// (same class as checkDownstream DAG-NIL from #95).
func TestCascadeSkip_NilDepStep(t *testing.T) {
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	board := biz.PlanBoard{
		ID:        "b-nil",
		SessionID: "sp-1",
		Steps: []biz.PlanStep{
			{ID: "root", Status: biz.PlanStepStatusFailed},
			{ID: "child", Status: biz.PlanStepStatusPending, DependsOn: []string{"root"}},
		},
	}
	r := &dagRun{pe: pe, board: board, stepsByID: map[string]*biz.PlanStep{
		"root":  &board.Steps[0],
		"child": nil, // poisoned
		"ghost": &board.Steps[1],
	}, dependents: map[string][]string{
		"root": {"child", "ghost"},
	}}
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("cascadeSkip panicked: %v", rec)
		}
	}()
	r.cascadeSkip(context.Background(), "root")
}

func TestUpdateGraphNode_NilStep(t *testing.T) {
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	r := &dagRun{pe: pe, board: biz.PlanBoard{ID: "b1", SessionID: "s1"}, graphStageID: "gs1", stepsByID: map[string]*biz.PlanStep{
		"missing": nil,
	}}
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("updateGraphNode panicked: %v", rec)
		}
	}()
	r.updateGraphNode(context.Background(), "missing", biz.GraphNodeStatusInterrupted, "")
}
