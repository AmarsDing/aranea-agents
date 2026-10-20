package data

import (
	"context"
	"database/sql"
	"strings"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

// knowledgeWriteBackOutboxRepo 是 G-10 写回重试队列的 raw-SQL 实现
// （表由 DDL migration 20261280 创建；模式复用 decision_repo.go outbox 段）。
type knowledgeWriteBackOutboxRepo struct {
	data *Data
	lg   loggateway.Logger
}

var _ bizknowledge.WriteBackOutboxRepo = (*knowledgeWriteBackOutboxRepo)(nil)

// NewKnowledgeWriteBackOutboxRepo 构造写回 outbox raw-SQL 仓储。
func NewKnowledgeWriteBackOutboxRepo(data *Data, lg loggateway.Logger) bizknowledge.WriteBackOutboxRepo {
	if data == nil || data.RWDB() == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &knowledgeWriteBackOutboxRepo{data: data, lg: lg.With(loggateway.Domain("knowledge_writeback_outbox"))}
}

// NewKnowledgeWriteBackOutboxRepoFromData 是 Wire 友好的构造器。
func NewKnowledgeWriteBackOutboxRepoFromData(d *Data) bizknowledge.WriteBackOutboxRepo {
	if d == nil {
		return nil
	}
	return NewKnowledgeWriteBackOutboxRepo(d, d.lg)
}

func (r *knowledgeWriteBackOutboxRepo) EnqueueWriteBackOutbox(ctx context.Context, batchKey string, in bizknowledge.WriteBackInput) error {
	if r == nil || r.data == nil || r.data.RWDB() == nil {
		return nil
	}
	batchKey = strings.TrimSpace(batchKey)
	if batchKey == "" || len(in.Facts) == 0 {
		return nil
	}
	payload, err := bizknowledge.EncodeWriteBackOutboxPayload(in)
	if err != nil {
		return entErrToBizErr(err, "KNOWLEDGE_WRITEBACK_OUTBOX")
	}
	d := r.data.Dialect()
	q := d.BuildInsertOrIgnore(
		"knowledge_writeback_outbox",
		"batch_key, workspace_id, payload, created_at",
		d.Placeholders(4),
		"batch_key",
	)
	_, err = r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q,
		batchKey, in.Workspace, string(payload), time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return entErrToBizErr(err, "KNOWLEDGE_WRITEBACK_OUTBOX")
	}
	r.lg.Info("知识库写回批次已入重试队列",
		loggateway.Str("batch_key", batchKey),
		loggateway.SessionID(in.SessionID),
	)
	return nil
}

// ListPendingWriteBackOutbox 取 pending 且 attempts 未触达上限的行（升序）。
// 触达上限的行已被 MarkWriteBackOutboxAttempt 翻 dead，此处 attempts<max 是
// 双保险——永久失败行不占用扫描窗口，避免 head-of-line blocking。
func (r *knowledgeWriteBackOutboxRepo) ListPendingWriteBackOutbox(ctx context.Context, limit int) ([]bizknowledge.WriteBackOutboxRow, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 100
	}
	q := r.data.Dialect().RenumberPlaceholders(
		`SELECT id, batch_key, workspace_id, payload, status, attempts, last_error, created_at, published_at
		 FROM knowledge_writeback_outbox WHERE status=? AND attempts < ? ORDER BY created_at ASC LIMIT ?`)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q,
		bizknowledge.WriteBackOutboxStatusPending, bizknowledge.WriteBackOutboxMaxAttempts, limit)
	if err != nil {
		return nil, entErrToBizErr(err, "KNOWLEDGE_WRITEBACK_OUTBOX")
	}
	defer rows.Close()
	out := make([]bizknowledge.WriteBackOutboxRow, 0, limit)
	for rows.Next() {
		var row bizknowledge.WriteBackOutboxRow
		var published sql.NullString
		var created string
		if err := rows.Scan(&row.ID, &row.BatchKey, &row.WorkspaceID, &row.Payload, &row.Status,
			&row.Attempts, &row.LastError, &created, &published); err != nil {
			return nil, entErrToBizErr(err, "KNOWLEDGE_WRITEBACK_OUTBOX")
		}
		row.CreatedAt = parseRFC3339Loose(created)
		if published.Valid && published.String != "" {
			if t := parseRFC3339Loose(published.String); !t.IsZero() {
				row.PublishedAt = &t
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "KNOWLEDGE_WRITEBACK_OUTBOX")
	}
	return out, nil
}

func (r *knowledgeWriteBackOutboxRepo) MarkWriteBackOutboxPublished(ctx context.Context, ids []int64, publishedAt time.Time) error {
	if r == nil || r.data == nil || r.data.RWDB() == nil || len(ids) == 0 {
		return nil
	}
	if publishedAt.IsZero() {
		publishedAt = time.Now().UTC()
	}
	marks := make([]string, len(ids))
	args := make([]any, 0, len(ids)+2)
	args = append(args, bizknowledge.WriteBackOutboxStatusPublished, publishedAt.UTC().Format(time.RFC3339Nano))
	for i, id := range ids {
		marks[i] = "?"
		args = append(args, id)
	}
	q := r.data.Dialect().RenumberPlaceholders(
		`UPDATE knowledge_writeback_outbox SET status=?, published_at=? WHERE id IN (` +
			strings.Join(marks, ",") + `)`)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q, args...)
	if err != nil {
		return entErrToBizErr(err, "KNOWLEDGE_WRITEBACK_OUTBOX")
	}
	return nil
}

// MarkWriteBackOutboxAttempt 记录一次失败重放；attempts+1 触达上限时同语句翻
// dead——行从此退出 replay 扫描，payload/last_error 留存供事后排查。
func (r *knowledgeWriteBackOutboxRepo) MarkWriteBackOutboxAttempt(ctx context.Context, id int64, lastError string) error {
	if r == nil || r.data == nil || r.data.RWDB() == nil || id <= 0 {
		return nil
	}
	if len(lastError) > 2000 {
		lastError = lastError[:2000]
	}
	q := r.data.Dialect().RenumberPlaceholders(
		`UPDATE knowledge_writeback_outbox
		 SET attempts=attempts+1, last_error=?,
		     status=CASE WHEN attempts+1 >= ? THEN ? ELSE status END
		 WHERE id=?`)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q,
		lastError, bizknowledge.WriteBackOutboxMaxAttempts, bizknowledge.WriteBackOutboxStatusDead, id)
	if err != nil {
		return entErrToBizErr(err, "KNOWLEDGE_WRITEBACK_OUTBOX")
	}
	return nil
}

// MarkWriteBackOutboxDead 把 poison 行（payload 不可解码）直接翻 dead 终态。
func (r *knowledgeWriteBackOutboxRepo) MarkWriteBackOutboxDead(ctx context.Context, ids []int64, lastError string) error {
	if r == nil || r.data == nil || r.data.RWDB() == nil || len(ids) == 0 {
		return nil
	}
	if len(lastError) > 2000 {
		lastError = lastError[:2000]
	}
	marks := make([]string, len(ids))
	args := make([]any, 0, len(ids)+2)
	args = append(args, bizknowledge.WriteBackOutboxStatusDead, lastError)
	for i, id := range ids {
		marks[i] = "?"
		args = append(args, id)
	}
	q := r.data.Dialect().RenumberPlaceholders(
		`UPDATE knowledge_writeback_outbox SET status=?, last_error=? WHERE id IN (` +
			strings.Join(marks, ",") + `)`)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q, args...)
	if err != nil {
		return entErrToBizErr(err, "KNOWLEDGE_WRITEBACK_OUTBOX")
	}
	return nil
}
