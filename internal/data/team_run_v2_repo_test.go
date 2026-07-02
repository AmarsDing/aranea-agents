package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestTeamRunV2Repo_CreateAndGet(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTeamRunV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	created, err := repo.CreateTeamRun(ctx, biz.TeamRun{
		ID: "tr-1", TeamStageID: "ts-1", TaskID: "t-1",
		SessionID: "s-1", SpiritSessionID: "s-1",
		DagNodeID: "node-1", DependsOn: []string{"tr-0"},
		Status: biz.TeamRunV2StatusRunning,
		StartedAt: now, Seq: 1, Version: 1,
	})
	if err != nil {
		t.Fatalf("CreateTeamRun: %v", err)
	}
	if created.ID != "tr-1" || created.TeamStageID != "ts-1" {
		t.Fatalf("created mismatch: %+v", created)
	}

	got, err := repo.GetTeamRun(ctx, "tr-1")
	if err != nil {
		t.Fatalf("GetTeamRun: %v", err)
	}
	if got.TeamStageID != "ts-1" || got.Status != biz.TeamRunV2StatusRunning {
		t.Fatalf("team run mismatch: %+v", got)
	}
	if len(got.DependsOn) != 1 || got.DependsOn[0] != "tr-0" {
		t.Fatalf("depends_on mismatch: %+v", got.DependsOn)
	}
}

func TestTeamRunV2Repo_Upsert_VersionGuard_WithCompletedAt(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTeamRunV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	_, err := repo.UpsertTeamRun(ctx, biz.TeamRun{
		ID: "tr-2", TeamStageID: "ts-1", TaskID: "t-1",
		SessionID: "s-1", SpiritSessionID: "s-1",
		Status: biz.TeamRunV2StatusRunning,
		StartedAt: now, Seq: 2, Version: 1,
	})
	if err != nil {
		t.Fatalf("UpsertTeamRun v1: %v", err)
	}
	// Stale version (0) should not overwrite v1.
	stale, err := repo.UpsertTeamRun(ctx, biz.TeamRun{
		ID: "tr-2", TeamStageID: "ts-1", TaskID: "t-1",
		SessionID: "s-1", SpiritSessionID: "s-1",
		Status: biz.TeamRunV2StatusFailed, Error: "stale error",
		StartedAt: now, Seq: 2, Version: 0,
	})
	if err != nil {
		t.Fatalf("UpsertTeamRun stale: %v", err)
	}
	if stale.Status != biz.TeamRunV2StatusRunning || stale.Error != "" {
		t.Fatalf("stale overwrote: %+v", stale)
	}
	// Newer version (2) should overwrite with CompletedAt and Error.
	completedAt := now.Add(5 * time.Second)
	_, err = repo.UpsertTeamRun(ctx, biz.TeamRun{
		ID: "tr-2", TeamStageID: "ts-1", TaskID: "t-1",
		SessionID: "s-1", SpiritSessionID: "s-1",
		Status: biz.TeamRunV2StatusCompleted, Error: "",
		StartedAt: now, CompletedAt: &completedAt,
		Seq: 2, Version: 2,
	})
	if err != nil {
		t.Fatalf("UpsertTeamRun v2: %v", err)
	}
	got, _ := repo.GetTeamRun(ctx, "tr-2")
	if got.Status != biz.TeamRunV2StatusCompleted || got.Version != 2 {
		t.Fatalf("newer did not overwrite: %+v", got)
	}
	if got.CompletedAt == nil || !got.CompletedAt.Equal(completedAt) {
		t.Fatalf("completedAt mismatch: got %+v, expected %v", got.CompletedAt, completedAt)
	}
}

func TestTeamRunV2Repo_ListByStage_SeqOrder(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTeamRunV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	for i, seq := range []int64{3, 1, 2} {
		_, err := repo.CreateTeamRun(ctx, biz.TeamRun{
			ID: "ord-" + string(rune('a'+i)), TeamStageID: "ts-x", TaskID: "t-1",
			SessionID: "s-1", SpiritSessionID: "s-1",
			Status: biz.TeamRunV2StatusRunning,
			StartedAt: now, Seq: seq, Version: 1,
		})
		if err != nil {
			t.Fatalf("CreateTeamRun[%d]: %v", i, err)
		}
	}
	runs, err := repo.ListTeamRunsByStage(ctx, "ts-x")
	if err != nil {
		t.Fatalf("ListTeamRunsByStage: %v", err)
	}
	if len(runs) != 3 || runs[0].Seq != 1 || runs[2].Seq != 3 {
		t.Fatalf("order: expected 1,2,3 got %+v", []int64{runs[0].Seq, runs[1].Seq, runs[2].Seq})
	}
}

func TestTeamRunV2Repo_GetTeamRun_NotFound(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTeamRunV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()

	_, err := repo.GetTeamRun(ctx, "nonexistent")
	if err == nil {
		t.Fatalf("expected error for nonexistent team run, got nil")
	}
}
