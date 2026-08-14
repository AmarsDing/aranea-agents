package service

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

func TestPlanExecutor_StartSubscription_LeaseDedupesSameBoard(t *testing.T) {
	repos := newFakeReposForExecutor()
	seq := &fakeSeq{repos: repos}
	orch := newFakeOrchestrator().withSeq(seq)
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	bus := event.NewV2Bus()
	pe.SetEventBus(bus)
	pe.StartSubscription()
	t.Cleanup(pe.Stop)
	// Allow subscriber goroutine to register.
	time.Sleep(20 * time.Millisecond)

	board := biz.PlanBoard{
		ID:        "board-lease",
		TaskID:    "task-lease",
		SessionID: "sess-lease",
		Status:    biz.PlanStatusPlanning,
		Steps: []biz.PlanStep{
			{ID: "s1", PlanID: "board-lease", TaskID: "task-lease", Label: "step1", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	ev := biz.NewPlanBoardCreatedEvent(board)
	bus.Publish(context.Background(), ev)
	bus.Publish(context.Background(), ev)

	if !orch.waitForCall("s1", 2*time.Second) {
		t.Fatal("expected one dispatch from first event")
	}
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		orch.mu.Lock()
		n := len(orch.calls)
		orch.mu.Unlock()
		if n > 1 {
			t.Fatalf("Orchestrate calls=%d want 1 (C-20 lease)", n)
		}
		time.Sleep(5 * time.Millisecond)
	}
	orch.mu.Lock()
	n := len(orch.calls)
	orch.mu.Unlock()
	if n != 1 {
		t.Fatalf("Orchestrate calls=%d want 1", n)
	}
	orch.completeStep("s1", true, "")
}

func TestPlanExecutor_StartSubscription_EmptyShellTakesNoLease(t *testing.T) {
	repos := newFakeReposForExecutor()
	seq := &fakeSeq{repos: repos}
	orch := newFakeOrchestrator().withSeq(seq)
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())

	bus := event.NewV2Bus()
	pe.SetEventBus(bus)
	pe.StartSubscription()
	t.Cleanup(pe.Stop)
	time.Sleep(20 * time.Millisecond)

	board := biz.PlanBoard{
		ID:        "board-empty-lease",
		TaskID:    "task-empty",
		SessionID: "sess-empty",
		Status:    biz.PlanStatusPlanning,
		Steps:     nil,
	}
	bus.Publish(context.Background(), biz.NewPlanBoardCreatedEvent(board))
	time.Sleep(50 * time.Millisecond)

	// 新契约（流式壳修复）：空壳 Created 不启动、不 fail-closed、不占 lease，
	// 等待 steps 就绪的 PlanBoardUpdatedEvent。
	if _, ok := pe.running.Load(board.ID); ok {
		t.Fatal("empty shell must not hold the execution lease")
	}
	if repos.board != nil {
		t.Fatalf("expected no board upsert for empty shell, got status=%s", repos.board.Status)
	}
}

func TestPlanExecutor_Subscribe_SkipsTerminalBoard(t *testing.T) {
	repos := newFakeReposForExecutor()
	seq := &fakeSeq{repos: repos}
	orch := newFakeOrchestrator().withSeq(seq)
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())

	err := pe.Subscribe(context.Background(), biz.PlanBoard{
		ID:     "board-done",
		Status: biz.PlanStatusCompleted,
		Steps: []biz.PlanStep{
			{ID: "s1", Status: biz.PlanStepStatusCompleted},
		},
	})
	if err == nil {
		t.Fatal("expected error for terminal board")
	}
	if orch.waitForCall("s1", 50*time.Millisecond) {
		t.Fatal("must not orchestrate terminal board")
	}
}

func TestPlanExecutor_StartSubscription_StopUnsubscribes(t *testing.T) {
	repos := newFakeReposForExecutor()
	seq := &fakeSeq{repos: repos}
	orch := newFakeOrchestrator().withSeq(seq)
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	bus := event.NewV2Bus()
	pe.SetEventBus(bus)
	pe.StartSubscription()
	time.Sleep(20 * time.Millisecond)
	pe.Stop()
	time.Sleep(20 * time.Millisecond)

	board := biz.PlanBoard{
		ID:        "board-after-stop",
		TaskID:    "task-after-stop",
		SessionID: "sess-after-stop",
		Status:    biz.PlanStatusPlanning,
		Steps: []biz.PlanStep{
			{ID: "s1", PlanID: "board-after-stop", TaskID: "task-after-stop", Label: "step1", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	bus.Publish(context.Background(), biz.NewPlanBoardCreatedEvent(board))
	if orch.waitForCall("s1", 200*time.Millisecond) {
		t.Fatal("must not dispatch PlanBoard events after Stop")
	}
}

func TestPlanExecutor_StopCancelsInFlightDag(t *testing.T) {
	repos := newFakeReposForExecutor()
	seq := &fakeSeq{repos: repos}
	orch := newFakeOrchestrator().withSeq(seq)
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	bus := event.NewV2Bus()
	pe.SetEventBus(bus)
	pe.StartSubscription()
	t.Cleanup(pe.Stop)
	time.Sleep(20 * time.Millisecond)

	board := biz.PlanBoard{
		ID:        "board-inflight",
		TaskID:    "task-inflight",
		SessionID: "sess-inflight",
		Status:    biz.PlanStatusPlanning,
		Steps: []biz.PlanStep{
			{ID: "s1", PlanID: "board-inflight", TaskID: "task-inflight", Label: "step1", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	bus.Publish(context.Background(), biz.NewPlanBoardCreatedEvent(board))
	if !orch.waitForCall("s1", 2*time.Second) {
		t.Fatal("expected in-flight DAG to start")
	}
	if _, ok := pe.running.Load(board.ID); !ok {
		t.Fatal("expected execution lease while DAG waits on completion")
	}
	pe.Stop()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := pe.running.Load(board.ID); !ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("Stop must cancel in-flight DAG lease so Subscribe exits")
}

func TestChatService_CloseStopsPlanExecutor(t *testing.T) {
	repos := newFakeReposForExecutor()
	seq := &fakeSeq{repos: repos}
	orch := newFakeOrchestrator().withSeq(seq)
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	bus := event.NewV2Bus()
	pe.SetEventBus(bus)
	pe.StartSubscription()
	svc := &ChatService{planExec: pe, lg: loggateway.NewNoop()}
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	board := biz.PlanBoard{
		ID:        "board-chat-close",
		TaskID:    "task-chat-close",
		SessionID: "sess-chat-close",
		Status:    biz.PlanStatusPlanning,
		Steps: []biz.PlanStep{
			{ID: "s1", PlanID: "board-chat-close", TaskID: "task-chat-close", Label: "step1", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	bus.Publish(context.Background(), biz.NewPlanBoardCreatedEvent(board))
	if orch.waitForCall("s1", 200*time.Millisecond) {
		t.Fatal("ChatService.Close must stop PlanExecutor subscription")
	}
}
