package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/taskv2"
	"aranea-agents/pkg/loggateway"
)

// taskV2Repo implements biz.TaskV2Repo.
// Stability:evolving
type taskV2Repo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.TaskV2Repo = (*taskV2Repo)(nil)

// NewTaskV2Repo creates a new TaskV2Repo.
// Logger is preset with domain "TASK_V2" per loggateway convention.
func NewTaskV2Repo(d *Data, lg loggateway.Logger) biz.TaskV2Repo {
	return &taskV2Repo{data: d, lg: lg.With(loggateway.Domain("TASK_V2"))}
}

// GetTask returns the Task by ID.
func (r *taskV2Repo) GetTask(ctx context.Context, id string) (biz.Task, error) {
	if r == nil || r.data == nil {
		return biz.Task{}, fmt.Errorf("task v2 repo: database not configured")
	}
	row, err := r.data.RW().Read(ctx).TaskV2.Get(ctx, id)
	if err != nil {
		return biz.Task{}, entErrToBizErr(err, "TASK_V2")
	}
	return entTaskV2ToBiz(row), nil
}

// ListTasksBySession returns all tasks for the given session, ordered by seq asc.
func (r *taskV2Repo) ListTasksBySession(ctx context.Context, sessionID string) ([]biz.Task, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("task v2 repo: database not configured")
	}
	rows, err := r.data.RW().Read(ctx).TaskV2.Query().
		Where(taskv2.SessionIDEQ(sessionID)).
		Order(ent.Asc(taskv2.FieldSeq)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "TASK_V2")
	}
	return entTasksV2ToBiz(rows), nil
}

// CreateTask inserts a new Task with the caller's claimed Version.
func (r *taskV2Repo) CreateTask(ctx context.Context, t biz.Task) (biz.Task, error) {
	if r == nil || r.data == nil {
		return biz.Task{}, fmt.Errorf("task v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).TaskV2.Create().
		SetID(t.ID).
		SetSessionID(t.SessionID).
		SetUserMessage(t.UserMessage).
		SetStatus(string(t.Status)).
		SetSeq(t.Seq).
		SetVersion(t.Version).
		SetCreatedAt(t.CreatedAt).
		SetUpdatedAt(t.UpdatedAt)
	if t.CompletedAt != nil {
		b.SetCompletedAt(*t.CompletedAt)
	}
	row, err := b.Save(ctx)
	if err != nil {
		return biz.Task{}, entErrToBizErr(err, "TASK_V2")
	}
	return entTaskV2ToBiz(row), nil
}

// UpdateTask overwrites all mutable fields (no version guard).
// Use this only when the caller has already established ordering; for
// concurrent-safe updates prefer UpsertTask.
func (r *taskV2Repo) UpdateTask(ctx context.Context, t biz.Task) (biz.Task, error) {
	if r == nil || r.data == nil {
		return biz.Task{}, fmt.Errorf("task v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).TaskV2.UpdateOneID(t.ID).
		SetSessionID(t.SessionID).
		SetUserMessage(t.UserMessage).
		SetStatus(string(t.Status)).
		SetSeq(t.Seq).
		SetVersion(t.Version).
		SetUpdatedAt(t.UpdatedAt)
	if t.CompletedAt != nil {
		b.SetCompletedAt(*t.CompletedAt)
	}
	row, err := b.Save(ctx)
	if err != nil {
		return biz.Task{}, entErrToBizErr(err, "TASK_V2")
	}
	return entTaskV2ToBiz(row), nil
}

// UpsertTask applies optimistic-concurrency upsert: only succeeds if the
// stored version is older than t.Version. If the version guard fails (either
// no existing row, or stored version >= t.Version), falls back to Create; if
// Create hits a unique constraint (race), returns the existing row.
func (r *taskV2Repo) UpsertTask(ctx context.Context, t biz.Task) (biz.Task, error) {
	if r == nil || r.data == nil {
		return biz.Task{}, fmt.Errorf("task v2 repo: database not configured")
	}
	// 1) Try update if stored version is older than t.Version.
	b := r.data.RW().Write(ctx).TaskV2.UpdateOneID(t.ID).
		Where(taskv2.VersionLT(t.Version)).
		SetSessionID(t.SessionID).
		SetUserMessage(t.UserMessage).
		SetStatus(string(t.Status)).
		SetSeq(t.Seq).
		SetUpdatedAt(t.UpdatedAt).
		SetVersion(t.Version)
	if t.CompletedAt != nil {
		b.SetCompletedAt(*t.CompletedAt)
	}
	if err := b.Exec(ctx); err == nil {
		row, getErr := r.data.RW().Read(ctx).TaskV2.Get(ctx, t.ID)
		if getErr != nil {
			return biz.Task{}, entErrToBizErr(getErr, "TASK_V2")
		}
		return entTaskV2ToBiz(row), nil
	}
	// UPDATE failed. Two possible causes:
	//   1. Record doesn't exist yet → fall through to CREATE.
	//   2. Record exists but Version >= t.Version (WHERE didn't match) →
	//      return existing record (idempotent: a newer version is already
	//      persisted, e.g. sync persist wrote before the async event arrived).
	//      Without this check, the CREATE fallback would fail with CONFLICT
	//      and propagate an error to the v2 sequencer's retry loop.
	if existing, getErr := r.data.RW().Read(ctx).TaskV2.Get(ctx, t.ID); getErr == nil {
		return entTaskV2ToBiz(existing), nil
	}
	// 2) Insert if not exists (or version guard rejected the update).
	cb := r.data.RW().Write(ctx).TaskV2.Create().
		SetID(t.ID).
		SetSessionID(t.SessionID).
		SetUserMessage(t.UserMessage).
		SetStatus(string(t.Status)).
		SetSeq(t.Seq).
		SetVersion(t.Version).
		SetCreatedAt(t.CreatedAt).
		SetUpdatedAt(t.UpdatedAt)
	if t.CompletedAt != nil {
		cb.SetCompletedAt(*t.CompletedAt)
	}
	row, err := cb.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			// Race: another writer inserted first; return the winner.
			existing, getErr := r.data.RW().Read(ctx).TaskV2.Get(ctx, t.ID)
			if getErr != nil {
				return biz.Task{}, entErrToBizErr(getErr, "TASK_V2")
			}
			return entTaskV2ToBiz(existing), nil
		}
		return biz.Task{}, entErrToBizErr(err, "TASK_V2")
	}
	return entTaskV2ToBiz(row), nil
}

// entTaskV2ToBiz converts an Ent TaskV2 row to biz.Task.
func entTaskV2ToBiz(row *ent.TaskV2) biz.Task {
	var completedAt *time.Time
	if row.CompletedAt != nil {
		t := *row.CompletedAt
		completedAt = &t
	}
	return biz.Task{
		ID:          row.ID,
		SessionID:   row.SessionID,
		UserMessage: row.UserMessage,
		Status:      biz.TaskStatus(row.Status),
		Seq:         row.Seq,
		Version:     row.Version,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
		CompletedAt: completedAt,
	}
}

func entTasksV2ToBiz(rows []*ent.TaskV2) []biz.Task {
	out := make([]biz.Task, 0, len(rows))
	for _, r := range rows {
		out = append(out, entTaskV2ToBiz(r))
	}
	return out
}
