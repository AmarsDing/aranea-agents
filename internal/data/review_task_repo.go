package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz/review"
	"aranea-agents/internal/data/ent"
	entrt "aranea-agents/internal/data/ent/reviewtask"
)

type reviewTaskRepo struct {
	data *Data
}

var _ review.Repo = (*reviewTaskRepo)(nil)

// NewReviewTaskRepo wires Ent-backed review.Repo.
func NewReviewTaskRepo(d *Data) review.Repo {
	return &reviewTaskRepo{data: d}
}

func (r *reviewTaskRepo) client(ctx context.Context) *ent.Client {
	if ctx == nil {
		ctx = context.Background()
	}
	return r.data.RW().Write(ctx)
}

func (r *reviewTaskRepo) Create(t review.Task) (review.Task, error) {
	ctx := context.Background()
	decided := ""
	if t.DecidedAt != nil {
		decided = mvpTimeString(*t.DecidedAt)
	}
	row, err := r.client(ctx).ReviewTask.Create().
		SetID(t.ID).
		SetContentItemID(t.ContentItemID).
		SetGraphExecutionID(t.GraphExecutionID).
		SetGraphNodeID(t.GraphNodeID).
		SetGraphTaskID(t.GraphTaskID).
		SetDraftArtifactID(t.DraftArtifactID).
		SetPlatformTargetsJSON(mvpEncodeStrings(t.PlatformTargets)).
		SetMachineFlagsJSON(mvpEncodeStrings(t.MachineFlags)).
		SetDraftTitle(t.DraftTitle).
		SetDraftBody(t.DraftBody).
		SetStatus(t.Status).
		SetDecisionNote(t.DecisionNote).
		SetReviewerID(t.ReviewerID).
		SetCreatorID(t.CreatorID).
		SetDecidedAt(decided).
		SetCreatedAt(mvpTimeString(t.CreatedAt)).
		SetUpdatedAt(mvpTimeString(t.UpdatedAt)).
		Save(ctx)
	if err != nil {
		return review.Task{}, fmt.Errorf("review: create: %w", err)
	}
	return entToReviewTask(row), nil
}

func (r *reviewTaskRepo) Get(id string) (review.Task, error) {
	ctx := context.Background()
	row, err := r.client(ctx).ReviewTask.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return review.Task{}, fmt.Errorf("%w: %s", review.ErrNotFound, id)
		}
		return review.Task{}, fmt.Errorf("review: get: %w", err)
	}
	return entToReviewTask(row), nil
}

func (r *reviewTaskRepo) Update(t review.Task) (review.Task, error) {
	ctx := context.Background()
	decided := ""
	if t.DecidedAt != nil {
		decided = mvpTimeString(*t.DecidedAt)
	}
	row, err := r.client(ctx).ReviewTask.UpdateOneID(t.ID).
		SetContentItemID(t.ContentItemID).
		SetGraphExecutionID(t.GraphExecutionID).
		SetGraphNodeID(t.GraphNodeID).
		SetGraphTaskID(t.GraphTaskID).
		SetDraftArtifactID(t.DraftArtifactID).
		SetPlatformTargetsJSON(mvpEncodeStrings(t.PlatformTargets)).
		SetMachineFlagsJSON(mvpEncodeStrings(t.MachineFlags)).
		SetDraftTitle(t.DraftTitle).
		SetDraftBody(t.DraftBody).
		SetStatus(t.Status).
		SetDecisionNote(t.DecisionNote).
		SetReviewerID(t.ReviewerID).
		SetCreatorID(t.CreatorID).
		SetDecidedAt(decided).
		SetUpdatedAt(mvpTimeString(t.UpdatedAt)).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return review.Task{}, fmt.Errorf("%w: %s", review.ErrNotFound, t.ID)
		}
		return review.Task{}, fmt.Errorf("review: update: %w", err)
	}
	return entToReviewTask(row), nil
}

func (r *reviewTaskRepo) ListPending(limit, offset int) ([]review.Task, int, error) {
	ctx := context.Background()
	q := r.client(ctx).ReviewTask.Query().Where(entrt.StatusEQ(review.StatusPending))
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("review: count: %w", err)
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := q.Offset(offset).Limit(limit).Order(ent.Asc(entrt.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("review: list: %w", err)
	}
	out := make([]review.Task, 0, len(rows))
	for _, row := range rows {
		out = append(out, entToReviewTask(row))
	}
	return out, total, nil
}

// Decide 以 CAS 方式落决议（E 阶段 W1b，需求文档 §5.3 验收 3）：仅当当前
// 状态等于 from 时更新——并发决议恰一个成功，0 行命中时区分 not found 与
// review.ErrConflict（状态已被并发推进）。
func (r *reviewTaskRepo) Decide(id, from, to, reviewerID, note string, decidedAt time.Time) (review.Task, error) {
	ctx := context.Background()
	n, err := r.client(ctx).ReviewTask.Update().
		Where(entrt.ID(id), entrt.StatusEQ(from)).
		SetStatus(to).
		SetReviewerID(reviewerID).
		SetDecisionNote(note).
		SetDecidedAt(mvpTimeString(decidedAt)).
		SetUpdatedAt(mvpTimeString(decidedAt)).
		Save(ctx)
	if err != nil {
		return review.Task{}, fmt.Errorf("review: decide: %w", err)
	}
	if n == 0 {
		if _, getErr := r.Get(id); getErr != nil {
			return review.Task{}, getErr
		}
		return review.Task{}, review.ErrConflict
	}
	return r.Get(id)
}

func (r *reviewTaskRepo) GetByGraphExecution(executionID, nodeID string) (review.Task, bool, error) {	ctx := context.Background()
	q := r.client(ctx).ReviewTask.Query().Where(entrt.GraphExecutionIDEQ(executionID))
	if nodeID != "" {
		q = q.Where(entrt.GraphNodeIDEQ(nodeID))
	}
	row, err := q.Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return review.Task{}, false, nil
		}
		return review.Task{}, false, fmt.Errorf("review: by graph: %w", err)
	}
	return entToReviewTask(row), true, nil
}

func entToReviewTask(row *ent.ReviewTask) review.Task {
	return review.Task{
		ID:               row.ID,
		ContentItemID:    row.ContentItemID,
		GraphExecutionID: row.GraphExecutionID,
		GraphNodeID:      row.GraphNodeID,
		GraphTaskID:      row.GraphTaskID,
		DraftArtifactID:  row.DraftArtifactID,
		PlatformTargets:  mvpDecodeStrings(row.PlatformTargetsJSON),
		MachineFlags:     mvpDecodeStrings(row.MachineFlagsJSON),
		DraftTitle:       row.DraftTitle,
		DraftBody:        row.DraftBody,
		Status:           row.Status,
		DecisionNote:     row.DecisionNote,
		ReviewerID:       row.ReviewerID,
		CreatorID:        row.CreatorID,
		DecidedAt:        mvpParseTimePtr(row.DecidedAt),
		CreatedAt:        mvpParseTime(row.CreatedAt),
		UpdatedAt:        mvpParseTime(row.UpdatedAt),
	}
}
