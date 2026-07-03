package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/graphstagev2"
	"aranea-agents/pkg/loggateway"
)

// graphStageV2Repo implements biz.GraphStageV2Repo.
// Stability:evolving
//
// GraphStage.Nodes is an in-memory field only; it is NOT persisted to the
// graph_stages_v2 table. Nodes are loaded separately via GraphNodeV2Repo.
type graphStageV2Repo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.GraphStageV2Repo = (*graphStageV2Repo)(nil)

// NewGraphStageV2Repo creates a new GraphStageV2Repo.
// Logger is preset with domain "GRAPH_STAGE_V2" per loggateway convention.
func NewGraphStageV2Repo(d *Data, lg loggateway.Logger) biz.GraphStageV2Repo {
	return &graphStageV2Repo{data: d, lg: lg.With(loggateway.Domain("GRAPH_STAGE_V2"))}
}

// GetGraphStage returns the GraphStage by ID.
// Nodes is intentionally left empty (loaded via GraphNodeV2Repo separately).
func (r *graphStageV2Repo) GetGraphStage(ctx context.Context, id string) (biz.GraphStage, error) {
	if r == nil || r.data == nil {
		return biz.GraphStage{}, fmt.Errorf("graph stage v2 repo: database not configured")
	}
	row, err := r.data.RW().Read(ctx).GraphStageV2.Get(ctx, id)
	if err != nil {
		return biz.GraphStage{}, entErrToBizErr(err, "GRAPH_STAGE_V2")
	}
	return entGraphStageV2ToBiz(row), nil
}

// ListGraphStagesByTask returns all graph stages for the given task, ordered by seq asc.
func (r *graphStageV2Repo) ListGraphStagesByTask(ctx context.Context, taskID string) ([]biz.GraphStage, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("graph stage v2 repo: database not configured")
	}
	rows, err := r.data.RW().Read(ctx).GraphStageV2.Query().
		Where(graphstagev2.TaskIDEQ(taskID)).
		Order(ent.Asc(graphstagev2.FieldSeq)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "GRAPH_STAGE_V2")
	}
	return entGraphStagesV2ToBiz(rows), nil
}

// GetGraphStageByPlanBoard returns the GraphStage associated with the given PlanBoard ID.
// Returns CodeNotFound if no graph stage is associated (one-to-one, but plan may not have graph yet).
func (r *graphStageV2Repo) GetGraphStageByPlanBoard(ctx context.Context, planBoardID string) (biz.GraphStage, error) {
	if r == nil || r.data == nil {
		return biz.GraphStage{}, fmt.Errorf("graph stage v2 repo: database not configured")
	}
	row, err := r.data.RW().Read(ctx).GraphStageV2.Query().
		Where(graphstagev2.PlanBoardIDEQ(planBoardID)).
		Only(ctx)
	if err != nil {
		return biz.GraphStage{}, entErrToBizErr(err, "GRAPH_STAGE_V2")
	}
	return entGraphStageV2ToBiz(row), nil
}

