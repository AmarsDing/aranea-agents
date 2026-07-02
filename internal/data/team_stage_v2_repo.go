package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/teamstagev2"
	"aranea-agents/pkg/loggateway"
)

// teamStageV2Repo implements biz.TeamStageV2Repo.
// Stability:evolving
type teamStageV2Repo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.TeamStageV2Repo = (*teamStageV2Repo)(nil)

// NewTeamStageV2Repo creates a new TeamStageV2Repo.
// Logger is preset with domain "TEAM_STAGE_V2" per loggateway convention.
func NewTeamStageV2Repo(d *Data, lg loggateway.Logger) biz.TeamStageV2Repo {
	return &teamStageV2Repo{data: d, lg: lg.With(loggateway.Domain("TEAM_STAGE_V2"))}
}

// GetTeamStage returns the TeamStage by ID.
func (r *teamStageV2Repo) GetTeamStage(ctx context.Context, id string) (biz.TeamStage, error) {
	if r == nil || r.data == nil {
		return biz.TeamStage{}, fmt.Errorf("team stage v2 repo: database not configured")
	}
	row, err := r.data.RW().Read(ctx).TeamStageV2.Get(ctx, id)
	if err != nil {
		return biz.TeamStage{}, entErrToBizErr(err, "TEAM_STAGE_V2")
	}
	return entTeamStageV2ToBiz(row), nil
}

// ListTeamStagesByTask returns all team stages for the given task, ordered by seq asc.
func (r *teamStageV2Repo) ListTeamStagesByTask(ctx context.Context, taskID string) ([]biz.TeamStage, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("team stage v2 repo: database not configured")
	}
	rows, err := r.data.RW().Read(ctx).TeamStageV2.Query().
		Where(teamstagev2.TaskIDEQ(taskID)).
		Order(ent.Asc(teamstagev2.FieldSeq)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "TEAM_STAGE_V2")
	}
	return entTeamStagesV2ToBiz(rows), nil
}

// CreateTeamStage inserts a new TeamStage with the caller's claimed Version.
func (r *teamStageV2Repo) CreateTeamStage(ctx context.Context, ts biz.TeamStage) (biz.TeamStage, error) {
	if r == nil || r.data == nil {
		return biz.TeamStage{}, fmt.Errorf("team stage v2 repo: database not configured")
	}
	row, err := r.data.RW().Write(ctx).TeamStageV2.Create().
		SetID(ts.ID).
		SetTaskID(ts.TaskID).
		SetTurnID(ts.TurnID).
		SetSessionID(ts.SessionID).
		SetTeamID(ts.TeamID).
		SetDagNodeID(ts.DagNodeID).
		SetDependsOn(ts.DependsOn).
		SetStatus(string(ts.Status)).
		SetStage(string(ts.Stage)).
		SetMembers(membersToEnt(ts.Members)).
		SetStrategy(ts.Strategy).
		SetStartedAt(ts.StartedAt).
		SetSeq(ts.Seq).
		SetVersion(ts.Version).
		Save(ctx)
	if err != nil {
		return biz.TeamStage{}, entErrToBizErr(err, "TEAM_STAGE_V2")
	}
	return entTeamStageV2ToBiz(row), nil
}

