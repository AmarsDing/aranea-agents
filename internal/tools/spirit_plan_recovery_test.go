package tools

import (
	"testing"

	"aranea-agents/internal/biz"
)

type recoveringOrch struct {
	stubOrch
	plan  *biz.TaskPlan
	alloc *biz.AllocationPlan
}

func (r *recoveringOrch) ConsumeRecoveredPlan(spiritSessionID, userMessage string) (*biz.TaskPlan, *biz.AllocationPlan, bool) {
	if r.plan == nil || r.plan.SpiritSessionID != spiritSessionID {
		return nil, nil, false
	}
	if userMessage != "" && r.plan.UserMessage != "" && r.plan.UserMessage != userMessage {
		return nil, nil, false
	}
	return r.plan, r.alloc, true
}

func TestConsumeRecoveredPlan_UsesConsumer(t *testing.T) {
	plan := &biz.TaskPlan{ID: "tp-1", SpiritSessionID: "sess-1", UserMessage: "hello"}
	alloc := &biz.AllocationPlan{ID: "ap-1", TaskPlanID: plan.ID}
	orch := &recoveringOrch{plan: plan, alloc: alloc}

	gotPlan, gotAlloc, ok := consumeRecoveredPlan(orch, "sess-1", "hello")
	if !ok || gotPlan == nil || gotPlan.ID != "tp-1" {
		t.Fatalf("want recovered plan, ok=%v plan=%+v", ok, gotPlan)
	}
	if gotAlloc == nil || gotAlloc.ID != "ap-1" {
		t.Fatalf("want recovered alloc, got %+v", gotAlloc)
	}
}

func TestConsumeRecoveredPlan_StubOrchNoConsumer(t *testing.T) {
	if _, _, ok := consumeRecoveredPlan(&stubOrch{}, "sess-1", "hello"); ok {
		t.Fatal("stub orchestrator must not report a recovered plan")
	}
	if _, _, ok := consumeRecoveredPlan(nil, "sess-1", "hello"); ok {
		t.Fatal("nil orchestrator must not report a recovered plan")
	}
}