// CreateGraphStage inserts a new GraphStage with the caller's claimed Version.
// Nodes is NOT persisted (in-memory only).
func (r *graphStageV2Repo) CreateGraphStage(ctx context.Context, gs biz.GraphStage) (biz.GraphStage, error) {
	if r == nil || r.data == nil {
		return biz.GraphStage{}, fmt.Errorf("graph stage v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).GraphStageV2.Create().
		SetID(gs.ID).
		SetTaskID(gs.TaskID).
		SetTurnID(gs.TurnID).
		SetSessionID(gs.SessionID).
		SetPlanBoardID(gs.PlanBoardID).
		SetStatus(string(gs.Status)).
		SetStartedAt(gs.StartedAt).
		SetSeq(gs.Seq).
		SetVersion(gs.Version)
	if gs.CompletedAt != nil {
		b.SetCompletedAt(*gs.CompletedAt)
	}
	row, err := b.Save(ctx)
	if err != nil {
		return biz.GraphStage{}, entErrToBizErr(err, "GRAPH_STAGE_V2")
	}
	return entGraphStageV2ToBiz(row), nil
}

// UpdateGraphStage patches mutable fields without version guard.
// Nodes is NOT persisted (in-memory only).
// Use UpsertGraphStage for concurrent-safe writes.
func (r *graphStageV2Repo) UpdateGraphStage(ctx context.Context, gs biz.GraphStage) (biz.GraphStage, error) {
	if r == nil || r.data == nil {
		return biz.GraphStage{}, fmt.Errorf("graph stage v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).GraphStageV2.UpdateOneID(gs.ID).
		SetTaskID(gs.TaskID).
		SetTurnID(gs.TurnID).
		SetSessionID(gs.SessionID).
		SetPlanBoardID(gs.PlanBoardID).
		SetStatus(string(gs.Status)).
		SetSeq(gs.Seq).
		SetVersion(gs.Version)
	if gs.CompletedAt != nil {
		b.SetCompletedAt(*gs.CompletedAt)
	}
	row, err := b.Save(ctx)
	if err != nil {
		return biz.GraphStage{}, entErrToBizErr(err, "GRAPH_STAGE_V2")
	}
	return entGraphStageV2ToBiz(row), nil
}

// UpsertGraphStage applies optimistic-concurrency upsert (see UpsertTask for semantics).
// Nodes is NOT persisted (in-memory only).
func (r *graphStageV2Repo) UpsertGraphStage(ctx context.Context, gs biz.GraphStage) (biz.GraphStage, error) {
	if r == nil || r.data == nil {
		return biz.GraphStage{}, fmt.Errorf("graph stage v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).GraphStageV2.UpdateOneID(gs.ID).
		Where(graphstagev2.VersionLT(gs.Version)).
		SetTaskID(gs.TaskID).
		SetTurnID(gs.TurnID).
		SetSessionID(gs.SessionID).
		SetPlanBoardID(gs.PlanBoardID).
		SetStatus(string(gs.Status)).
		SetSeq(gs.Seq).
		SetVersion(gs.Version)
	if gs.CompletedAt != nil {
		b.SetCompletedAt(*gs.CompletedAt)
	}
	if err := b.Exec(ctx); err == nil {
		row, getErr := r.data.RW().Read(ctx).GraphStageV2.Get(ctx, gs.ID)
		if getErr != nil {
			return biz.GraphStage{}, entErrToBizErr(getErr, "GRAPH_STAGE_V2")
		}
		return entGraphStageV2ToBiz(row), nil
	}
	cb := r.data.RW().Write(ctx).GraphStageV2.Create().
		SetID(gs.ID).
		SetTaskID(gs.TaskID).
		SetTurnID(gs.TurnID).
		SetSessionID(gs.SessionID).
		SetPlanBoardID(gs.PlanBoardID).
		SetStatus(string(gs.Status)).
		SetStartedAt(gs.StartedAt).
		SetSeq(gs.Seq).
		SetVersion(gs.Version)
	if gs.CompletedAt != nil {
		cb.SetCompletedAt(*gs.CompletedAt)
	}
	row, err := cb.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			existing, getErr := r.data.RW().Read(ctx).GraphStageV2.Get(ctx, gs.ID)
			if getErr != nil {
				return biz.GraphStage{}, entErrToBizErr(getErr, "GRAPH_STAGE_V2")
			}
			return entGraphStageV2ToBiz(existing), nil
		}
		return biz.GraphStage{}, entErrToBizErr(err, "GRAPH_STAGE_V2")
	}
	return entGraphStageV2ToBiz(row), nil
}

// entGraphStageV2ToBiz converts an Ent GraphStageV2 row to biz.GraphStage.
// Nodes is intentionally left nil — it is an in-memory field loaded separately.
func entGraphStageV2ToBiz(row *ent.GraphStageV2) biz.GraphStage {
	var completedAt *time.Time
	if row.CompletedAt != nil {
		t := *row.CompletedAt
		completedAt = &t
	}
	return biz.GraphStage{
		ID:          row.ID,
		TaskID:      row.TaskID,
		TurnID:      row.TurnID,
		SessionID:   row.SessionID,
		PlanBoardID: row.PlanBoardID,
		Status:      biz.GraphStageStatus(row.Status),
		StartedAt:   row.StartedAt,
		CompletedAt: completedAt,
		Seq:         row.Seq,
		Version:     row.Version,
		// Nodes intentionally not set: in-memory only, loaded via GraphNodeV2Repo
	}
}

func entGraphStagesV2ToBiz(rows []*ent.GraphStageV2) []biz.GraphStage {
	out := make([]biz.GraphStage, 0, len(rows))
	for _, r := range rows {
		out = append(out, entGraphStageV2ToBiz(r))
	}
	return out
}
