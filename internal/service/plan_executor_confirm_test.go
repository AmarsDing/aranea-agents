package service

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestHoldPlaybookConfirmApproveDoesNotSkip(t *testing.T) {
	t.Parallel()
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	r := newDagRun(pe, biz.PlanBoard{SessionID: "sp-1", ID: "pb-1"})
	step := &biz.PlanStep{ID: "st-confirm", Label: "发布", ConfirmBefore: true, Status: biz.PlanStepStatusPending}

	type result struct {
		approved bool
		held     bool
	}
	done := make(chan result, 1)
	go func() {
		approved, held := r.holdPlaybookConfirm(context.Background(), step)
		done <- result{approved: approved, held: held}
	}()
	deadline := time.Now().Add(2 * time.Second)
	for !pe.HasPlaybookStageConfirm("sp-1", "st-confirm") {
		if time.Now().After(deadline) {
			t.Fatal("waiter not registered")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !pe.ResolvePlaybookStageConfirm("sp-1", "st-confirm", true) {
		t.Fatal("resolve failed")
	}
	select {
	case got := <-done:
		if !got.held || !got.approved {
			t.Fatalf("held=%v approved=%v", got.held, got.approved)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("hold did not unblock")
	}
}

func TestHoldPlaybookConfirmDefaultHandoffHasNoCard(t *testing.T) {
	t.Parallel()
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	r := newDagRun(pe, biz.PlanBoard{SessionID: "sp-1", ID: "pb-1"})
	step := &biz.PlanStep{ID: "st-plain", Label: "设计", Status: biz.PlanStepStatusPending}
	approved, held := r.holdPlaybookConfirm(context.Background(), step)
	if !approved || held {
		t.Fatalf("default handoff must not wait: approved=%v held=%v", approved, held)
	}
}

func TestHoldPlaybookConfirmPublishesConfirmCard(t *testing.T) {
	t.Parallel()
	seq := &fakeSeq{}
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), seq, loggateway.NewNoop())
	r := newDagRun(pe, biz.PlanBoard{SessionID: "sp-1", ID: "pb-1", TaskID: "task-1", TurnID: "turn-1"})
	step := &biz.PlanStep{ID: "st-confirm", Label: "发布", ConfirmBefore: true, Status: biz.PlanStepStatusPending}

	done := make(chan struct{})
	go func() {
		_, _ = r.holdPlaybookConfirm(context.Background(), step)
		close(done)
	}()
	deadline := time.Now().Add(2 * time.Second)
	actID := biz.PlaybookConfirmActivityID("sp-1", "st-confirm")
	for !pe.HasPlaybookStageConfirm("sp-1", actID) {
		if time.Now().After(deadline) {
			t.Fatal("waiter not registered under card id")
		}
		time.Sleep(10 * time.Millisecond)
	}
	found := false
	for _, ev := range seq.snapshot() {
		created, ok := ev.(*biz.StepCreatedEvent)
		if !ok {
			continue
		}
		if created.Step.Kind == biz.StepKindConfirm && created.Step.Status == biz.StepStatusToolBlocked && created.Step.ID == actID {
			found = true
			if created.Step.ToolName != "playbook_confirm_before" {
				t.Fatalf("tool=%q", created.Step.ToolName)
			}
		}
	}
	if !found {
		t.Fatal("confirm card step not published")
	}
	if !pe.ResolvePlaybookStageConfirm("sp-1", actID, true) {
		t.Fatal("resolve by card id failed")
	}
	<-done
	updated := false
	for _, ev := range seq.snapshot() {
		u, ok := ev.(*biz.StepUpdatedEvent)
		if !ok {
			continue
		}
		if u.Step.ID == actID && u.Step.Status == biz.StepStatusCompleted {
			updated = true
		}
	}
	if !updated {
		t.Fatal("confirm card was not closed")
	}
}

func TestResolvePlaybookStageConfirmUnknownIsFalse(t *testing.T) {
	t.Parallel()
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	if pe.HasPlaybookStageConfirm("sp-1", "missing") || pe.ResolvePlaybookStageConfirm("sp-1", "missing", true) {
		t.Fatal("unknown waiter must not resolve")
	}
}
