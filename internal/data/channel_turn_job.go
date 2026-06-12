package data

import (
	"context"
	"database/sql"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

type channelTurnJobRepo struct {
	data *Data
}

var _ biz.ChannelTurnJobRepo = (*channelTurnJobRepo)(nil)

// NewChannelTurnJobRepo implements biz.ChannelTurnJobRepo.
func NewChannelTurnJobRepo(d *Data) biz.ChannelTurnJobRepo {
	return &channelTurnJobRepo{data: d}
}

const channelTurnJobSelectSQL = `SELECT id, channel_id, session_id, peer_id, peer_key, idempotency_key, status, preview_message_id, content_preview, async_target_type, async_target_id, error_message, started_at, finished_at, created_at, updated_at FROM channel_turn_job`

func scanChannelTurnJobRow(rows *sql.Rows) (biz.ChannelTurnJob, error) {
	var j biz.ChannelTurnJob
	err := rows.Scan(
		&j.ID, &j.ChannelID, &j.SessionID, &j.PeerID, &j.PeerKey, &j.IdempotencyKey, &j.Status,
		&j.PreviewMessageID, &j.ContentPreview, &j.AsyncTargetType, &j.AsyncTargetID,
		&j.ErrorMessage, &j.StartedAt, &j.FinishedAt, &j.CreatedAt, &j.UpdatedAt,
	)
	return j, err
}

func queryChannelTurnJob(ctx context.Context, db execer, where string, args ...any) (biz.ChannelTurnJob, error) {
	rows, err := db.QueryContext(ctx, channelTurnJobSelectSQL+" "+where, args...)
	if err != nil {
		return biz.ChannelTurnJob{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return biz.ChannelTurnJob{}, apierror.NotFound(apierror.DomainChannel, "not found")
	}
	j, err := scanChannelTurnJobRow(rows)
	if err != nil {
		return biz.ChannelTurnJob{}, err
	}
	return j, rows.Err()
}

// Create uses Raw SQL because it relies on ON CONFLICT DO UPDATE upsert
// with conditional status preservation logic not expressible via Ent's Create API.
func (r *channelTurnJobRepo) Create(ctx context.Context, job biz.ChannelTurnJob) (string, error) {
	db := r.data.RWDB().WriteDB(ctx)
	if db == nil {
		return "", apierror.Internal("CHANNEL_TURN_JOB", "repository unavailable")
	}
	channelID := strings.TrimSpace(job.ChannelID)
	idempotency := strings.TrimSpace(job.IdempotencyKey)
	var actualID string
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		e := TxExecerFromCtx(txCtx, r.data.RWDB().WriteHandle())
		_, execErr := e.ExecContext(txCtx, `
INSERT INTO channel_turn_job (
  id, channel_id, session_id, peer_id, peer_key, idempotency_key, status,
  preview_message_id, content_preview, async_target_type, async_target_id,
  error_message, started_at, finished_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(channel_id, idempotency_key) DO UPDATE SET
  updated_at=excluded.updated_at,
  status=CASE WHEN channel_turn_job.status IN ('completed','failed','timeout','cancelled','queued','async_queued','running')
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
		if execErr != nil {
			return execErr
		}
		row, queryErr := queryChannelTurnJob(txCtx, e, `WHERE channel_id = ? AND idempotency_key = ? LIMIT 1`,
			channelID, idempotency)
		if queryErr != nil {
			return queryErr
		}
		actualID = strings.TrimSpace(row.ID)
		return nil
	})
	if err != nil {
		return "", err
	}
	return actualID, nil
}

// UpdateStatus uses Raw SQL because it relies on conditional CASE expressions
// for setting started_at/finished_at based on status, not expressible via Ent's Update API.
func (r *channelTurnJobRepo) UpdateStatus(ctx context.Context, id, status, errMsg, previewMsgID, contentPreview string) error {
	db := r.data.RWDB().WriteDB(ctx)
	if db == nil {
		return apierror.Internal("CHANNEL_TURN_JOB", "repository unavailable")
	}
	if strings.TrimSpace(id) == "" {
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

// UpdateAsyncTarget uses Raw SQL because it relies on conditional CASE expressions.
func (r *channelTurnJobRepo) UpdateAsyncTarget(ctx context.Context, id, targetType, targetID string) error {
	db := r.data.RWDB().WriteDB(ctx)
	if db == nil {
		return apierror.Internal("CHANNEL_TURN_JOB", "repository unavailable")
	}
	if strings.TrimSpace(id) == "" {
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
	if r == nil || r.data == nil {
		return biz.ChannelTurnJob{}, apierror.NotFound(apierror.DomainChannel, "not found")
	}
	db := r.data.RWDB().ReadDB(ctx)
	if db == nil {
		return biz.ChannelTurnJob{}, apierror.NotFound(apierror.DomainChannel, "not found")
	}
	return queryChannelTurnJob(ctx, db, `WHERE channel_id = ? AND idempotency_key = ? LIMIT 1`,
		strings.TrimSpace(channelID), strings.TrimSpace(idempotencyKey))
}

func (r *channelTurnJobRepo) GetByID(ctx context.Context, id string) (biz.ChannelTurnJob, error) {
	if r == nil || r.data == nil {
		return biz.ChannelTurnJob{}, apierror.NotFound(apierror.DomainChannel, "not found")
	}
	db := r.data.RWDB().ReadDB(ctx)
	if db == nil {
		return biz.ChannelTurnJob{}, apierror.NotFound(apierror.DomainChannel, "not found")
	}
	return queryChannelTurnJob(ctx, db, `WHERE id = ? LIMIT 1`, strings.TrimSpace(id))
}

func (r *channelTurnJobRepo) ListByChannel(ctx context.Context, channelID string, limit int) ([]biz.ChannelTurnJob, error) {
	if r == nil || r.data == nil {
		return nil, nil
	}
	db := r.data.RWDB().ReadDB(ctx)
	if db == nil {
		return nil, nil
	}
	limit = biz.NormalizeChannelTurnJobListLimit(limit)
	rows, err := db.QueryContext(ctx, channelTurnJobSelectSQL+` WHERE channel_id = ? ORDER BY created_at DESC LIMIT ?`,
		strings.TrimSpace(channelID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []biz.ChannelTurnJob
	for rows.Next() {
		j, err := scanChannelTurnJobRow(rows)
		if err != nil {
			return out, err
		}
		out = append(out, j)
	}
	return out, nil
}

// ListFiltered uses Raw SQL because it JOINs sessions and graph_executions tables
// for agent_id and graph_id lookup, not expressible via Ent's predicate system.
func (r *channelTurnJobRepo) ListFiltered(ctx context.Context, q biz.ChannelTurnJobListQuery) ([]biz.ChannelTurnJob, error) {
	db := r.data.RWDB().ReadDB(ctx)
	if db == nil {
		return nil, nil
	}
	limit := biz.NormalizeChannelTurnJobListLimit(q.Limit)
	query := `
SELECT j.id, j.channel_id, j.session_id, j.peer_id, j.peer_key, j.idempotency_key, j.status,
  j.preview_message_id, j.content_preview, j.async_target_type, j.async_target_id,
  j.error_message, j.started_at, j.finished_at, j.created_at, j.updated_at,
  COALESCE(s.agent_id, ''), COALESCE(g.graph_id, '')
FROM channel_turn_job j
LEFT JOIN sessions s ON s.id = j.session_id
LEFT JOIN graph_executions g ON g.id = j.async_target_id
  AND j.async_target_type IN ('graph', 'team_graph')
WHERE (? = '' OR j.session_id = ?)
  AND (? = '' OR s.agent_id = ?)
  AND (? = '' OR j.status = ?)
ORDER BY j.created_at DESC
LIMIT ?`
	sessionID := strings.TrimSpace(q.SessionID)
	agentID := strings.TrimSpace(q.AgentID)
	statusFilter := strings.TrimSpace(q.Status)
	statusParam := ""
	if statusFilter != "" {
		statusParam = biz.NormalizeChannelTurnJobStatus(statusFilter)
	}
	rows, err := db.QueryContext(ctx, query,
		sessionID, sessionID,
		agentID, agentID,
		statusParam, statusParam,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []biz.ChannelTurnJob
	for rows.Next() {
		job, err := scanChannelTurnJobListRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

func scanChannelTurnJobListRow(rows *sql.Rows) (biz.ChannelTurnJob, error) {
	var j biz.ChannelTurnJob
	err := rows.Scan(
		&j.ID, &j.ChannelID, &j.SessionID, &j.PeerID, &j.PeerKey, &j.IdempotencyKey, &j.Status,
		&j.PreviewMessageID, &j.ContentPreview, &j.AsyncTargetType, &j.AsyncTargetID,
		&j.ErrorMessage, &j.StartedAt, &j.FinishedAt, &j.CreatedAt, &j.UpdatedAt,
		&j.AgentID, &j.GraphID,
	)
	return j, err
}

