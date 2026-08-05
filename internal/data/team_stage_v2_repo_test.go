package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestTeamStageV2Repo_CreateAndGet(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTeamStageV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	created, err := repo.CreateTeamStage(ctx, biz.TeamStage{
		ID: "ts-1", TaskID: "t-1", TurnID: "turn-1", SessionID: "s-1",
		TeamID: "team-1", DagNodeID: "node-1", DependsOn: []string{"ts-0"},
		Status: biz.TeamStageStatusPending, Stage: biz.TeamStageStageAssembled,
		Members: []biz.MemberInfo{
			{AgentKey: "agent-1", AgentName: "Agent One", Status: "pending"},
		},
		Strategy:  "parallel",
		StartedAt: now, Seq: 1, Version: 1,
	})
	if err != nil {
		t.Fatalf("CreateTeamStage: %v", err)
	}
	if created.ID != "ts-1" || created.TeamID != "team-1" {
		t.Fatalf("created mismatch: %+v", created)
	}

	got, err := repo.GetTeamStage(ctx, "ts-1")
	if err != nil {
		t.Fatalf("GetTeamStage: %v", err)
	}
	if got.TeamID != "team-1" || got.Stage != biz.TeamStageStageAssembled {
		t.Fatalf("team stage mismatch: %+v", got)
	}
	if len(got.Members) != 1 || got.Members[0].AgentKey != "agent-1" {
		t.Fatalf("members mismatch: %+v", got.Members)
	}
	if len(got.DependsOn) != 1 || got.DependsOn[0] != "ts-0" {
		t.Fatalf("depends_on mismatch: %+v", got.DependsOn)
	}
}

func TestTeamStageV2Repo_Upsert_VersionGuard(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTeamStageV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	_, err := repo.UpsertTeamStage(ctx, biz.TeamStage{
		ID: "ts-2", TaskID: "t-1", TurnID: "turn-1", SessionID: "s-1",
		TeamID: "team-1", Status: biz.TeamStageStatusRunning,
		Stage: biz.TeamStageStagePlanning, Strategy: "dag",
		StartedAt: now, Seq: 2, Version: 1,
	})
	if err != nil {
		t.Fatalf("UpsertTeamStage v1: %v", err)
	}
	// Stale version (0) should not overwrite v1.
	stale, err := repo.UpsertTeamStage(ctx, biz.TeamStage{
		ID: "ts-2", TaskID: "t-1", TurnID: "turn-1", SessionID: "s-1",
		TeamID: "team-1", Status: biz.TeamStageStatusFailed,
		Stage: biz.TeamStageStageFailed, Strategy: "stale",
		StartedAt: now, Seq: 2, Version: 0,
	})
	if err != nil {
		t.Fatalf("UpsertTeamStage stale: %v", err)
	}
	if stale.Status != biz.TeamStageStatusRunning || stale.Strategy != "dag" {
		t.Fatalf("stale overwrote: %+v", stale)
	}
	// Newer version (2) should overwrite.
	_, err = repo.UpsertTeamStage(ctx, biz.TeamStage{
		ID: "ts-2", TaskID: "t-1", TurnID: "turn-1", SessionID: "s-1",
		TeamID: "team-1", Status: biz.TeamStageStatusCompleted,
		Stage: biz.TeamStageStageCompleted, Strategy: "dag",
		StartedAt: now, Seq: 2, Version: 2,
	})
	if err != nil {
		t.Fatalf("UpsertTeamStage v2: %v", err)
	}
	got, _ := repo.GetTeamStage(ctx, "ts-2")
	if got.Status != biz.TeamStageStatusCompleted || got.Version != 2 {
		t.Fatalf("newer did not overwrite: %+v", got)
	}
}

func TestTeamStageV2Repo_ListByTask_SeqOrder(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTeamStageV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	for i, seq := range []int64{3, 1, 2} {
		_, err := repo.CreateTeamStage(ctx, biz.TeamStage{
			ID: "ord-" + string(rune('a'+i)), TaskID: "t-x", TurnID: "turn-1",
			SessionID: "s-1", TeamID: "team-1",
			Status: biz.TeamStageStatusPending, Stage: biz.TeamStageStageAssembled,
			StartedAt: now, Seq: seq, Version: 1,
		})
		if err != nil {
			t.Fatalf("CreateTeamStage[%d]: %v", i, err)
		}
	}
	stages, err := repo.ListTeamStagesByTask(ctx, "t-x")
	if err != nil {
		t.Fatalf("ListTeamStagesByTask: %v", err)
	}
	if len(stages) != 3 || stages[0].Seq != 1 || stages[2].Seq != 3 {
		t.Fatalf("order: expected 1,2,3 got %+v", []int64{stages[0].Seq, stages[1].Seq, stages[2].Seq})
	}
}

func TestTeamStageV2Repo_GetTeamStage_NotFound(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTeamStageV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()

	_, err := repo.GetTeamStage(ctx, "nonexistent")
	if err == nil {
		t.Fatalf("expected error for nonexistent team stage, got nil")
	}
}

// TestTeamStageV2Repo_GetLatestByTeam verifies S-3's reader for ctx-less
// paths (spirit view / pause sync / cancel): run-isolated stage IDs make
// deterministic derivation impossible, so these paths query the latest
// stage by team (StartedAt desc, Seq desc tiebreak).
func TestTeamStageV2Repo_GetLatestByTeam(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTeamStageV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Second)

	// Two runs of team-1 (older + newer) and one row for another team.
	for _, ts := range []biz.TeamStage{
		{ID: "ts-old", TeamID: "team-1", SessionID: "s-1", Status: biz.TeamStageStatusCompleted, Stage: biz.TeamStageStageCompleted, StartedAt: base, Seq: 1, Version: 1},
		{ID: "ts-new", TeamID: "team-1", SessionID: "s-1", Status: biz.TeamStageStatusRunning, Stage: biz.TeamStageStageExecuting, StartedAt: base.Add(time.Minute), Seq: 2, Version: 1},
		{ID: "ts-other", TeamID: "team-2", SessionID: "s-1", Status: biz.TeamStageStatusRunning, Stage: biz.TeamStageStageExecuting, StartedAt: base.Add(2 * time.Minute), Seq: 3, Version: 1},
	} {
		if _, err := repo.CreateTeamStage(ctx, ts); err != nil {
			t.Fatalf("CreateTeamStage %s: %v", ts.ID, err)
		}
	}

	got, err := repo.GetLatestTeamStageByTeam(ctx, "team-1")
	if err != nil {
		t.Fatalf("GetLatestTeamStageByTeam: %v", err)
	}
	if got.ID != "ts-new" {
		t.Fatalf("latest = %q, want ts-new", got.ID)
	}

	if _, err := repo.GetLatestTeamStageByTeam(ctx, "team-no-such"); err == nil {
		t.Fatal("expected NotFound error for unknown team")
	}
}
