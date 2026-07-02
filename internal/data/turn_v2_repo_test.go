package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestTurnV2Repo_CreateAndGet(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTurnV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	created, err := repo.CreateTurn(ctx, biz.Turn{
		ID: "tn-1", TaskID: "t-1", SessionID: "s-1", SpiritSessionID: "s-1",
		AgentKey: "agent-1", Seq: 1, Status: biz.TurnStatusRunning,
		StartedAt: now, Version: 1,
	})
	if err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	if created.ID != "tn-1" || created.AgentKey != "agent-1" {
		t.Fatalf("created mismatch: %+v", created)
	}

	got, err := repo.GetTurn(ctx, "tn-1")
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if got.AgentKey != "agent-1" || got.Seq != 1 || got.Version != 1 {
		t.Fatalf("turn mismatch: %+v", got)
	}
}

func TestTurnV2Repo_Upsert_VersionGuard(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTurnV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	_, err := repo.UpsertTurn(ctx, biz.Turn{
		ID: "tn-2", TaskID: "t-1", SessionID: "s-1", SpiritSessionID: "s-1",
		AgentKey: "a", Seq: 2, Status: biz.TurnStatusRunning,
		StartedAt: now, Version: 1,
	})
	if err != nil {
		t.Fatalf("UpsertTurn v1: %v", err)
	}
	// Stale version (0) should not overwrite v1.
	stale, err := repo.UpsertTurn(ctx, biz.Turn{
		ID: "tn-2", TaskID: "t-1", SessionID: "s-1", SpiritSessionID: "s-1",
		AgentKey: "stale", Seq: 2, Status: biz.TurnStatusFailed,
		StartedAt: now, Version: 0,
	})
	if err != nil {
		t.Fatalf("UpsertTurn stale: %v", err)
	}
	if stale.AgentKey != "a" || stale.Status != biz.TurnStatusRunning {
		t.Fatalf("stale overwrote: %+v", stale)
	}
	// Newer version (2) should overwrite.
	_, err = repo.UpsertTurn(ctx, biz.Turn{
		ID: "tn-2", TaskID: "t-1", SessionID: "s-1", SpiritSessionID: "s-1",
		AgentKey: "b", Seq: 2, Status: biz.TurnStatusCompleted,
		StartedAt: now, Version: 2,
	})
	if err != nil {
		t.Fatalf("UpsertTurn v2: %v", err)
	}
	got, _ := repo.GetTurn(ctx, "tn-2")
	if got.AgentKey != "b" || got.Status != biz.TurnStatusCompleted || got.Version != 2 {
		t.Fatalf("newer did not overwrite: %+v", got)
	}
}

func TestTurnV2Repo_ListByTask_SeqOrder(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTurnV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	for i, seq := range []int64{3, 1, 2} {
		_, err := repo.CreateTurn(ctx, biz.Turn{
			ID: "ord-" + string(rune('a'+i)), TaskID: "t-x", SessionID: "s-1",
			SpiritSessionID: "s-1", Seq: seq, Status: biz.TurnStatusRunning,
			StartedAt: now, Version: 1,
		})
		if err != nil {
			t.Fatalf("CreateTurn[%d]: %v", i, err)
		}
	}
	turns, err := repo.ListTurnsByTask(ctx, "t-x")
	if err != nil {
		t.Fatalf("ListTurnsByTask: %v", err)
	}
	if len(turns) != 3 || turns[0].Seq != 1 || turns[2].Seq != 3 {
		t.Fatalf("order: expected 1,2,3 got %+v", []int64{turns[0].Seq, turns[1].Seq, turns[2].Seq})
	}
}

func TestTurnV2Repo_GetTurn_NotFound(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTurnV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()

	_, err := repo.GetTurn(ctx, "nonexistent")
	if err == nil {
		t.Fatalf("expected error for nonexistent turn, got nil")
	}
}
