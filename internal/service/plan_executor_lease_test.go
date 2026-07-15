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

func TestPlanExecutor_StartSubscription_EmptyBoardAlsoTakesLease(t *testing.T) {
	repos := newFakeReposForExecutor()
	seq := &fakeSeq{repos: repos}
	orch := newFakeOrchestrator().withSeq(seq)
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())

	bus := event.NewV2Bus()
	pe.SetEventBus(bus)
	pe.StartSubscription()
	time.Sleep(20 * time.Millisecond)

	board := biz.PlanBoard{
		ID:        "board-empty-lease",
		TaskID:    "task-empty",
		SessionID: "sess-empty",
		Status:    biz.PlanStatusPlanning,
		Steps:     nil,
	}
	// Hold the lease manually to prove the empty-path also uses LoadOrStore.
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	pe.running.Store(board.ID, &boardRunLease{cancel: cancel})

	bus.Publish(context.Background(), biz.NewPlanBoardCreatedEvent(board))
	time.Sleep(50 * time.Millisecond)

	// With lease held, empty-board event must not start another Subscribe
	// (no PlanBoard upsert from fail-closed path).
	if repos.board != nil {
		t.Fatalf("expected no board upsert while lease held, got status=%s", repos.board.Status)
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
