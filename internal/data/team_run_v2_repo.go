package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/teamrunv2"
	"aranea-agents/pkg/loggateway"
)

// teamRunV2Repo implements biz.TeamRunV2Repo.
// Stability:evolving
type teamRunV2Repo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.TeamRunV2Repo = (*teamRunV2Repo)(nil)

// NewTeamRunV2Repo creates a new TeamRunV2Repo.
// Logger is preset with domain "TEAM_RUN_V2" per loggateway convention.
func NewTeamRunV2Repo(d *Data, lg loggateway.Logger) biz.TeamRunV2Repo {
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
		if ent.IsConstraintError(err) {
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
