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

func TestHoldPlaybookConfirmUsesNotedDecision(t *testing.T) {
	t.Parallel()
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	r := newDagRun(pe, biz.PlanBoard{SessionID: "sp-1", ID: "pb-1"})
	step := &biz.PlanStep{ID: "st-confirm", Label: "发布", ConfirmBefore: true, Status: biz.PlanStepStatusPending}
	pe.NotePlaybookConfirmDecision("sp-1", "st-confirm", true)
	approved, held := r.holdPlaybookConfirm(context.Background(), step)
	if !held || !approved {
		t.Fatalf("noted decision must resume without waiter: held=%v approved=%v", held, approved)
	}
}

func TestHoldPlaybookConfirmAbortOnCancel(t *testing.T) {
	t.Parallel()
	seq := &fakeSeq{}
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), seq, loggateway.NewNoop())
	r := newDagRun(pe, biz.PlanBoard{SessionID: "sp-1", ID: "pb-1"})
	step := &biz.PlanStep{ID: "st-confirm", Label: "发布", ConfirmBefore: true, Status: biz.PlanStepStatusPending}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_, _ = r.holdPlaybookConfirm(ctx, step)
		close(done)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for !pe.HasPlaybookStageConfirm("sp-1", "st-confirm") {
		if time.Now().After(deadline) {
			t.Fatal("waiter not registered")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	r.abortPlaybookConfirm(context.Background(), step)
	if pe.HasPlaybookStageConfirm("sp-1", "st-confirm") {
		t.Fatal("waiter must be cleared on abort")
	}
}

func TestResolvePlaybookStageConfirmUnknownIsFalse(t *testing.T) {
	t.Parallel()
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	if pe.HasPlaybookStageConfirm("sp-1", "missing") || pe.ResolvePlaybookStageConfirm("sp-1", "missing", true) {
		t.Fatal("unknown waiter must not resolve")
	}
}

func seedExecutingConfirmBoard(t *testing.T) (*fakeReposForExecutor, biz.PlanBoard, biz.PlanStep) {
	t.Helper()
	repos := newFakeReposForExecutor()
	board := biz.PlanBoard{
		ID:        "pb-exec",
		TaskID:    "task-1",
		SessionID: "sp-1",
		Status:    biz.PlanStatusExecuting,
		Version:   2,
	}
	if _, err := repos.UpsertPlanBoard(context.Background(), board); err != nil {
		t.Fatalf("seed board: %v", err)
	}
	step := biz.PlanStep{
		ID:            "st-confirm",
		PlanID:        board.ID,
		TaskID:        board.TaskID,
		Label:         "发布",
		ConfirmBefore: true,
		Status:        biz.PlanStepStatusPending,
		Version:       1,
	}
	if _, err := repos.UpsertPlanStep(context.Background(), step); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	return repos, board, step
}

func TestRecoverUnfinishedBoards_ResumesExecutingConfirmWaiter(t *testing.T) {
	t.Parallel()
	repos, _, _ := seedExecutingConfirmBoard(t)
	orch := newFakeOrchestrator()
	pe := NewPlanExecutor(repos, orch, &fakeSeq{}, loggateway.NewNoop())
	pe.RecoverUnfinishedBoards(context.Background())

	deadline := time.Now().Add(2 * time.Second)
	for !pe.HasActiveRunForSession("sp-1") {
		if time.Now().After(deadline) {
			t.Fatal("executing board was not re-subscribed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	for !pe.HasPlaybookStageConfirm("sp-1", "st-confirm") {
		if time.Now().After(deadline) {
			t.Fatal("confirm_before waiter was not re-registered")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !pe.ResolvePlaybookStageConfirm("sp-1", "st-confirm", true) {
		t.Fatal("resolve after recover failed")
	}
	if !orch.waitForCall("st-confirm", 2*time.Second) {
		t.Fatal("recovered confirm step was not dispatched")
	}
	orch.completeStep("st-confirm", true, "")
	deadline = time.Now().Add(2 * time.Second)
	for pe.HasActiveRunForSession("sp-1") {
		if time.Now().After(deadline) {
			t.Fatal("recovered DAG did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRecoverUnfinishedBoards_UsesNotedDecision(t *testing.T) {
	t.Parallel()
	repos, _, _ := seedExecutingConfirmBoard(t)
	orch := newFakeOrchestrator()
	pe := NewPlanExecutor(repos, orch, &fakeSeq{}, loggateway.NewNoop())
	pe.NotePlaybookConfirmDecision("sp-1", "st-confirm", true)
	pe.RecoverUnfinishedBoards(context.Background())
	if !orch.waitForCall("st-confirm", 2*time.Second) {
		t.Fatal("noted decision must resume recovered confirm_before without a live waiter")
	}
	orch.completeStep("st-confirm", true, "")
}

func TestRecoverUnfinishedBoards_DispatchesReadyPendingAfterCompletedRoot(t *testing.T) {
	t.Parallel()
	repos := newFakeReposForExecutor()
	board := biz.PlanBoard{
		ID:        "pb-exec",
		TaskID:    "task-1",
		SessionID: "sp-1",
		Status:    biz.PlanStatusExecuting,
		Version:   2,
	}
	if _, err := repos.UpsertPlanBoard(context.Background(), board); err != nil {
		t.Fatalf("seed board: %v", err)
	}
	if _, err := repos.UpsertPlanStep(context.Background(), biz.PlanStep{
		ID: "s1", PlanID: board.ID, TaskID: board.TaskID, Label: "done",
		Status: biz.PlanStepStatusCompleted, Version: 2,
	}); err != nil {
		t.Fatalf("seed s1: %v", err)
	}
	if _, err := repos.UpsertPlanStep(context.Background(), biz.PlanStep{
		ID: "s2", PlanID: board.ID, TaskID: board.TaskID, Label: "next",
		DependsOn: []string{"s1"}, Status: biz.PlanStepStatusPending, Version: 1,
	}); err != nil {
		t.Fatalf("seed s2: %v", err)
	}
	orch := newFakeOrchestrator()
	pe := NewPlanExecutor(repos, orch, &fakeSeq{}, loggateway.NewNoop())
	pe.RecoverUnfinishedBoards(context.Background())
	if orch.waitForCall("s1", 80*time.Millisecond) {
		t.Fatal("completed root must not be re-dispatched")
	}
	if !orch.waitForCall("s2", 2*time.Second) {
		t.Fatal("pending child of completed root must resume after recover")
	}
	orch.completeStep("s2", true, "")
}
