package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/planboardv2"
	"aranea-agents/pkg/loggateway"
)

// planBoardV2Repo implements biz.PlanBoardV2Repo.
// Stability:evolving
//
// NOTE: PlanBoard.Steps is an in-memory field only; it is NOT persisted to the
// plan_boards_v2 table (the Ent schema has no steps column). Steps are loaded
// separately via PlanStepV2Repo.ListPlanStepsByPlan when needed.
type planBoardV2Repo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.PlanBoardV2Repo = (*planBoardV2Repo)(nil)

// NewPlanBoardV2Repo creates a new PlanBoardV2Repo.
// Logger is preset with domain "PLAN_BOARD_V2" per loggateway convention.
func NewPlanBoardV2Repo(d *Data, lg loggateway.Logger) biz.PlanBoardV2Repo {
	return &planBoardV2Repo{data: d, lg: lg.With(loggateway.Domain("PLAN_BOARD_V2"))}
}

// GetPlanBoard returns the PlanBoard by ID.
// Steps is intentionally left empty (loaded via PlanStepV2Repo separately).
func (r *planBoardV2Repo) GetPlanBoard(ctx context.Context, id string) (biz.PlanBoard, error) {
	if r == nil || r.data == nil {
		return biz.PlanBoard{}, fmt.Errorf("plan board v2 repo: database not configured")
	}
	row, err := r.data.RW().Read(ctx).PlanBoardV2.Get(ctx, id)
	if err != nil {
		return biz.PlanBoard{}, entErrToBizErr(err, "PLAN_BOARD_V2")
	}
	return entPlanBoardV2ToBiz(row), nil
}

// ListPlanBoardsByTask returns all plan boards for the given task, ordered by seq asc.
func (r *planBoardV2Repo) ListPlanBoardsByTask(ctx context.Context, taskID string) ([]biz.PlanBoard, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("plan board v2 repo: database not configured")
	}
	rows, err := r.data.RW().Read(ctx).PlanBoardV2.Query().
		Where(planboardv2.TaskIDEQ(taskID)).
		Order(ent.Asc(planboardv2.FieldSeq)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "PLAN_BOARD_V2")
	}
	return entPlanBoardsV2ToBiz(rows), nil
}

