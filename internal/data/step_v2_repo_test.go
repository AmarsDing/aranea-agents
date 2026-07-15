package data

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestStepV2Repo_CreateAndGet(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewStepV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	_, err := repo.CreateStep(ctx, biz.Step{
		ID: "st-1", TurnID: "turn-1", TaskID: "t-1",
		SessionID: "s-1", SpiritSessionID: "s-1",
		Kind: biz.StepKindThinking, Seq: 1,
		Content: "thinking...", Status: biz.StepStatusRunning,
		StartedAt: now, Version: 1,
	})
	if err != nil {
		t.Fatalf("CreateStep: %v", err)
	}
	got, err := repo.GetStep(ctx, "st-1")
	if err != nil {
		t.Fatalf("GetStep: %v", err)
	}
	if got.Kind != biz.StepKindThinking || got.Content != "thinking..." {
		t.Fatalf("step mismatch: %+v", got)
	}
}

func TestStepV2Repo_Upsert_JSONArgs(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewStepV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	args, _ := json.Marshal(map[string]any{"command": "ls -la"})
	_, err := repo.UpsertStep(ctx, biz.Step{
		ID: "st-2", TurnID: "turn-1", TaskID: "t-1",
		SessionID: "s-1", SpiritSessionID: "s-1",
		Kind: biz.StepKindAction, Seq: 2,
		ToolName: "shell", ToolArgs: args,
		Status: biz.StepStatusToolRunning, StartedAt: now, Version: 1,
	})
	if err != nil {
		t.Fatalf("UpsertStep: %v", err)
	}
	got, _ := repo.GetStep(ctx, "st-2")
	if got.ToolName != "shell" {
		t.Fatalf("tool name: expected shell, got %s", got.ToolName)
	}
	var argsGot map[string]any
	if err := json.Unmarshal(got.ToolArgs, &argsGot); err != nil {
		t.Fatalf("unmarshal tool args: %v", err)
	}
	if argsGot["command"] != "ls -la" {
		t.Fatalf("tool args mismatch: %v", argsGot)
	}
}

// TestStepV2Repo_Upsert_VersionGuard verifies production CAS: stale Version
// (incoming < stored) is a no-op returning the stored row (same as Task/Turn).
func TestStepV2Repo_Upsert_VersionGuard(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewStepV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	_, err := repo.UpsertStep(ctx, biz.Step{
		ID: "st-cas", TurnID: "turn-1", TaskID: "t-1",
		SessionID: "s-1", SpiritSessionID: "s-1",
		Kind: biz.StepKindThinking, Seq: 1, Content: "v1",
		Status: biz.StepStatusRunning, StartedAt: now, Version: 1,
	})
	if err != nil {
		t.Fatalf("UpsertStep v1: %v", err)
	}
	stale, err := repo.UpsertStep(ctx, biz.Step{
		ID: "st-cas", TurnID: "turn-1", TaskID: "t-1",
		SessionID: "s-1", SpiritSessionID: "s-1",
		Kind: biz.StepKindThinking, Seq: 1, Content: "stale",
		Status: biz.StepStatusPending, StartedAt: now, Version: 0,
	})
	if err != nil {
		t.Fatalf("UpsertStep stale: %v", err)
	}
	if stale.Content != "v1" || stale.Status != biz.StepStatusRunning {
		t.Fatalf("stale version overwrote: %+v", stale)
	}
	_, err = repo.UpsertStep(ctx, biz.Step{
		ID: "st-cas", TurnID: "turn-1", TaskID: "t-1",
		SessionID: "s-1", SpiritSessionID: "s-1",
		Kind: biz.StepKindThinking, Seq: 1, Content: "v2",
		Status: biz.StepStatusCompleted, StartedAt: now, Version: 2,
	})
	if err != nil {
		t.Fatalf("UpsertStep v2: %v", err)
	}
	got, _ := repo.GetStep(ctx, "st-cas")
	if got.Content != "v2" || got.Status != biz.StepStatusCompleted || got.Version != 2 {
		t.Fatalf("newer version not applied: %+v", got)
	}
}

func TestStepV2Repo_ListByTurn_SeqOrder(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewStepV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	for i, seq := range []int64{3, 1, 2} {
		_, err := repo.CreateStep(ctx, biz.Step{
			ID: "ord-" + string(rune('a'+i)), TurnID: "turn-x", TaskID: "t-1",
			SessionID: "s-1", SpiritSessionID: "s-1",
			Kind: biz.StepKindThinking, Seq: seq,
			Status: biz.StepStatusCompleted, StartedAt: now, Version: 1,
		})
		if err != nil {
			t.Fatalf("CreateStep[%d]: %v", i, err)
		}
	}
	steps, err := repo.ListStepsByTurn(ctx, "turn-x")
	if err != nil {
		t.Fatalf("ListStepsByTurn: %v", err)
	}
	if len(steps) != 3 || steps[0].Seq != 1 || steps[2].Seq != 3 {
		t.Fatalf("order: expected 1,2,3 got %+v", []int64{steps[0].Seq, steps[1].Seq, steps[2].Seq})
	}
}

func TestStepV2Repo_GetStep_NotFound(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewStepV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()

	_, err := repo.GetStep(ctx, "nonexistent")
	if err == nil {
		t.Fatalf("expected error for nonexistent step, got nil")
	}
}
