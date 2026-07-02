package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/membersessionv2"
	"aranea-agents/pkg/loggateway"
)

// memberSessionV2Repo implements biz.MemberSessionV2Repo.
// Stability:evolving
type memberSessionV2Repo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.MemberSessionV2Repo = (*memberSessionV2Repo)(nil)

// NewMemberSessionV2Repo creates a new MemberSessionV2Repo.
// Logger is preset with domain "MEMBER_SESSION_V2" per loggateway convention.
func NewMemberSessionV2Repo(d *Data, lg loggateway.Logger) biz.MemberSessionV2Repo {
	return &memberSessionV2Repo{data: d, lg: lg.With(loggateway.Domain("MEMBER_SESSION_V2"))}
}

// GetMemberSession returns the MemberSession by ID.
func (r *memberSessionV2Repo) GetMemberSession(ctx context.Context, id string) (biz.MemberSession, error) {
	if r == nil || r.data == nil {
		return biz.MemberSession{}, fmt.Errorf("member session v2 repo: database not configured")
	}
	row, err := r.data.RW().Read(ctx).MemberSessionV2.Get(ctx, id)
	if err != nil {
		return biz.MemberSession{}, entErrToBizErr(err, "MEMBER_SESSION_V2")
	}
	return entMemberSessionV2ToBiz(row), nil
}

// ListMemberSessionsByRun returns all member sessions for the given team run, ordered by seq asc.
func (r *memberSessionV2Repo) ListMemberSessionsByRun(ctx context.Context, runID string) ([]biz.MemberSession, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("member session v2 repo: database not configured")
	}
	rows, err := r.data.RW().Read(ctx).MemberSessionV2.Query().
		Where(membersessionv2.TeamRunIDEQ(runID)).
		Order(ent.Asc(membersessionv2.FieldSeq)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMBER_SESSION_V2")
	}
	return entMemberSessionsV2ToBiz(rows), nil
}

