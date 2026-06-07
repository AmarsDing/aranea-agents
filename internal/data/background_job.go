package data

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz/backgroundjob"
	bizbackgroundjob "aranea-agents/internal/biz/backgroundjob"
	"aranea-agents/internal/data/ent"
	entbgjob "aranea-agents/internal/data/ent/backgroundjob"

	"github.com/google/uuid"
)

type backgroundJobRepo struct {
	data *Data
}

var _ bizbackgroundjob.Repo = (*backgroundJobRepo)(nil)

// NewBackgroundJobRepo wires the data-layer implementation of backgroundjob.Repo.
func NewBackgroundJobRepo(d *Data) backgroundjob.Repo {
	return &backgroundJobRepo{data: d}
}

func (r *backgroundJobRepo) Create(ctx context.Context, req backgroundjob.CreateRequest) (*backgroundjob.Job, error) {
	priority := req.Priority
	if priority <= 0 {
		priority = backgroundjob.PriorityNormal
	}
	maxAttempts := req.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	payload := req.Payload
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	var schedMS int64
	if !req.ScheduledAt.IsZero() {
		schedMS = req.ScheduledAt.UnixMilli()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	row, err := r.data.RW().Write(ctx).BackgroundJob.Create().
		SetID(uuid.NewString()).
		SetKind(req.Kind).
		SetOwnerType(string(req.OwnerType)).
		SetOwnerID(req.OwnerID).
		SetParentJobID(req.ParentJobID).
		SetPriority(priority).
		SetStatus(string(backgroundjob.StatusQueued)).
		SetPayload(payload).
		SetMaxAttempts(maxAttempts).
		SetScheduledAt(schedMS).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("backgroundjob create: %w", err)
	}
	return entToJob(row), nil
}

func (r *backgroundJobRepo) Get(ctx context.Context, id string) (*backgroundjob.Job, error) {
	row, err := r.data.RW().Read(ctx).BackgroundJob.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("backgroundjob %s: %w", id, backgroundjob.ErrNotFound)
		}
		return nil, fmt.Errorf("backgroundjob get %s: %w", id, err)
	}
	return entToJob(row), nil
}

func (r *backgroundJobRepo) List(ctx context.Context, f backgroundjob.ListFilter) ([]*backgroundjob.Job, error) {
	q := r.data.RW().Read(ctx).BackgroundJob.Query().
		Order(ent.Asc(entbgjob.FieldPriority), ent.Asc(entbgjob.FieldCreatedAt))
	if f.OwnerType != "" {
		q = q.Where(entbgjob.OwnerTypeEQ(string(f.OwnerType)))
	}
	if f.OwnerID != "" {
		q = q.Where(entbgjob.OwnerIDEQ(f.OwnerID))
	}
	if f.Kind != "" {
		q = q.Where(entbgjob.KindEQ(f.Kind))
	}
	if f.Status != "" {
		q = q.Where(entbgjob.StatusEQ(string(f.Status)))
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	q = q.Limit(limit).Offset(f.Offset)
	rows, err := q.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("backgroundjob list: %w", err)
	}
	jobs := make([]*backgroundjob.Job, 0, len(rows))
	for _, row := range rows {
		jobs = append(jobs, entToJob(row))
	}
	return jobs, nil
}

