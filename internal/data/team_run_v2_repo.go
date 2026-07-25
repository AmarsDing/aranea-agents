package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/teamrunv2"
	"aranea-agents/internal/data/ent/teamstagev2"
	"aranea-agents/pkg/loggateway"
)

// teamRunV2Repo implements biz.TeamRunV2Repo.
// Stability:evolving
type teamRunV2Repo struct {
	data *Data
	lg   loggateway.Logger
}

var (
	_ biz.TeamRunV2Repo            = (*teamRunV2Repo)(nil)
	_ biz.SpiritTeamRunStatsReader = (*teamRunV2Repo)(nil)
)

// NewTeamRunV2Repo creates a new TeamRunV2Repo.
// Logger is preset with domain "TEAM_RUN_V2" per loggateway convention.
func NewTeamRunV2Repo(d *Data, lg loggateway.Logger) biz.TeamRunV2Repo {
	return &teamRunV2Repo{data: d, lg: lg.With(loggateway.Domain("TEAM_RUN_V2"))}
}

// NewSpiritTeamRunStatsReader exposes the team run v2 repo through the
// stats-reader port consumed by the execution report (B.10.17). Returned as
// the biz interface directly so Wire needs no Bind.
func NewSpiritTeamRunStatsReader(d *Data, lg loggateway.Logger) biz.SpiritTeamRunStatsReader {
	return &teamRunV2Repo{data: d, lg: lg.With(loggateway.Domain("TEAM_RUN_V2"))}
}

// GetTeamRun returns the TeamRun by ID.
func (r *teamRunV2Repo) GetTeamRun(ctx context.Context, id string) (biz.TeamRun, error) {
	if r == nil || r.data == nil {
		return biz.TeamRun{}, fmt.Errorf("team run v2 repo: database not configured")
	}
	row, err := r.data.RW().Read(ctx).TeamRunV2.Get(ctx, id)
	if err != nil {
		return biz.TeamRun{}, entErrToBizErr(err, "TEAM_RUN_V2")
	}
	return entTeamRunV2ToBiz(row), nil
}

// ListTeamRunsByStage returns all team runs for the given stage, ordered by seq asc.
func (r *teamRunV2Repo) ListTeamRunsByStage(ctx context.Context, stageID string) ([]biz.TeamRun, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("team run v2 repo: database not configured")
	}
	rows, err := r.data.RW().Read(ctx).TeamRunV2.Query().
		Where(teamrunv2.TeamStageIDEQ(stageID)).
		Order(ent.Asc(teamrunv2.FieldSeq)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "TEAM_RUN_V2")
	}
	return entTeamRunsV2ToBiz(rows), nil
}

// ListLatestRunStatsByTeams implements biz.SpiritTeamRunStatsReader (B.10.17
// execution report). team_runs_v2 has no direct team_id column — the mapping
// goes through team_stages_v2 (team_id → stage id → runs). The latest run per
// team is picked by (started_at, seq) across all of the team's stages.
func (r *teamRunV2Repo) ListLatestRunStatsByTeams(ctx context.Context, teamIDs []string) (map[string]biz.SpiritTeamRunStats, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("team run v2 repo: database not configured")
	}
	if len(teamIDs) == 0 {
		return nil, nil
	}
	stages, err := r.data.RW().Read(ctx).TeamStageV2.Query().
		Where(teamstagev2.TeamIDIn(teamIDs...)).
		Select(teamstagev2.FieldID, teamstagev2.FieldTeamID).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "TEAM_RUN_V2")
	}
	if len(stages) == 0 {
		return nil, nil
	}
	stageToTeam := make(map[string]string, len(stages))
	stageIDs := make([]string, 0, len(stages))
	for _, s := range stages {
		stageToTeam[s.ID] = s.TeamID
		stageIDs = append(stageIDs, s.ID)
	}
	runs, err := r.data.RW().Read(ctx).TeamRunV2.Query().
		Where(teamrunv2.TeamStageIDIn(stageIDs...)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "TEAM_RUN_V2")
	}
	latest := make(map[string]*ent.TeamRunV2, len(teamIDs))
	for _, run := range runs {
		teamID, ok := stageToTeam[run.TeamStageID]
		if !ok {
			continue
		}
		cur := latest[teamID]
		if cur == nil || run.StartedAt.After(cur.StartedAt) ||
			(run.StartedAt.Equal(cur.StartedAt) && run.Seq > cur.Seq) {
			latest[teamID] = run
		}
	}
	out := make(map[string]biz.SpiritTeamRunStats, len(latest))
	for teamID, run := range latest {
		out[teamID] = biz.SpiritTeamRunStats{
			TeamID:       teamID,
			DurationMs:   teamRunV2DurationMs(run),
			ErrorMessage: run.Error,
		}
	}
	return out, nil
}