// CreateMemberSession inserts a new MemberSession with the caller's claimed Version.
func (r *memberSessionV2Repo) CreateMemberSession(ctx context.Context, ms biz.MemberSession) (biz.MemberSession, error) {
	if r == nil || r.data == nil {
		return biz.MemberSession{}, fmt.Errorf("member session v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).MemberSessionV2.Create().
		SetID(ms.ID).
		SetTeamRunID(ms.TeamRunID).
		SetTeamStageID(ms.TeamStageID).
		SetTaskID(ms.TaskID).
		SetSessionID(ms.SessionID).
		SetSpiritSessionID(ms.SpiritSessionID).
		SetAgentKey(ms.AgentKey).
		SetAgentName(ms.AgentName).
		SetAvatarURL(ms.AvatarURL).
		SetStatus(string(ms.Status)).
		SetSeq(ms.Seq).
		SetVersion(ms.Version).
		SetStartedAt(ms.StartedAt).
		SetError(ms.Error)
	if ms.FinishedAt != nil {
		b.SetFinishedAt(*ms.FinishedAt)
	}
	row, err := b.Save(ctx)
	if err != nil {
		return biz.MemberSession{}, entErrToBizErr(err, "MEMBER_SESSION_V2")
	}
	return entMemberSessionV2ToBiz(row), nil
}

// UpdateMemberSession patches mutable fields without version guard.
// Use UpsertMemberSession for concurrent-safe writes.
func (r *memberSessionV2Repo) UpdateMemberSession(ctx context.Context, ms biz.MemberSession) (biz.MemberSession, error) {
	if r == nil || r.data == nil {
		return biz.MemberSession{}, fmt.Errorf("member session v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).MemberSessionV2.UpdateOneID(ms.ID).
		SetTeamRunID(ms.TeamRunID).
		SetTeamStageID(ms.TeamStageID).
		SetTaskID(ms.TaskID).
		SetSessionID(ms.SessionID).
		SetSpiritSessionID(ms.SpiritSessionID).
		SetAgentKey(ms.AgentKey).
		SetAgentName(ms.AgentName).
		SetAvatarURL(ms.AvatarURL).
		SetStatus(string(ms.Status)).
		SetSeq(ms.Seq).
		SetVersion(ms.Version).
		SetError(ms.Error)
	if ms.FinishedAt != nil {
		b.SetFinishedAt(*ms.FinishedAt)
	}
	row, err := b.Save(ctx)
	if err != nil {
		return biz.MemberSession{}, entErrToBizErr(err, "MEMBER_SESSION_V2")
	}
	return entMemberSessionV2ToBiz(row), nil
}

// UpsertMemberSession applies optimistic-concurrency upsert (see UpsertTask for semantics).
func (r *memberSessionV2Repo) UpsertMemberSession(ctx context.Context, ms biz.MemberSession) (biz.MemberSession, error) {
	if r == nil || r.data == nil {
		return biz.MemberSession{}, fmt.Errorf("member session v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).MemberSessionV2.UpdateOneID(ms.ID).
		Where(membersessionv2.VersionLT(ms.Version)).
		SetTeamRunID(ms.TeamRunID).
		SetTeamStageID(ms.TeamStageID).
		SetTaskID(ms.TaskID).
		SetSessionID(ms.SessionID).
		SetSpiritSessionID(ms.SpiritSessionID).
		SetAgentKey(ms.AgentKey).
		SetAgentName(ms.AgentName).
		SetAvatarURL(ms.AvatarURL).
		SetStatus(string(ms.Status)).
		SetSeq(ms.Seq).
		SetVersion(ms.Version).
		SetError(ms.Error)
	if ms.FinishedAt != nil {
		b.SetFinishedAt(*ms.FinishedAt)
	}
	if err := b.Exec(ctx); err == nil {
		row, getErr := r.data.RW().Read(ctx).MemberSessionV2.Get(ctx, ms.ID)
		if getErr != nil {
			return biz.MemberSession{}, entErrToBizErr(getErr, "MEMBER_SESSION_V2")
		}
		return entMemberSessionV2ToBiz(row), nil
	}
	cb := r.data.RW().Write(ctx).MemberSessionV2.Create().
		SetID(ms.ID).
		SetTeamRunID(ms.TeamRunID).
		SetTeamStageID(ms.TeamStageID).
		SetTaskID(ms.TaskID).
		SetSessionID(ms.SessionID).
		SetSpiritSessionID(ms.SpiritSessionID).
		SetAgentKey(ms.AgentKey).
		SetAgentName(ms.AgentName).
		SetAvatarURL(ms.AvatarURL).
		SetStatus(string(ms.Status)).
		SetSeq(ms.Seq).
		SetVersion(ms.Version).
		SetStartedAt(ms.StartedAt).
		SetError(ms.Error)
	if ms.FinishedAt != nil {
		cb.SetFinishedAt(*ms.FinishedAt)
	}
	row, err := cb.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			existing, getErr := r.data.RW().Read(ctx).MemberSessionV2.Get(ctx, ms.ID)
			if getErr != nil {
				return biz.MemberSession{}, entErrToBizErr(getErr, "MEMBER_SESSION_V2")
			}
			return entMemberSessionV2ToBiz(existing), nil
		}
		return biz.MemberSession{}, entErrToBizErr(err, "MEMBER_SESSION_V2")
	}
	return entMemberSessionV2ToBiz(row), nil
}

// entMemberSessionV2ToBiz converts an Ent MemberSessionV2 row to biz.MemberSession.
func entMemberSessionV2ToBiz(row *ent.MemberSessionV2) biz.MemberSession {
	var finishedAt *time.Time
	if row.FinishedAt != nil {
		t := *row.FinishedAt
		finishedAt = &t
	}
	return biz.MemberSession{
		ID:              row.ID,
		TeamRunID:       row.TeamRunID,
		TeamStageID:     row.TeamStageID,
		TaskID:          row.TaskID,
		SessionID:       row.SessionID,
		SpiritSessionID: row.SpiritSessionID,
		AgentKey:        row.AgentKey,
		AgentName:       row.AgentName,
		AvatarURL:       row.AvatarURL,
		Status:          biz.MemberSessionStatus(row.Status),
		Seq:             row.Seq,
		Version:         row.Version,
		StartedAt:       row.StartedAt,
		FinishedAt:      finishedAt,
		Error:           row.Error,
	}
}

func entMemberSessionsV2ToBiz(rows []*ent.MemberSessionV2) []biz.MemberSession {
	out := make([]biz.MemberSession, 0, len(rows))
	for _, r := range rows {
		out = append(out, entMemberSessionV2ToBiz(r))
	}
	return out
}
