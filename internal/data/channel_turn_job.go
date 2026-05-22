package data

import (
	"context"
	"database/sql"
	"strings"

	"aranea-agents/internal/biz"
)

type channelTurnJobRepo struct {
	data *Data
}

// NewChannelTurnJobRepo implements biz.ChannelTurnJobRepo.
func NewChannelTurnJobRepo(d *Data) biz.ChannelTurnJobRepo {
	return &channelTurnJobRepo{data: d}
}

func (r *channelTurnJobRepo) Create(ctx context.Context, job biz.ChannelTurnJob) (string, error) {
	db := r.data.RawDB()
	if db == nil {
		return strings.TrimSpace(job.ID), nil
	}
	channelID := strings.TrimSpace(job.ChannelID)
	idempotency := strings.TrimSpace(job.IdempotencyKey)
	_, err := db.ExecContext(ctx, `
INSERT INTO channel_turn_job (
  id, channel_id, session_id, peer_id, peer_key, idempotency_key, status,
  preview_message_id, content_preview, async_target_type, async_target_id,
  error_message, started_at, finished_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(channel_id, idempotency_key) DO UPDATE SET
  updated_at=excluded.updated_at,
  status=CASE WHEN channel_turn_job.status IN ('completed','failed','timeout','cancelled','queued','async_queued')
    THEN channel_turn_job.status ELSE excluded.status END`,
		strings.TrimSpace(job.ID),
		channelID,
		strings.TrimSpace(job.SessionID),
		strings.TrimSpace(job.PeerID),
		strings.TrimSpace(job.PeerKey),
		idempotency,
		biz.NormalizeChannelTurnJobStatus(job.Status),
		strings.TrimSpace(job.PreviewMessageID),
		strings.TrimSpace(job.ContentPreview),
		strings.TrimSpace(job.AsyncTargetType),
		strings.TrimSpace(job.AsyncTargetID),
		strings.TrimSpace(job.ErrorMessage),
		strings.TrimSpace(job.StartedAt),
		strings.TrimSpace(job.FinishedAt),
		strings.TrimSpace(job.CreatedAt),
		strings.TrimSpace(job.UpdatedAt),
	)
	if err != nil {
		return "", err
	}
	row, err := r.GetByIdempotency(ctx, channelID, idempotency)
	if err != nil {
		return strings.TrimSpace(job.ID), err
	}
	return strings.TrimSpace(row.ID), nil
}

func (r *channelTurnJobRepo) UpdateStatus(ctx context.Context, id, status, errMsg, previewMsgID, contentPreview string) error {
	db := r.data.RawDB()
	if db == nil || strings.TrimSpace(id) == "" {
		return nil
	}
	now := biz.ChannelTurnJobNow()
	status = biz.NormalizeChannelTurnJobStatus(status)
	_, err := db.ExecContext(ctx, `
UPDATE channel_turn_job SET
  status=?,
  error_message=CASE WHEN ? != '' THEN ? ELSE error_message END,
  preview_message_id=CASE WHEN ? != '' THEN ? ELSE preview_message_id END,
  content_preview=CASE WHEN ? != '' THEN ? ELSE content_preview END,
  started_at=CASE WHEN started_at='' AND ? IN ('running','async_queued') THEN ? ELSE started_at END,
  finished_at=CASE WHEN ? IN ('completed','failed','timeout','cancelled','queued') THEN ? ELSE finished_at END,
  updated_at=?
WHERE id=?`,
		status,
		errMsg, errMsg,
		previewMsgID, previewMsgID,
		contentPreview, contentPreview,
		status, now,
		status, now,
		now,
		id,
	)
	return err
}

func (r *channelTurnJobRepo) UpdateAsyncTarget(ctx context.Context, id, targetType, targetID string) error {
	db := r.data.RawDB()
	if db == nil || strings.TrimSpace(id) == "" {
		return nil
	}
	now := biz.ChannelTurnJobNow()
	_, err := db.ExecContext(ctx, `
UPDATE channel_turn_job SET
  async_target_type=CASE WHEN ? != '' THEN ? ELSE async_target_type END,
  async_target_id=CASE WHEN ? != '' THEN ? ELSE async_target_id END,
  updated_at=?
WHERE id=?`,
		targetType, targetType,
		targetID, targetID,
		now,
		id,
	)
	return err
}

func (r *channelTurnJobRepo) GetByIdempotency(ctx context.Context, channelID, idempotencyKey string) (biz.ChannelTurnJob, error) {
	db := r.data.RawDB()
	if db == nil {
		return biz.ChannelTurnJob{}, sql.ErrNoRows
	}
	row := db.QueryRowContext(ctx, `
SELECT id, channel_id, session_id, peer_id, peer_key, idempotency_key, status,
  preview_message_id, content_preview, async_target_type, async_target_id,
  error_message, started_at, finished_at, created_at, updated_at
FROM channel_turn_job WHERE channel_id=? AND idempotency_key=? LIMIT 1`,
		strings.TrimSpace(channelID), strings.TrimSpace(idempotencyKey),
	)
	return scanChannelTurnJob(row)
}

func (r *channelTurnJobRepo) ListByChannel(ctx context.Context, channelID string, limit int) ([]biz.ChannelTurnJob, error) {
	db := r.data.RawDB()
	if db == nil {
		return nil, nil
	}
	limit = biz.NormalizeChannelTurnJobListLimit(limit)
	rows, err := db.QueryContext(ctx, `
SELECT id, channel_id, session_id, peer_id, peer_key, idempotency_key, status,
  preview_message_id, content_preview, async_target_type, async_target_id,
  error_message, started_at, finished_at, created_at, updated_at
FROM channel_turn_job WHERE channel_id=? ORDER BY created_at DESC LIMIT ?`,
		strings.TrimSpace(channelID), limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []biz.ChannelTurnJob
	for rows.Next() {
		job, err := scanChannelTurnJobRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

func scanChannelTurnJob(row *sql.Row) (biz.ChannelTurnJob, error) {
	var j biz.ChannelTurnJob
	err := row.Scan(
		&j.ID, &j.ChannelID, &j.SessionID, &j.PeerID, &j.PeerKey, &j.IdempotencyKey, &j.Status,
		&j.PreviewMessageID, &j.ContentPreview, &j.AsyncTargetType, &j.AsyncTargetID,
		&j.ErrorMessage, &j.StartedAt, &j.FinishedAt, &j.CreatedAt, &j.UpdatedAt,
	)
	return j, err
}

func scanChannelTurnJobRows(rows *sql.Rows) (biz.ChannelTurnJob, error) {
	var j biz.ChannelTurnJob
	err := rows.Scan(
		&j.ID, &j.ChannelID, &j.SessionID, &j.PeerID, &j.PeerKey, &j.IdempotencyKey, &j.Status,
		&j.PreviewMessageID, &j.ContentPreview, &j.AsyncTargetType, &j.AsyncTargetID,
		&j.ErrorMessage, &j.StartedAt, &j.FinishedAt, &j.CreatedAt, &j.UpdatedAt,
	)
	return j, err
}
