package service

import (
	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"context"
	"testing"
)

func TestRunClarificationGate_UnderspecifiedDoesNotAutoResolve(t *testing.T) {
	taskWriter := &stubTaskV2Writer{}
	stepWriter := &stubStepV2Writer{}
	seq := &stubEventPublisher{}
	stateMgr := &stubSessionStateTransitor{}
	orch := newClarificationTestOrch(taskWriter, stepWriter, seq, stateMgr)
	ag := biz.Agent{AgentKey: "test-agent", Settings: &biz.AgentRuntimeSettings{ClarificationEnabled: true}}
	art := &intent.Artifact{
		RiskFlags: []string{intent.RiskFlagNeedsClarification},
		Clarifications: []intent.ClarificationQuestion{
			{Question: "目标平台？", Mode: "single", Options: []string{"Web", "iOS"}, Recommended: []string{"Web"}},
		},
	}
	decision, err := orch.runClarificationGate(context.Background(), "sess-1", art, ag,
		biz.TurnInput{SessionID: "sess-1", Content: "帮我做个应用"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.AutoResolved {
		t.Fatal("underspecified request must hang, not auto-draft")
	}
	if !decision.Triggered {
		t.Fatal("underspecified request must trigger clarification")
	}
}

func TestRunClarificationGate_TriggeredWhenQuestionLacksRecommended(t *testing.T) {
	taskWriter := &stubTaskV2Writer{}
	stepWriter := &stubStepV2Writer{}
	seq := &stubEventPublisher{}
	stateMgr := &stubSessionStateTransitor{}
	orch := newClarificationTestOrch(taskWriter, stepWriter, seq, stateMgr)

	ag := biz.Agent{
		AgentKey: "test-agent",
		Settings: &biz.AgentRuntimeSettings{
			ClarificationEnabled: true,
		},
	}
	art := &intent.Artifact{
		RiskFlags: []string{intent.RiskFlagNeedsClarification},
		Clarifications: []intent.ClarificationQuestion{
			{Question: "目标平台？", Mode: "single", Options: []string{"Web", "iOS"}, Recommended: []string{"Web"}},
			{Question: "配色偏好？", Mode: "single", Options: []string{"深色", "浅色"}},
		},
	}

	decision, err := orch.runClarificationGate(context.Background(), "sess-1", art, ag,
		biz.TurnInput{SessionID: "sess-1", Content: "帮我做个应用"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decision.Triggered {
		t.Fatal("expected Triggered=true when any question lacks a recommendation")
	}
	if decision.AutoResolved {
		t.Error("expected AutoResolved=false")
	}
	if len(stepWriter.created) != 1 {
		t.Fatalf("expected 1 step created, got %d", len(stepWriter.created))
	}
	if stepWriter.created[0].Status != biz.StepStatusAwaitingInput {
		t.Errorf("step.Status = %q, want %q", stepWriter.created[0].Status, biz.StepStatusAwaitingInput)
	}
	if len(stateMgr.statuses) != 1 || stateMgr.statuses[0] != sessstatus.SessionStatusAwaitingConfirmation {
		t.Errorf("expected session awaiting_confirmation, got %v", stateMgr.statuses)
	}
}
