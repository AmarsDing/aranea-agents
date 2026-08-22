package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestPlanBoardV2Repo_CreateAndGet(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewPlanBoardV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	created, err := repo.CreatePlanBoard(ctx, biz.PlanBoard{
		ID: "pb-1", TaskID: "t-1", TurnID: "turn-1", SessionID: "s-1",
		Strategy: biz.PlanStrategyDAG, Status: biz.PlanStatusPlanning,
		StartedAt: now, Seq: 1, Version: 1,
		// Steps is in-memory only, intentionally not persisted
		Steps: []biz.PlanStep{{ID: "ps-1", PlanID: "pb-1", Label: "Step 1"}},
	})
	if err != nil {
		t.Fatalf("CreatePlanBoard: %v", err)
	}
	if created.ID != "pb-1" || created.Strategy != biz.PlanStrategyDAG {
		t.Fatalf("created mismatch: %+v", created)
	}

	got, err := repo.GetPlanBoard(ctx, "pb-1")
	if err != nil {
		t.Fatalf("GetPlanBoard: %v", err)
	}
	if got.TaskID != "t-1" || got.Status != biz.PlanStatusPlanning {
		t.Fatalf("plan board mismatch: %+v", got)
	}
	// Steps is in-memory only: after round-trip it should be empty (loaded via PlanStepV2Repo separately)
	if len(got.Steps) != 0 {
		t.Fatalf("Steps should not be persisted, got %+v", got.Steps)
	}
}

func TestPlanBoardV2Repo_Upsert_VersionGuard(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewPlanBoardV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	_, err := repo.UpsertPlanBoard(ctx, biz.PlanBoard{
		ID: "pb-2", TaskID: "t-1", TurnID: "turn-1", SessionID: "s-1",
		Strategy: biz.PlanStrategySequential, Status: biz.PlanStatusPlanning,
		StartedAt: now, Seq: 2, Version: 1,
	})
	if err != nil {
		t.Fatalf("UpsertPlanBoard v1: %v", err)
	}
	// Stale version (0) should not overwrite v1.
	stale, err := repo.UpsertPlanBoard(ctx, biz.PlanBoard{
		ID: "pb-2", TaskID: "t-1", TurnID: "turn-1", SessionID: "s-1",
		Strategy: biz.PlanStrategyParallel, Status: biz.PlanStatusFailed,
		StartedAt: now, Seq: 2, Version: 0,
	})
	if err != nil {
		t.Fatalf("UpsertPlanBoard stale: %v", err)
	}
	if stale.Strategy != biz.PlanStrategySequential || stale.Status != biz.PlanStatusPlanning {
		t.Fatalf("stale overwrote: %+v", stale)
	}
	// Newer version (2) should overwrite with CompletedAt.
	completedAt := now.Add(30 * time.Second)
	_, err = repo.UpsertPlanBoard(ctx, biz.PlanBoard{
		ID: "pb-2", TaskID: "t-1", TurnID: "turn-1", SessionID: "s-1",
		Strategy: biz.PlanStrategySequential, Status: biz.PlanStatusCompleted,
		StartedAt: now, CompletedAt: &completedAt,
		Seq: 2, Version: 2,
	})
	if err != nil {
		t.Fatalf("UpsertPlanBoard v2: %v", err)
	}
	got, _ := repo.GetPlanBoard(ctx, "pb-2")
	if got.Status != biz.PlanStatusCompleted || got.Version != 2 {
		t.Fatalf("newer did not overwrite: %+v", got)
	}
	if got.CompletedAt == nil || !got.CompletedAt.Equal(completedAt) {
		t.Fatalf("completedAt mismatch: got %+v, expected %v", got.CompletedAt, completedAt)
	}
}

func TestPlanBoardV2Repo_ListByTask_SeqOrder(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewPlanBoardV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	for i, seq := range []int64{3, 1, 2} {
		_, err := repo.CreatePlanBoard(ctx, biz.PlanBoard{
			ID: "ord-" + string(rune('a'+i)), TaskID: "t-x", TurnID: "turn-1",
			SessionID: "s-1", Strategy: biz.PlanStrategySequential,
			Status:    biz.PlanStatusPlanning,
			StartedAt: now, Seq: seq, Version: 1,
		})
		if err != nil {
			t.Fatalf("CreatePlanBoard[%d]: %v", i, err)
		}
	}
	boards, err := repo.ListPlanBoardsByTask(ctx, "t-x")
	if err != nil {
		t.Fatalf("ListPlanBoardsByTask: %v", err)
	}
	if len(boards) != 3 || boards[0].Seq != 1 || boards[2].Seq != 3 {
		t.Fatalf("order: expected 1,2,3 got %+v", []int64{boards[0].Seq, boards[1].Seq, boards[2].Seq})
	}
}

func TestPlanBoardV2Repo_ListByStatuses(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewPlanBoardV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	for i, st := range []biz.PlanStatus{biz.PlanStatusPlanning, biz.PlanStatusExecuting, biz.PlanStatusCompleted} {
		if _, err := repo.CreatePlanBoard(ctx, biz.PlanBoard{
			ID: "st-" + string(rune('a'+i)), TaskID: "t-st", TurnID: "turn-1",
			SessionID: "s-1", Strategy: biz.PlanStrategySequential,
			Status: st, StartedAt: now, Seq: int64(i + 1), Version: 1,
		}); err != nil {
			t.Fatalf("CreatePlanBoard[%d]: %v", i, err)
		}
	}
	got, err := repo.ListPlanBoardsByStatuses(ctx, []biz.PlanStatus{
		biz.PlanStatusPlanning, biz.PlanStatusExecuting,
	})
	if err != nil {
		t.Fatalf("ListPlanBoardsByStatuses: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 unfinished boards, got %d", len(got))
	}
}

func TestPlanBoardV2Repo_GetPlanBoard_NotFound(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewPlanBoardV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()

	_, err := repo.GetPlanBoard(ctx, "nonexistent")
	if err == nil {
		t.Fatalf("expected error for nonexistent plan board, got nil")
	}
}
