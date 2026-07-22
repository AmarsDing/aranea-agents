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
		Status:    biz.TeamRunV2StatusRunning,
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
		Status:    biz.TeamRunV2StatusRunning,
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
			Status:    biz.TeamRunV2StatusRunning,
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

// B.10.17 execution report: latest run stats per team via team_stages_v2 join.
func TestTeamRunV2Repo_ListLatestRunStatsByTeams(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTeamRunV2Repo(d, loggateway.NewNoop())
	tsRepo := NewTeamStageV2Repo(d, loggateway.NewNoop())
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Second)

	seedStage := func(id, teamID string, seq int64) {
		t.Helper()
		if _, err := tsRepo.CreateTeamStage(ctx, biz.TeamStage{
			ID: id, TaskID: "task-1", TurnID: "turn-1", SessionID: "spirit-1",
			TeamID: teamID, Status: biz.TeamStageStatusCompleted,
			StartedAt: base, Seq: seq, Version: 1,
		}); err != nil {
			t.Fatalf("seed stage %s: %v", id, err)
		}
	}
	seedRun := func(id, stageID string, status biz.TeamRunV2Status, startedAt time.Time, completedAt *time.Time, seq int64, runErr string) {
		t.Helper()
		if _, err := repo.CreateTeamRun(ctx, biz.TeamRun{
			ID: id, TeamStageID: stageID, TaskID: "task-1",
			SessionID: "sess-" + stageID, SpiritSessionID: "spirit-1",
			Status: status, StartedAt: startedAt,
			CompletedAt: completedAt, Seq: seq, Version: 1, Error: runErr,
		}); err != nil {
			t.Fatalf("seed run %s: %v", id, err)
		}
	}
	completedAt := func(offset time.Duration) *time.Time {
		ts := base.Add(offset)
		return &ts
	}

	// team-1: single stage, two runs — latest (higher started_at) wins.
	seedStage("stage-t1", "team-1", 1)
	seedRun("run-t1-old", "stage-t1", biz.TeamRunV2StatusCompleted, base, completedAt(2*time.Second), 1, "")
	seedRun("run-t1-new", "stage-t1", biz.TeamRunV2StatusCompleted, base.Add(10*time.Second), completedAt(14*time.Second), 2, "")

	// team-2: two stages (re-run) — the newer stage's run wins.
	seedStage("stage-t2-old", "team-2", 1)
	seedRun("run-t2-old", "stage-t2-old", biz.TeamRunV2StatusCompleted, base, completedAt(time.Second), 1, "")
	seedStage("stage-t2-new", "team-2", 2)
	seedRun("run-t2-new", "stage-t2-new", biz.TeamRunV2StatusFailed, base.Add(20*time.Second), completedAt(25*time.Second), 1, "boom")

	// team-3: stage exists but run not completed → duration 0.
	seedStage("stage-t3", "team-3", 1)
	seedRun("run-t3", "stage-t3", biz.TeamRunV2StatusRunning, base.Add(30*time.Second), nil, 1, "")

	// team-4: no stages at all → omitted from the result map.

	statsRepo := repo.(biz.SpiritTeamRunStatsReader)
	stats, err := statsRepo.ListLatestRunStatsByTeams(ctx, []string{"team-1", "team-2", "team-3", "team-4"})
	if err != nil {
		t.Fatalf("ListLatestRunStatsByTeams: %v", err)
	}
	if len(stats) != 3 {
		t.Fatalf("expected 3 teams with stats, got %d: %+v", len(stats), stats)
	}
	if s := stats["team-1"]; s.DurationMs != 4000 || s.ErrorMessage != "" {
		t.Fatalf("team-1 stats mismatch: %+v", s)
	}
	if s := stats["team-2"]; s.DurationMs != 5000 || s.ErrorMessage != "boom" {
		t.Fatalf("team-2 stats mismatch: %+v", s)
	}
	if s := stats["team-3"]; s.DurationMs != 0 {
		t.Fatalf("team-3 uncompleted run must report duration 0, got %+v", s)
	}
}

func TestTeamRunV2Repo_ListLatestRunStatsByTeams_EmptyInput(t *testing.T) {
	d := openTestDataWithRWDB(t)
	repo := NewTeamRunV2Repo(d, loggateway.NewNoop())

	stats, err := repo.(biz.SpiritTeamRunStatsReader).ListLatestRunStatsByTeams(context.Background(), nil)
	if err != nil || stats != nil {
		t.Fatalf("empty input must return (nil, nil), got stats=%+v err=%v", stats, err)
	}
}
