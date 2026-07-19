package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/turnv2"
	"aranea-agents/pkg/loggateway"
)

// turnV2Repo implements biz.TurnV2Repo.
// Stability:evolving
type turnV2Repo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.TurnV2Repo = (*turnV2Repo)(nil)

// NewTurnV2Repo creates a new TurnV2Repo.
// Logger is preset with domain "TURN_V2" per loggateway convention.
func NewTurnV2Repo(d *Data, lg loggateway.Logger) biz.TurnV2Repo {
	return &turnV2Repo{data: d, lg: lg.With(loggateway.Domain("TURN_V2"))}
}

// GetTurn returns the Turn by ID.
func (r *turnV2Repo) GetTurn(ctx context.Context, id string) (biz.Turn, error) {
	if r == nil || r.data == nil {
		return biz.Turn{}, fmt.Errorf("turn v2 repo: database not configured")
	}
	row, err := r.data.RW().Read(ctx).TurnV2.Get(ctx, id)
	if err != nil {
		return biz.Turn{}, entErrToBizErr(err, "TURN_V2")
	}
	return entTurnV2ToBiz(row), nil
}

// ListTurnsByTask returns all turns for the given task, ordered by seq asc.
func (r *turnV2Repo) ListTurnsByTask(ctx context.Context, taskID string) ([]biz.Turn, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("turn v2 repo: database not configured")
	}
	rows, err := r.data.RW().Read(ctx).TurnV2.Query().
		Where(turnv2.TaskIDEQ(taskID)).
		Order(ent.Asc(turnv2.FieldSeq)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "TURN_V2")
	}
	return entTurnsV2ToBiz(rows), nil
}

// CreateTurn inserts a new Turn with the caller's claimed Version.
func (r *turnV2Repo) CreateTurn(ctx context.Context, t biz.Turn) (biz.Turn, error) {
	if r == nil || r.data == nil {
		return biz.Turn{}, fmt.Errorf("turn v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).TurnV2.Create().
		SetID(t.ID).
		SetTaskID(t.TaskID).
		SetSessionID(t.SessionID).
		SetSpiritSessionID(t.SpiritSessionID).
		SetParentTurnID(t.ParentTurnID).
		SetAgentKey(t.AgentKey).
		SetTeamID(t.TeamID).
		SetTeamStageID(t.TeamStageID).
		SetSeq(t.Seq).
		SetStatus(string(t.Status)).
		SetStartedAt(t.StartedAt).
		SetVersion(t.Version)
	if t.CompletedAt != nil {
		b.SetCompletedAt(*t.CompletedAt)
	}
	row, err := b.Save(ctx)
	if err != nil {
		return biz.Turn{}, entErrToBizErr(err, "TURN_V2")
	}
	return entTurnV2ToBiz(row), nil
}

// UpdateTurn patches status (and CompletedAt) without version guard.
// Use UpsertTurn for concurrent-safe writes.
func (r *turnV2Repo) UpdateTurn(ctx context.Context, t biz.Turn) (biz.Turn, error) {
	if r == nil || r.data == nil {
		return biz.Turn{}, fmt.Errorf("turn v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).TurnV2.UpdateOneID(t.ID).
		SetStatus(string(t.Status)).
		SetVersion(t.Version)
	if t.CompletedAt != nil {
		b.SetCompletedAt(*t.CompletedAt)
	}
	row, err := b.Save(ctx)
	if err != nil {
		return biz.Turn{}, entErrToBizErr(err, "TURN_V2")
	}
	return entTurnV2ToBiz(row), nil
}

// UpsertTurn applies optimistic-concurrency upsert (see UpsertTask for semantics).
func (r *turnV2Repo) UpsertTurn(ctx context.Context, t biz.Turn) (biz.Turn, error) {
	if r == nil || r.data == nil {
		return biz.Turn{}, fmt.Errorf("turn v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).TurnV2.UpdateOneID(t.ID).
		Where(turnv2.VersionLT(t.Version)).
		SetTaskID(t.TaskID).
		SetSessionID(t.SessionID).
		SetSpiritSessionID(t.SpiritSessionID).
		SetParentTurnID(t.ParentTurnID).
		SetAgentKey(t.AgentKey).
		SetTeamID(t.TeamID).
		SetTeamStageID(t.TeamStageID).
		SetSeq(t.Seq).
		SetStatus(string(t.Status)).
		SetVersion(t.Version)
	if t.CompletedAt != nil {
		b.SetCompletedAt(*t.CompletedAt)
	}
	if err := b.Exec(ctx); err == nil {
		row, getErr := r.data.RW().Read(ctx).TurnV2.Get(ctx, t.ID)
		if getErr != nil {
			return biz.Turn{}, entErrToBizErr(getErr, "TURN_V2")
		}
		return entTurnV2ToBiz(row), nil
	}
	// UPDATE failed. Two possible causes:
	//   1. Record doesn't exist yet → fall through to CREATE.
	//   2. Record exists but Version >= t.Version (WHERE didn't match) →
	//      return existing record (idempotent: a newer version is already
	//      persisted, e.g. sync persist wrote before the async event arrived).
	//      Without this check, the CREATE fallback would fail with CONFLICT
	//      and propagate an error to the v2 sequencer's retry loop.
	if existing, getErr := r.data.RW().Read(ctx).TurnV2.Get(ctx, t.ID); getErr == nil {
		return entTurnV2ToBiz(existing), nil
	}
	cb := r.data.RW().Write(ctx).TurnV2.Create().
		SetID(t.ID).
		SetTaskID(t.TaskID).
		SetSessionID(t.SessionID).
		SetSpiritSessionID(t.SpiritSessionID).
		SetParentTurnID(t.ParentTurnID).
		SetAgentKey(t.AgentKey).
		SetTeamID(t.TeamID).
		SetTeamStageID(t.TeamStageID).
		SetSeq(t.Seq).
		SetStatus(string(t.Status)).
		SetStartedAt(t.StartedAt).
		SetVersion(t.Version)
	if t.CompletedAt != nil {
		cb.SetCompletedAt(*t.CompletedAt)
	}
	row, err := cb.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) || isPgUniqueViolation(err) {
			existing, getErr := r.data.RW().Read(ctx).TurnV2.Get(ctx, t.ID)
			if getErr != nil {
				return biz.Turn{}, entErrToBizErr(getErr, "TURN_V2")
			}
			return entTurnV2ToBiz(existing), nil
		}
		return biz.Turn{}, entErrToBizErr(err, "TURN_V2")
	}
	return entTurnV2ToBiz(row), nil
}

// entTurnV2ToBiz converts an Ent TurnV2 row to biz.Turn.
func entTurnV2ToBiz(row *ent.TurnV2) biz.Turn {
	var completedAt *time.Time
	if row.CompletedAt != nil {
		t := *row.CompletedAt
		completedAt = &t
	}
	return biz.Turn{
		ID:              row.ID,
		TaskID:          row.TaskID,
		SessionID:       row.SessionID,
		SpiritSessionID: row.SpiritSessionID,
		ParentTurnID:    row.ParentTurnID,
		AgentKey:        row.AgentKey,
		TeamID:          row.TeamID,
		TeamStageID:     row.TeamStageID,
		Seq:             row.Seq,
		Version:         row.Version,
		Status:          biz.TurnStatus(row.Status),
		StartedAt:       row.StartedAt,
		CompletedAt:     completedAt,
	}
}

func entTurnsV2ToBiz(rows []*ent.TurnV2) []biz.Turn {
	out := make([]biz.Turn, 0, len(rows))
	for _, r := range rows {
		out = append(out, entTurnV2ToBiz(r))
	}
	return out
}
