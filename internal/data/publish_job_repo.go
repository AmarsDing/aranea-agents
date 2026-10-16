package data

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz/publish"
	"aranea-agents/internal/data/ent"
	entpj "aranea-agents/internal/data/ent/publishjob"
)

type publishJobRepo struct {
	data *Data
}

var _ publish.Repo = (*publishJobRepo)(nil)

// NewPublishJobRepo wires Ent-backed publish.Repo.
func NewPublishJobRepo(d *Data) publish.Repo {
	return &publishJobRepo{data: d}
}

func (r *publishJobRepo) client(ctx context.Context) *ent.Client {
	if ctx == nil {
		ctx = context.Background()
	}
	return r.data.RW().Write(ctx)
}

func (r *publishJobRepo) Create(j publish.Job) (publish.Job, error) {
	ctx := context.Background()
	row, err := r.client(ctx).PublishJob.Create().
		SetID(j.ID).
		SetReviewTaskID(j.ReviewTaskID).
		SetContentItemID(j.ContentItemID).
		SetPlatform(j.Platform).
		SetMode(j.Mode).
		SetStatus(j.Status).
		SetAttempt(j.Attempt).
		SetLastError(j.LastError).
		SetArtifactID(j.ArtifactID).
		SetPackageDir(j.PackageDir).
		SetIdempotencyKey(j.IdempotencyKey).
		SetTitle(j.Title).
		SetBody(j.Body).
		SetTagsJSON(mvpEncodeStrings(j.Tags)).
		SetMediaPathsJSON(mvpEncodeStrings(j.MediaPaths)).
		SetCreatedAt(mvpTimeString(j.CreatedAt)).
		SetUpdatedAt(mvpTimeString(j.UpdatedAt)).
		Save(ctx)
	if err != nil {
		return publish.Job{}, fmt.Errorf("publish: create: %w", err)
	}
	return entToPublishJob(row), nil
}

func (r *publishJobRepo) Get(id string) (publish.Job, error) {
	ctx := context.Background()
	row, err := r.client(ctx).PublishJob.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return publish.Job{}, fmt.Errorf("publish: not found %s", id)
		}
		return publish.Job{}, fmt.Errorf("publish: get: %w", err)
	}
	return entToPublishJob(row), nil
}

func (r *publishJobRepo) Update(j publish.Job) (publish.Job, error) {
	ctx := context.Background()
	row, err := r.client(ctx).PublishJob.UpdateOneID(j.ID).
		SetReviewTaskID(j.ReviewTaskID).
		SetContentItemID(j.ContentItemID).
		SetPlatform(j.Platform).
		SetMode(j.Mode).
		SetStatus(j.Status).
		SetAttempt(j.Attempt).
		SetLastError(j.LastError).
		SetArtifactID(j.ArtifactID).
		SetPackageDir(j.PackageDir).
		SetIdempotencyKey(j.IdempotencyKey).
		SetTitle(j.Title).
		SetBody(j.Body).
		SetTagsJSON(mvpEncodeStrings(j.Tags)).
		SetMediaPathsJSON(mvpEncodeStrings(j.MediaPaths)).
		SetUpdatedAt(mvpTimeString(j.UpdatedAt)).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return publish.Job{}, fmt.Errorf("publish: not found %s", j.ID)
		}
		return publish.Job{}, fmt.Errorf("publish: update: %w", err)
	}
	return entToPublishJob(row), nil
}

func (r *publishJobRepo) ListByStatus(status string, limit int) ([]publish.Job, error) {
	ctx := context.Background()
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.client(ctx).PublishJob.Query().
		Where(entpj.StatusEQ(status)).
		Order(ent.Asc(entpj.FieldCreatedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("publish: list: %w", err)
	}
	out := make([]publish.Job, 0, len(rows))
	for _, row := range rows {
		out = append(out, entToPublishJob(row))
	}
	return out, nil
}

func (r *publishJobRepo) GetByIdempotency(key string) (publish.Job, bool, error) {
	ctx := context.Background()
	row, err := r.client(ctx).PublishJob.Query().
		Where(entpj.IdempotencyKeyEQ(key)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return publish.Job{}, false, nil
		}
		return publish.Job{}, false, fmt.Errorf("publish: idempotency: %w", err)
	}
	return entToPublishJob(row), true, nil
}

func entToPublishJob(row *ent.PublishJob) publish.Job {
	return publish.Job{
		ID:             row.ID,
		ReviewTaskID:   row.ReviewTaskID,
		ContentItemID:  row.ContentItemID,
		Platform:       row.Platform,
		Mode:           row.Mode,
		Status:         row.Status,
		Attempt:        row.Attempt,
		LastError:      row.LastError,
		ArtifactID:     row.ArtifactID,
		PackageDir:     row.PackageDir,
		IdempotencyKey: row.IdempotencyKey,
		Title:          row.Title,
		Body:           row.Body,
		Tags:           mvpDecodeStrings(row.TagsJSON),
		MediaPaths:     mvpDecodeStrings(row.MediaPathsJSON),
		CreatedAt:      mvpParseTime(row.CreatedAt),
		UpdatedAt:      mvpParseTime(row.UpdatedAt),
	}
}