// UpdateTeamStage patches mutable fields without version guard.
// Use UpsertTeamStage for concurrent-safe writes.
func (r *teamStageV2Repo) UpdateTeamStage(ctx context.Context, ts biz.TeamStage) (biz.TeamStage, error) {
	if r == nil || r.data == nil {
		return biz.TeamStage{}, fmt.Errorf("team stage v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).TeamStageV2.UpdateOneID(ts.ID).
		SetTaskID(ts.TaskID).
		SetTurnID(ts.TurnID).
		SetSessionID(ts.SessionID).
		SetTeamID(ts.TeamID).
		SetDagNodeID(ts.DagNodeID).
		SetDependsOn(ts.DependsOn).
		SetStatus(string(ts.Status)).
		SetStage(string(ts.Stage)).
		SetMembers(membersToEnt(ts.Members)).
		SetStrategy(ts.Strategy).
		SetSeq(ts.Seq).
		SetVersion(ts.Version)
	if ts.CompletedAt != nil {
		b.SetCompletedAt(*ts.CompletedAt)
	}
	row, err := b.Save(ctx)
	if err != nil {
		return biz.TeamStage{}, entErrToBizErr(err, "TEAM_STAGE_V2")
	}
	return entTeamStageV2ToBiz(row), nil
}

// UpsertTeamStage applies optimistic-concurrency upsert (see UpsertTask for semantics).
func (r *teamStageV2Repo) UpsertTeamStage(ctx context.Context, ts biz.TeamStage) (biz.TeamStage, error) {
	if r == nil || r.data == nil {
		return biz.TeamStage{}, fmt.Errorf("team stage v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).TeamStageV2.UpdateOneID(ts.ID).
		Where(teamstagev2.VersionLT(ts.Version)).
		SetTaskID(ts.TaskID).
		SetTurnID(ts.TurnID).
		SetSessionID(ts.SessionID).
		SetTeamID(ts.TeamID).
		SetDagNodeID(ts.DagNodeID).
		SetDependsOn(ts.DependsOn).
		SetStatus(string(ts.Status)).
		SetStage(string(ts.Stage)).
		SetMembers(membersToEnt(ts.Members)).
		SetStrategy(ts.Strategy).
		SetSeq(ts.Seq).
		SetVersion(ts.Version)
	if ts.CompletedAt != nil {
		b.SetCompletedAt(*ts.CompletedAt)
	}
	if err := b.Exec(ctx); err == nil {
		row, getErr := r.data.RW().Read(ctx).TeamStageV2.Get(ctx, ts.ID)
		if getErr != nil {
			return biz.TeamStage{}, entErrToBizErr(getErr, "TEAM_STAGE_V2")
		}
		return entTeamStageV2ToBiz(row), nil
	}
	cb := r.data.RW().Write(ctx).TeamStageV2.Create().
		SetID(ts.ID).
		SetTaskID(ts.TaskID).
		SetTurnID(ts.TurnID).
		SetSessionID(ts.SessionID).
		SetTeamID(ts.TeamID).
		SetDagNodeID(ts.DagNodeID).
		SetDependsOn(ts.DependsOn).
		SetStatus(string(ts.Status)).
		SetStage(string(ts.Stage)).
		SetMembers(membersToEnt(ts.Members)).
		SetStrategy(ts.Strategy).
		SetStartedAt(ts.StartedAt).
		SetSeq(ts.Seq).
		SetVersion(ts.Version)
	if ts.CompletedAt != nil {
		cb.SetCompletedAt(*ts.CompletedAt)
	}
	row, err := cb.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			existing, getErr := r.data.RW().Read(ctx).TeamStageV2.Get(ctx, ts.ID)
			if getErr != nil {
				return biz.TeamStage{}, entErrToBizErr(getErr, "TEAM_STAGE_V2")
			}
			return entTeamStageV2ToBiz(existing), nil
		}
		return biz.TeamStage{}, entErrToBizErr(err, "TEAM_STAGE_V2")
	}
	return entTeamStageV2ToBiz(row), nil
}

// entTeamStageV2ToBiz converts an Ent TeamStageV2 row to biz.TeamStage.
// SessionID field holds spirit_session_id value (see Ent schema comment).
// Members is stored as []map[string]any in Ent, converted to []MemberInfo.
func entTeamStageV2ToBiz(row *ent.TeamStageV2) biz.TeamStage {
	var completedAt *time.Time
	if row.CompletedAt != nil {
		t := *row.CompletedAt
		completedAt = &t
	}
	return biz.TeamStage{
		ID:          row.ID,
		TaskID:      row.TaskID,
		TurnID:      row.TurnID,
		SessionID:   row.SessionID,
		TeamID:      row.TeamID,
		DagNodeID:   row.DagNodeID,
		DependsOn:   row.DependsOn,
		Status:      biz.TeamStageStatus(row.Status),
		Stage:       biz.TeamStageStage(row.Stage),
		Members:     membersFromEnt(row.Members),
		Strategy:    row.Strategy,
		StartedAt:   row.StartedAt,
		CompletedAt: completedAt,
		Seq:        row.Seq,
		Version:    row.Version,
	}
}

func entTeamStagesV2ToBiz(rows []*ent.TeamStageV2) []biz.TeamStage {
	out := make([]biz.TeamStage, 0, len(rows))
	for _, r := range rows {
		out = append(out, entTeamStageV2ToBiz(r))
	}
	return out
}

// membersToEnt converts biz.MemberInfo slice to Ent's []map[string]any for JSON field.
func membersToEnt(members []biz.MemberInfo) []map[string]any {
	if len(members) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(members))
	for _, m := range members {
		out = append(out, map[string]any{
			"agent_key":        m.AgentKey,
			"agent_name":       m.AgentName,
			"avatar_url":       m.AvatarURL,
			"child_session_id": m.ChildSessionID,
			"status":           m.Status,
		})
	}
	return out
}

// membersFromEnt converts Ent's []map[string]any back to biz.MemberInfo slice.
func membersFromEnt(raw []map[string]any) []biz.MemberInfo {
	if len(raw) == 0 {
		return nil
	}
	out := make([]biz.MemberInfo, 0, len(raw))
	for _, m := range raw {
		out = append(out, biz.MemberInfo{
			AgentKey:       stringFromMap(m, "agent_key"),
			AgentName:      stringFromMap(m, "agent_name"),
			AvatarURL:      stringFromMap(m, "avatar_url"),
			ChildSessionID: stringFromMap(m, "child_session_id"),
			Status:         stringFromMap(m, "status"),
		})
	}
	return out
}

// stringFromMap safely extracts a string value from a map[string]any.
func stringFromMap(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}