// TryClaim atomically claims the next available job for workerID.
// Claimable: status='queued', scheduled_at <= now, attempts < max_attempts,
// and (parent_job_id is empty OR parent job status='succeeded').
//
// Note: SQLite does not support SELECT FOR UPDATE. We rely on the application-level
// claim: we SELECT the candidate, then UPDATE WHERE status='queued' to detect races.
func (r *backgroundJobRepo) TryClaim(ctx context.Context, workerID string, kinds []string) (*backgroundjob.Job, error) {
	nowMS := time.Now().UnixMilli()
	q := r.data.RW().Read(ctx).BackgroundJob.Query().
		Where(
			entbgjob.StatusEQ(string(backgroundjob.StatusQueued)),
			entbgjob.ScheduledAtLTE(nowMS),
		).
		Order(ent.Asc(entbgjob.FieldPriority), ent.Asc(entbgjob.FieldCreatedAt)).
		Limit(5) // read a small batch to reduce contention
	if len(kinds) > 0 {
		q = q.Where(entbgjob.KindIn(kinds...))
	}
	candidates, err := q.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("backgroundjob TryClaim query: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	nowMS = time.Now().UnixMilli()
	for _, cand := range candidates {
		if cand.Attempts >= cand.MaxAttempts {
			continue
		}
		// Skip jobs whose parent has not yet succeeded.
		if cand.ParentJobID != "" {
			parent, perr := r.data.RW().Read(ctx).BackgroundJob.Get(ctx, cand.ParentJobID)
			if perr != nil || parent.Status != string(backgroundjob.StatusSucceeded) {
				continue
			}
		}
		// Optimistic claim: UPDATE WHERE status='queued'.
		n, uerr := r.data.RW().Write(ctx).BackgroundJob.Update().
			Where(entbgjob.IDEQ(cand.ID), entbgjob.StatusEQ(string(backgroundjob.StatusQueued))).
			SetStatus(string(backgroundjob.StatusClaimed)).
			SetWorkerID(workerID).
			SetAttempts(cand.Attempts + 1).
			SetClaimedAt(nowMS).
			SetUpdatedAt(now).
			Save(ctx)
		if uerr != nil {
			continue
		}
		if n == 0 {
			continue // another worker won the race
		}
		// Re-read to get the fresh state.
		fresh, gerr := r.data.RW().Read(ctx).BackgroundJob.Get(ctx, cand.ID)
		if gerr != nil {
			return nil, fmt.Errorf("backgroundjob TryClaim re-read: %w", gerr)
		}
		return entToJob(fresh), nil
	}
	return nil, nil
}

func (r *backgroundJobRepo) MarkRunning(ctx context.Context, id, workerID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.data.RW().Write(ctx).BackgroundJob.UpdateOneID(id).
		SetStatus(string(backgroundjob.StatusClaimed)).
		SetWorkerID(workerID).
		SetUpdatedAt(now).
		Save(ctx)
	return wrapBGJobErr(err, "MarkRunning", id)
}

func (r *backgroundJobRepo) MarkSucceeded(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	nowMS := time.Now().UnixMilli()
	_, err := r.data.RW().Write(ctx).BackgroundJob.UpdateOneID(id).
		SetStatus(string(backgroundjob.StatusSucceeded)).
		SetFinishedAt(nowMS).
		SetUpdatedAt(now).
		Save(ctx)
	return wrapBGJobErr(err, "MarkSucceeded", id)
}

func (r *backgroundJobRepo) MarkFailed(ctx context.Context, id, errMsg string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	nowMS := time.Now().UnixMilli()
	if len(errMsg) > 1024 {
		errMsg = errMsg[:1024]
	}
	_, err := r.data.RW().Write(ctx).BackgroundJob.UpdateOneID(id).
		SetStatus(string(backgroundjob.StatusFailed)).
		SetLastError(errMsg).
		SetFinishedAt(nowMS).
		SetUpdatedAt(now).
		Save(ctx)
	return wrapBGJobErr(err, "MarkFailed", id)
}

func (r *backgroundJobRepo) Cancel(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	nowMS := time.Now().UnixMilli()
	n, err := r.data.RW().Write(ctx).BackgroundJob.Update().
		Where(
			entbgjob.IDEQ(id),
			entbgjob.StatusIn(
				string(backgroundjob.StatusQueued),
				string(backgroundjob.StatusClaimed),
			),
		).
		SetStatus(string(backgroundjob.StatusCancelled)).
		SetFinishedAt(nowMS).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("backgroundjob cancel %s: %w", id, err)
	}
	_ = n // affected rows not needed
	return nil
}

func (r *backgroundJobRepo) CancelByOwner(ctx context.Context, ownerType backgroundjob.OwnerType, ownerID string) (int, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	nowMS := time.Now().UnixMilli()
	n, err := r.data.RW().Write(ctx).BackgroundJob.Update().
		Where(
			entbgjob.OwnerTypeEQ(string(ownerType)),
			entbgjob.OwnerIDEQ(ownerID),
			entbgjob.StatusIn(
				string(backgroundjob.StatusQueued),
				string(backgroundjob.StatusClaimed),
			),
		).
		SetStatus(string(backgroundjob.StatusCancelled)).
		SetFinishedAt(nowMS).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return 0, fmt.Errorf("backgroundjob CancelByOwner %s/%s: %w", ownerType, ownerID, err)
	}
	return n, nil
}

func (r *backgroundJobRepo) DeleteTerminated(ctx context.Context, f backgroundjob.ListFilter) (int, error) {
	cutoffMS := time.Now().Add(-48 * time.Hour).UnixMilli()
	q := r.data.RW().Write(ctx).BackgroundJob.Delete().
		Where(
			entbgjob.StatusIn(
				string(backgroundjob.StatusSucceeded),
				string(backgroundjob.StatusFailed),
				string(backgroundjob.StatusCancelled),
			),
			entbgjob.FinishedAtLTE(cutoffMS),
		)
	if f.OwnerType != "" {
		q = q.Where(entbgjob.OwnerTypeEQ(string(f.OwnerType)))
	}
	n, err := q.Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("backgroundjob DeleteTerminated: %w", err)
	}
	return n, nil
}

// entToJob converts an Ent row to the domain type.
func entToJob(row *ent.BackgroundJob) *backgroundjob.Job {
	return &backgroundjob.Job{
		ID:          row.ID,
		Kind:        row.Kind,
		OwnerType:   backgroundjob.OwnerType(row.OwnerType),
		OwnerID:     row.OwnerID,
		ParentJobID: row.ParentJobID,
		Priority:    row.Priority,
		Status:      backgroundjob.Status(row.Status),
		Payload:     row.Payload,
		WorkerID:    row.WorkerID,
		Attempts:    row.Attempts,
		MaxAttempts: row.MaxAttempts,
		LastError:   row.LastError,
		ScheduledAt: msToTime(row.ScheduledAt),
		ClaimedAt:   msToTime(row.ClaimedAt),
		FinishedAt:  msToTime(row.FinishedAt),
		CreatedAt:   parseRFC3339(row.CreatedAt),
		UpdatedAt:   parseRFC3339(row.UpdatedAt),
	}
}

func msToTime(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

func parseRFC3339(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

func wrapBGJobErr(err error, op, id string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("backgroundjob %s %s: %w", op, id, err)
}
