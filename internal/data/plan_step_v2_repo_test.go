package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestPlanStepV2Repo_CreateAndGet(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewPlanStepV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	created, err := repo.CreatePlanStep(ctx, biz.PlanStep{
		ID: "ps-1", PlanID: "pb-1", TaskID: "t-1",
		Label: "Step One", Description: "Do something",
		DependsOn: []string{"ps-0"}, MappedTeamStageID: "ts-1",
		Status: biz.PlanStepStatusPending, AutoSynthesis: false,
		StartedAt: now, Seq: 1, Version: 1,
	})
	if err != nil {
		t.Fatalf("CreatePlanStep: %v", err)
	}
	if created.ID != "ps-1" || created.Label != "Step One" {
		t.Fatalf("created mismatch: %+v", created)
	}

	got, err := repo.GetPlanStep(ctx, "ps-1")
	if err != nil {
		t.Fatalf("GetPlanStep: %v", err)
	}
	if got.Label != "Step One" || got.MappedTeamStageID != "ts-1" {
		t.Fatalf("plan step mismatch: %+v", got)
	}
	if len(got.DependsOn) != 1 || got.DependsOn[0] != "ps-0" {
		t.Fatalf("depends_on mismatch: %+v", got.DependsOn)
	}
	if got.Result != nil || got.Error != nil {
		t.Fatalf("Result/Error should be nil initially: %+v %+v", got.Result, got.Error)
	}
}

func TestPlanStepV2Repo_Upsert_VersionGuard_WithResultError(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewPlanStepV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	_, err := repo.UpsertPlanStep(ctx, biz.PlanStep{
		ID: "ps-2", PlanID: "pb-1", TaskID: "t-1",
		Label: "Step Two", Status: biz.PlanStepStatusRunning,
		StartedAt: now, Seq: 2, Version: 1,
	})
	if err != nil {
		t.Fatalf("UpsertPlanStep v1: %v", err)
	}
	// Stale version (0) should not overwrite v1.
	stale, err := repo.UpsertPlanStep(ctx, biz.PlanStep{
		ID: "ps-2", PlanID: "pb-1", TaskID: "t-1",
		Label: "stale", Status: biz.PlanStepStatusFailed,
		StartedAt: now, Seq: 2, Version: 0,
	})
	if err != nil {
		t.Fatalf("UpsertPlanStep stale: %v", err)
	}
	if stale.Label != "Step Two" || stale.Status != biz.PlanStepStatusRunning {
		t.Fatalf("stale overwrote: %+v", stale)
	}
	// Newer version (2) should overwrite with Result and CompletedAt.
	completedAt := now.Add(15 * time.Second)
	result := &biz.StepResult{
		Output: "done",
		MemberReports: []biz.MemberReport{
			{AgentKey: "agent-1", Output: "member done", TokensUsed: biz.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150}, DurationMs: 5000},
		},
		TokensUsed: biz.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
		DurationMs: 5000,
	}
	_, err = repo.UpsertPlanStep(ctx, biz.PlanStep{
		ID: "ps-2", PlanID: "pb-1", TaskID: "t-1",
		Label: "Step Two", Status: biz.PlanStepStatusCompleted,
		StartedAt: now, CompletedAt: &completedAt,
		Seq: 2, Version: 2, Result: result,
	})
	if err != nil {
		t.Fatalf("UpsertPlanStep v2: %v", err)
	}
	got, _ := repo.GetPlanStep(ctx, "ps-2")
	if got.Status != biz.PlanStepStatusCompleted || got.Version != 2 {
		t.Fatalf("newer did not overwrite: %+v", got)
	}
	if got.CompletedAt == nil || !got.CompletedAt.Equal(completedAt) {
		t.Fatalf("completedAt mismatch: got %+v, expected %v", got.CompletedAt, completedAt)
	}
	if got.Result == nil || got.Result.Output != "done" {
		t.Fatalf("result mismatch: %+v", got.Result)
	}
	if got.Result.DurationMs != 5000 {
		t.Fatalf("result durationMs: expected 5000, got %d", got.Result.DurationMs)
	}
	if len(got.Result.MemberReports) != 1 || got.Result.MemberReports[0].AgentKey != "agent-1" {
		t.Fatalf("member reports mismatch: %+v", got.Result.MemberReports)
	}
	if got.Result.TokensUsed.TotalTokens != 150 {
		t.Fatalf("tokensUsed: expected 150, got %d", got.Result.TokensUsed.TotalTokens)
	}
	// Newer version (3) should overwrite with Error.
	stepErr := &biz.StepError{
		Code: "team_failed", Message: "member crashed", Retryable: true,
		FailedMember: &biz.MemberReport{AgentKey: "agent-1", Error: "panic"},
	}
	_, err = repo.UpsertPlanStep(ctx, biz.PlanStep{
		ID: "ps-2", PlanID: "pb-1", TaskID: "t-1",
		Label: "Step Two", Status: biz.PlanStepStatusFailed,
		StartedAt: now, CompletedAt: &completedAt,
		Seq: 2, Version: 3, Error: stepErr,
	})
	if err != nil {
		t.Fatalf("UpsertPlanStep v3: %v", err)
	}
	got, _ = repo.GetPlanStep(ctx, "ps-2")
	if got.Error == nil || got.Error.Code != "team_failed" || got.Error.Message != "member crashed" {
		t.Fatalf("error mismatch: %+v", got.Error)
	}
	if !got.Error.Retryable {
		t.Fatalf("error retryable: expected true, got false")
	}
	if got.Error.FailedMember == nil || got.Error.FailedMember.AgentKey != "agent-1" {
		t.Fatalf("failedMember mismatch: %+v", got.Error.FailedMember)
	}
}

func TestPlanStepV2Repo_ListByPlan_SeqOrder(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewPlanStepV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	for i, seq := range []int64{3, 1, 2} {
		_, err := repo.CreatePlanStep(ctx, biz.PlanStep{
			ID: "ord-" + string(rune('a'+i)), PlanID: "pb-x", TaskID: "t-1",
			Label: "step", Status: biz.PlanStepStatusPending,
			StartedAt: now, Seq: seq, Version: 1,
		})
		if err != nil {
			t.Fatalf("CreatePlanStep[%d]: %v", i, err)
		}
	}
	steps, err := repo.ListPlanStepsByPlan(ctx, "pb-x")
	if err != nil {
		t.Fatalf("ListPlanStepsByPlan: %v", err)
	}
	if len(steps) != 3 || steps[0].Seq != 1 || steps[2].Seq != 3 {
		t.Fatalf("order: expected 1,2,3 got %+v", []int64{steps[0].Seq, steps[1].Seq, steps[2].Seq})
	}
}

func TestPlanStepV2Repo_ListByTask(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewPlanStepV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	for i := 0; i < 3; i++ {
		_, err := repo.CreatePlanStep(ctx, biz.PlanStep{
			ID: "t-" + string(rune('a'+i)), PlanID: "pb-1", TaskID: "task-y",
			Label: "step", Status: biz.PlanStepStatusPending,
			StartedAt: now, Seq: int64(i + 1), Version: 1,
		})
		if err != nil {
			t.Fatalf("CreatePlanStep[%d]: %v", i, err)
		}
	}
	steps, err := repo.ListPlanStepsByTask(ctx, "task-y")
	if err != nil {
		t.Fatalf("ListPlanStepsByTask: %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}
}

func TestPlanStepV2Repo_GetPlanStep_NotFound(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewPlanStepV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()

	_, err := repo.GetPlanStep(ctx, "nonexistent")
	if err == nil {
		t.Fatalf("expected error for nonexistent plan step, got nil")
	}
}
