package service

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// TestPlanExecutor_CyclicDAGRejected verifies that a cyclic PlanBoard
// (c1→c2→c1, no roots) is rejected fail-closed: Subscribe returns an error,
// the board is marked Failed (NOT Completed), and no step is dispatched to
// the orchestrator.
//
// Regression guard: previously run() dispatched only root steps; a cycle has
// no roots → WaitGroup stayed 0 → run() returned nil and the board was marked
// Completed without executing anything.
func TestPlanExecutor_CyclicDAGRejected(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-cycle",
		TaskID:    "task-cycle",
		SessionID: "sess-cycle",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "c1", PlanID: "board-cycle", TaskID: "task-cycle", Label: "c1", DependsOn: []string{"c2"}, Status: biz.PlanStepStatusPending, Version: 1},
			{ID: "c2", PlanID: "board-cycle", TaskID: "task-cycle", Label: "c2", DependsOn: []string{"c1"}, Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error for cyclic DAG, got nil (silent success)")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe timed out on cyclic DAG")
	}

	// No step should have been dispatched to the orchestrator.
	orch.mu.Lock()
	calls := append([]string(nil), orch.calls...)
	orch.mu.Unlock()
	if len(calls) != 0 {
		t.Fatalf("expected no dispatch for cyclic DAG, got calls: %v", calls)
	}

	// Board must be Failed, not Completed.
	repos.mu.Lock()
	defer repos.mu.Unlock()
	if repos.board == nil {
		t.Fatal("plan board not persisted")
	}
	if repos.board.Status == biz.PlanStatusCompleted {
		t.Fatalf("cyclic board marked Completed (should be Failed); status=%s", repos.board.Status)
	}
	if repos.board.Status != biz.PlanStatusFailed {
		t.Fatalf("expected board status %s, got %s", biz.PlanStatusFailed, repos.board.Status)
	}
}

// TestPlanExecutor_DanglingDependencyRejected verifies that a step depending
// on a non-existent step ID is also rejected fail-closed (malformed DAG).
func TestPlanExecutor_DanglingDependencyRejected(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-dangling",
		TaskID:    "task-dangling",
		SessionID: "sess-dangling",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "d1", PlanID: "board-dangling", TaskID: "task-dangling", Label: "d1", DependsOn: []string{"ghost"}, Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error for dangling dependency, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe timed out on dangling dependency")
	}

	orch.mu.Lock()
	calls := append([]string(nil), orch.calls...)
	orch.mu.Unlock()
	if len(calls) != 0 {
		t.Fatalf("expected no dispatch for dangling-dep DAG, got calls: %v", calls)
	}

	repos.mu.Lock()
	defer repos.mu.Unlock()
	if repos.board == nil {
		t.Fatal("plan board not persisted")
	}
	if repos.board.Status != biz.PlanStatusFailed {
		t.Fatalf("expected board status %s, got %s", biz.PlanStatusFailed, repos.board.Status)
	}
}

// TestPlanExecutor_CancelWaitsWorkerBarrier verifies P1 (audit): when Subscribe
// ctx is cancelled mid-flight, run() waits for in-flight dispatch goroutines
// before publishing terminal state. Workers exit on ctx.Done(); the board must
// be Failed (not Completed) and Subscribe must return context.Canceled.
func TestPlanExecutor_CancelWaitsWorkerBarrier(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-cancel",
		TaskID:    "task-cancel",
		SessionID: "sess-cancel",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "hang", PlanID: "board-cancel", TaskID: "task-cancel", Label: "hang", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(ctx, board) }()

	if !orch.waitForCall("hang", 2*time.Second) {
		t.Fatal("hang step was not dispatched")
	}
	// Cancel while worker is blocked on CompletionChan.
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected context error after cancel, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe did not return after cancel (barrier hang?)")
	}

	repos.mu.Lock()
	defer repos.mu.Unlock()
	if repos.board == nil {
		t.Fatal("plan board not persisted after cancel")
	}
	if repos.board.Status == biz.PlanStatusCompleted {
		t.Fatalf("cancelled board marked Completed; status=%s", repos.board.Status)
	}
	if repos.board.Status != biz.PlanStatusFailed {
		t.Fatalf("expected Failed after cancel, got %s", repos.board.Status)
	}
}

// TestPlanExecutor_PlanningBoardReachesCompleted verifies B-05:
// Subscribe starting from Status=planning must transition through executing
// and land on completed after a successful single-step DAG. Previously
// markPlanBoardExecuting mutated a by-value copy, so terminal transition
// (Executing→Completed) was skipped and the board stayed "executing".
func TestPlanExecutor_PlanningBoardReachesCompleted(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-b05",
		TaskID:    "task-b05",
		SessionID: "sess-b05",
		Status:    biz.PlanStatusPlanning,
		Steps: []biz.PlanStep{
			{ID: "s1", PlanID: "board-b05", TaskID: "task-b05", Label: "only", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()

	if !orch.waitForCall("s1", 2*time.Second) {
		t.Fatal("s1 was not dispatched")
	}
	orch.completeStep("s1", true, "")

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Subscribe returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe timed out")
	}

	repos.mu.Lock()
	defer repos.mu.Unlock()
	if repos.board == nil {
		t.Fatal("plan board not persisted")
	}
	if repos.board.Status != biz.PlanStatusCompleted {
		t.Fatalf("B-05 regression: expected Completed after planning→executing→done, got %s", repos.board.Status)
	}
}

// TestPlanExecutor_EmptyDAGRejected verifies empty boards fail-closed instead
// of silently completing with zero work.
func TestPlanExecutor_EmptyDAGRejected(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-empty",
		TaskID:    "task-empty",
		SessionID: "sess-empty",
		Status:    biz.PlanStatusExecuting,
		Steps:     nil,
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())

	err := pe.Subscribe(context.Background(), board)
	if err == nil {
		t.Fatal("expected error for empty DAG, got nil")
	}
	repos.mu.Lock()
	defer repos.mu.Unlock()
	if repos.board != nil && repos.board.Status == biz.PlanStatusCompleted {
		t.Fatal("empty board must not be marked Completed")
	}
}