// teamRunV2DurationMs returns completed_at - started_at in milliseconds, or 0
// when the run has not completed or the timestamps are inverted.
func teamRunV2DurationMs(run *ent.TeamRunV2) int64 {
	if run.CompletedAt == nil {
		return 0
	}
	d := run.CompletedAt.Sub(run.StartedAt).Milliseconds()
	if d < 0 {
		return 0
	}
	return d
}

// CreateTeamRun inserts a new TeamRun with the caller's claimed Version.
func (r *teamRunV2Repo) CreateTeamRun(ctx context.Context, tr biz.TeamRun) (biz.TeamRun, error) {
	if r == nil || r.data == nil {
		return biz.TeamRun{}, fmt.Errorf("team run v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).TeamRunV2.Create().
		SetID(tr.ID).
		SetTeamStageID(tr.TeamStageID).
		SetTaskID(tr.TaskID).
		SetSessionID(tr.SessionID).
		SetSpiritSessionID(tr.SpiritSessionID).
		SetDagNodeID(tr.DagNodeID).
		SetDependsOn(tr.DependsOn).
		SetStatus(string(tr.Status)).
		SetStartedAt(tr.StartedAt).
		SetSeq(tr.Seq).
		SetError(tr.Error).
		SetVersion(tr.Version)
	if tr.CompletedAt != nil {
		b.SetCompletedAt(*tr.CompletedAt)
	}
	row, err := b.Save(ctx)
	if err != nil {
		return biz.TeamRun{}, entErrToBizErr(err, "TEAM_RUN_V2")
	}
	return entTeamRunV2ToBiz(row), nil
}

// UpdateTeamRun patches mutable fields without version guard.
// Use UpsertTeamRun for concurrent-safe writes.
func (r *teamRunV2Repo) UpdateTeamRun(ctx context.Context, tr biz.TeamRun) (biz.TeamRun, error) {
	if r == nil || r.data == nil {
		return biz.TeamRun{}, fmt.Errorf("team run v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).TeamRunV2.UpdateOneID(tr.ID).
		SetTeamStageID(tr.TeamStageID).
		SetTaskID(tr.TaskID).
		SetSessionID(tr.SessionID).
		SetSpiritSessionID(tr.SpiritSessionID).
		SetDagNodeID(tr.DagNodeID).
		SetDependsOn(tr.DependsOn).
		SetStatus(string(tr.Status)).
		SetSeq(tr.Seq).
		SetError(tr.Error).
		SetVersion(tr.Version)
	if tr.CompletedAt != nil {
		b.SetCompletedAt(*tr.CompletedAt)
	}
	row, err := b.Save(ctx)
	if err != nil {
		return biz.TeamRun{}, entErrToBizErr(err, "TEAM_RUN_V2")
	}
	return entTeamRunV2ToBiz(row), nil
}

// UpsertTeamRun applies optimistic-concurrency upsert (see UpsertTask for semantics).
func (r *teamRunV2Repo) UpsertTeamRun(ctx context.Context, tr biz.TeamRun) (biz.TeamRun, error) {
	if r == nil || r.data == nil {
		return biz.TeamRun{}, fmt.Errorf("team run v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).TeamRunV2.UpdateOneID(tr.ID).
		Where(teamrunv2.VersionLT(tr.Version)).
		SetTeamStageID(tr.TeamStageID).
		SetTaskID(tr.TaskID).
		SetSessionID(tr.SessionID).
		SetSpiritSessionID(tr.SpiritSessionID).
		SetDagNodeID(tr.DagNodeID).
		SetDependsOn(tr.DependsOn).
		SetStatus(string(tr.Status)).
		SetSeq(tr.Seq).
		SetError(tr.Error).
		SetVersion(tr.Version)
	if tr.CompletedAt != nil {
		b.SetCompletedAt(*tr.CompletedAt)
	}
	if err := b.Exec(ctx); err == nil {
		row, getErr := r.data.RW().Read(ctx).TeamRunV2.Get(ctx, tr.ID)
		if getErr != nil {
			return biz.TeamRun{}, entErrToBizErr(getErr, "TEAM_RUN_V2")
		}
		return entTeamRunV2ToBiz(row), nil
	}
	// UPDATE failed. Two possible causes:
	//   1. Record doesn't exist yet → fall through to CREATE.
	//   2. Record exists but Version >= tr.Version (WHERE didn't match) →
	//      return existing record (idempotent: a newer version is already
	//      persisted, e.g. sync persist wrote before the async event arrived).
	//      Without this check, the CREATE fallback would fail with CONFLICT
	//      and propagate an error to the v2 sequencer's retry loop.
	if existing, getErr := r.data.RW().Read(ctx).TeamRunV2.Get(ctx, tr.ID); getErr == nil {
		return entTeamRunV2ToBiz(existing), nil
	}
	cb := r.data.RW().Write(ctx).TeamRunV2.Create().
		SetID(tr.ID).
		SetTeamStageID(tr.TeamStageID).
		SetTaskID(tr.TaskID).
		SetSessionID(tr.SessionID).
		SetSpiritSessionID(tr.SpiritSessionID).
		SetDagNodeID(tr.DagNodeID).
		SetDependsOn(tr.DependsOn).
		SetStatus(string(tr.Status)).
		SetStartedAt(tr.StartedAt).
		SetSeq(tr.Seq).
		SetError(tr.Error).
		SetVersion(tr.Version)
	if tr.CompletedAt != nil {
		cb.SetCompletedAt(*tr.CompletedAt)
	}
	row, err := cb.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) || isPgUniqueViolation(err) {
			existing, getErr := r.data.RW().Read(ctx).TeamRunV2.Get(ctx, tr.ID)
			if getErr != nil {
				return biz.TeamRun{}, entErrToBizErr(getErr, "TEAM_RUN_V2")
			}
			return entTeamRunV2ToBiz(existing), nil
		}
		return biz.TeamRun{}, entErrToBizErr(err, "TEAM_RUN_V2")
	}
	return entTeamRunV2ToBiz(row), nil
}

// entTeamRunV2ToBiz converts an Ent TeamRunV2 row to biz.TeamRun.
func entTeamRunV2ToBiz(row *ent.TeamRunV2) biz.TeamRun {
	var completedAt *time.Time
	if row.CompletedAt != nil {
		t := *row.CompletedAt
		completedAt = &t
	}
	return biz.TeamRun{
		ID:              row.ID,
		TeamStageID:     row.TeamStageID,
		TaskID:          row.TaskID,
		SessionID:       row.SessionID,
		SpiritSessionID: row.SpiritSessionID,
		DagNodeID:       row.DagNodeID,
		DependsOn:       row.DependsOn,
		Status:          biz.TeamRunV2Status(row.Status),
		StartedAt:       row.StartedAt,
		CompletedAt:     completedAt,
		Seq:             row.Seq,
		Version:         row.Version,
		Error:           row.Error,
	}
}

func entTeamRunsV2ToBiz(rows []*ent.TeamRunV2) []biz.TeamRun {
	out := make([]biz.TeamRun, 0, len(rows))
	for _, r := range rows {
		out = append(out, entTeamRunV2ToBiz(r))
	}
	return out
}