// CreatePlanBoard inserts a new PlanBoard with the caller's claimed Version.
// Steps is NOT persisted (in-memory only).
func (r *planBoardV2Repo) CreatePlanBoard(ctx context.Context, pb biz.PlanBoard) (biz.PlanBoard, error) {
	if r == nil || r.data == nil {
		return biz.PlanBoard{}, fmt.Errorf("plan board v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).PlanBoardV2.Create().
		SetID(pb.ID).
		SetTaskID(pb.TaskID).
		SetTurnID(pb.TurnID).
		SetSessionID(pb.SessionID).
		SetStrategy(string(pb.Strategy)).
		SetStatus(string(pb.Status)).
		SetStartedAt(pb.StartedAt).
		SetSeq(pb.Seq).
		SetVersion(pb.Version)
	if pb.CompletedAt != nil {
		b.SetCompletedAt(*pb.CompletedAt)
	}
	row, err := b.Save(ctx)
	if err != nil {
		return biz.PlanBoard{}, entErrToBizErr(err, "PLAN_BOARD_V2")
	}
	return entPlanBoardV2ToBiz(row), nil
}

// UpdatePlanBoard patches mutable fields without version guard.
// Steps is NOT persisted (in-memory only).
// Use UpsertPlanBoard for concurrent-safe writes.
func (r *planBoardV2Repo) UpdatePlanBoard(ctx context.Context, pb biz.PlanBoard) (biz.PlanBoard, error) {
	if r == nil || r.data == nil {
		return biz.PlanBoard{}, fmt.Errorf("plan board v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).PlanBoardV2.UpdateOneID(pb.ID).
		SetTaskID(pb.TaskID).
		SetTurnID(pb.TurnID).
		SetSessionID(pb.SessionID).
		SetStrategy(string(pb.Strategy)).
		SetStatus(string(pb.Status)).
		SetSeq(pb.Seq).
		SetVersion(pb.Version)
	if pb.CompletedAt != nil {
		b.SetCompletedAt(*pb.CompletedAt)
	}
	row, err := b.Save(ctx)
	if err != nil {
		return biz.PlanBoard{}, entErrToBizErr(err, "PLAN_BOARD_V2")
	}
	return entPlanBoardV2ToBiz(row), nil
}

// UpsertPlanBoard applies optimistic-concurrency upsert (see UpsertTask for semantics).
// Steps is NOT persisted (in-memory only).
func (r *planBoardV2Repo) UpsertPlanBoard(ctx context.Context, pb biz.PlanBoard) (biz.PlanBoard, error) {
	if r == nil || r.data == nil {
		return biz.PlanBoard{}, fmt.Errorf("plan board v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).PlanBoardV2.UpdateOneID(pb.ID).
		Where(planboardv2.VersionLT(pb.Version)).
		SetTaskID(pb.TaskID).
		SetTurnID(pb.TurnID).
		SetSessionID(pb.SessionID).
		SetStrategy(string(pb.Strategy)).
		SetStatus(string(pb.Status)).
		SetSeq(pb.Seq).
		SetVersion(pb.Version)
	if pb.CompletedAt != nil {
		b.SetCompletedAt(*pb.CompletedAt)
	}
	if err := b.Exec(ctx); err == nil {
		row, getErr := r.data.RW().Read(ctx).PlanBoardV2.Get(ctx, pb.ID)
		if getErr != nil {
			return biz.PlanBoard{}, entErrToBizErr(getErr, "PLAN_BOARD_V2")
		}
		return entPlanBoardV2ToBiz(row), nil
	}
	cb := r.data.RW().Write(ctx).PlanBoardV2.Create().
		SetID(pb.ID).
		SetTaskID(pb.TaskID).
		SetTurnID(pb.TurnID).
		SetSessionID(pb.SessionID).
		SetStrategy(string(pb.Strategy)).
		SetStatus(string(pb.Status)).
		SetStartedAt(pb.StartedAt).
		SetSeq(pb.Seq).
		SetVersion(pb.Version)
	if pb.CompletedAt != nil {
		cb.SetCompletedAt(*pb.CompletedAt)
	}
	row, err := cb.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			existing, getErr := r.data.RW().Read(ctx).PlanBoardV2.Get(ctx, pb.ID)
			if getErr != nil {
				return biz.PlanBoard{}, entErrToBizErr(getErr, "PLAN_BOARD_V2")
			}
			return entPlanBoardV2ToBiz(existing), nil
		}
		return biz.PlanBoard{}, entErrToBizErr(err, "PLAN_BOARD_V2")
	}
	return entPlanBoardV2ToBiz(row), nil
}

// entPlanBoardV2ToBiz converts an Ent PlanBoardV2 row to biz.PlanBoard.
// Steps is intentionally left nil — it is an in-memory field loaded separately.
func entPlanBoardV2ToBiz(row *ent.PlanBoardV2) biz.PlanBoard {
	var completedAt *time.Time
	if row.CompletedAt != nil {
		t := *row.CompletedAt
		completedAt = &t
	}
	return biz.PlanBoard{
		ID:          row.ID,
		TaskID:      row.TaskID,
		TurnID:      row.TurnID,
		SessionID:   row.SessionID,
		Strategy:    biz.PlanStrategy(row.Strategy),
		Status:      biz.PlanStatus(row.Status),
		StartedAt:   row.StartedAt,
		CompletedAt: completedAt,
		Seq:         row.Seq,
		Version:     row.Version,
		// Steps intentionally not set: in-memory only, loaded via PlanStepV2Repo
	}
}

func entPlanBoardsV2ToBiz(rows []*ent.PlanBoardV2) []biz.PlanBoard {
	out := make([]biz.PlanBoard, 0, len(rows))
	for _, r := range rows {
		out = append(out, entPlanBoardV2ToBiz(r))
	}
	return out
}
