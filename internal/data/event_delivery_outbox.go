package data

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

const (
	defaultEventOutboxListLimit    = 100
	defaultEventOutboxPendingLimit = 200
)

type eventDeliveryOutboxRepo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.EventDeliveryOutboxRepo = (*eventDeliveryOutboxRepo)(nil)

// NewEventDeliveryOutboxRepo constructs the raw-SQL outbox repo (B-06).
func NewEventDeliveryOutboxRepo(data *Data, lg loggateway.Logger) biz.EventDeliveryOutboxRepo {
	if data == nil || data.RWDB() == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &eventDeliveryOutboxRepo{data: data, lg: lg.With(loggateway.Domain("event_outbox"))}
}

// NewEventDeliveryOutboxRepoFromData is the Wire-friendly constructor.
func NewEventDeliveryOutboxRepoFromData(d *Data) biz.EventDeliveryOutboxRepo {
	if d == nil {
		return nil
	}
	return NewEventDeliveryOutboxRepo(d, d.lg)
}

func (r *eventDeliveryOutboxRepo) Insert(ctx context.Context, row biz.EventDeliveryOutboxRow) error {
	if r == nil || r.data == nil || r.data.RWDB() == nil {
		return nil
	}
	sessionID := strings.TrimSpace(row.SessionID)
	eventID := strings.TrimSpace(row.EventID)
	if sessionID == "" || eventID == "" || row.Seq <= 0 {
		return nil
	}
	id := strings.TrimSpace(row.ID)
	if id == "" {
		id = uuid.NewString()
	}
	createdAt := row.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	var published any
	if row.PublishedAt != nil && !row.PublishedAt.IsZero() {
		published = row.PublishedAt.UTC().Format(time.RFC3339Nano)
	}
	d := r.data.Dialect()
	cols := "id, session_id, seq, event_id, kind, entity_id, payload, published_at, created_at"
	placeholders := d.Placeholders(9)
	q := d.BuildInsertOrIgnore(
		"event_delivery_outbox",
		cols,
		placeholders,
		"session_id, seq",
	)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q,
		id, sessionID, row.Seq, eventID, row.Kind, row.EntityID, row.Payload, published,
		createdAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return entErrToBizErr(err, "EVENT_OUTBOX")
	}
	return nil
}

func (r *eventDeliveryOutboxRepo) MarkPublished(ctx context.Context, id string, publishedAt time.Time) error {
	return r.MarkPublishedBatch(ctx, []string{id}, publishedAt)
}

func (r *eventDeliveryOutboxRepo) MarkPublishedBatch(ctx context.Context, ids []string, publishedAt time.Time) error {
	if r == nil || r.data == nil || r.data.RWDB() == nil {
		return nil
	}
	dedup := make(map[string]struct{}, len(ids))
	clean := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := dedup[id]; ok {
			continue
		}
		dedup[id] = struct{}{}
		clean = append(clean, id)
	}
	if len(clean) == 0 {
		return nil
	}
	if publishedAt.IsZero() {
		publishedAt = time.Now().UTC()
	}
	// published_at is timestamptz on Postgres (raw-DDL column) — comparing it
	// to '' raises 22007 (invalid input syntax), so only NULL is tested.
	unpublishedCond := "published_at IS NULL"
	placeholders := make([]string, len(clean))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	q := r.data.Dialect().RenumberPlaceholders(
		`UPDATE event_delivery_outbox SET published_at=? WHERE id IN (` + strings.Join(placeholders, ",") + `) AND ` + unpublishedCond,
	)
	args := make([]any, 0, len(clean)+1)
	args = append(args, publishedAt.UTC().Format(time.RFC3339Nano))
	for _, id := range clean {
		args = append(args, id)
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q, args...)
	if err != nil {
		return entErrToBizErr(err, "EVENT_OUTBOX")
	}
	return nil
}

func (r *eventDeliveryOutboxRepo) ListAfter(ctx context.Context, sessionID, afterEventID string, afterSeq int64, limit int) ([]biz.EventDeliveryOutboxRow, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil {
		return nil, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = defaultEventOutboxListLimit
	}
	d := r.data.Dialect()
	cursorSeq := afterSeq
	afterEventID = strings.TrimSpace(afterEventID)
	if afterEventID != "" {
		var seq int64
		q := d.RenumberPlaceholders(
			`SELECT seq FROM event_delivery_outbox WHERE session_id=? AND event_id=? LIMIT 1`,
		)
		err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), q, []any{sessionID, afterEventID}, &seq)
		if err != nil {
			if apierror.IsCode(err, apierror.CodeNotFound) {
				return nil, nil
			}
			return nil, entErrToBizErr(err, "EVENT_OUTBOX")
		}
		cursorSeq = seq
	}
	if cursorSeq < 0 {
		cursorSeq = 0
	}
	q := d.RenumberPlaceholders(
		`SELECT id, session_id, seq, event_id, kind, entity_id, payload, published_at, created_at
		 FROM event_delivery_outbox
		 WHERE session_id=? AND seq>?
		 ORDER BY seq ASC
		 LIMIT ?`,
	)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, sessionID, cursorSeq, limit)
	if err != nil {
		return nil, entErrToBizErr(err, "EVENT_OUTBOX")
	}
	defer rows.Close()
	out, err := scanEventOutboxRows(rows, limit)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *eventDeliveryOutboxRepo) ListPending(ctx context.Context, cutoff time.Time, limit int) ([]biz.EventDeliveryOutboxRow, error) {
	if r == nil || r.data == nil || r.data.RWDB() == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = defaultEventOutboxPendingLimit
	}
	d := r.data.Dialect()
	// See MarkPublished: published_at is timestamptz on Postgres, so only
	// NULL is tested for "unpublished".
	unpublishedCond := "published_at IS NULL"
	args := make([]any, 0, 3)
	createdCond := ""
	if !cutoff.IsZero() {
		// created_at is written as UTC RFC3339Nano TEXT; a same-format string
		// compare coerces cleanly to timestamptz on PG.
		createdCond = " AND created_at < ?"
		args = append(args, cutoff.UTC().Format(time.RFC3339Nano))
	}
	args = append(args, limit)
	q := d.RenumberPlaceholders(
		`SELECT id, session_id, seq, event_id, kind, entity_id, payload, published_at, created_at
		 FROM event_delivery_outbox
		 WHERE ` + unpublishedCond + createdCond + `
		 ORDER BY created_at ASC
		 LIMIT ?`,
	)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "EVENT_OUTBOX")
	}
	defer rows.Close()
	out, err := scanEventOutboxRows(rows, limit)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// scanEventOutboxRows decodes rows produced by the shared outbox SELECT
// column list. Timestamps are stored as RFC3339(Nano) strings (timestamptz on
// Postgres returns the same layout), parsed leniently — an unparseable value
// leaves the field zero/nil rather than failing the whole batch.
func scanEventOutboxRows(rows *sql.Rows, capHint int) ([]biz.EventDeliveryOutboxRow, error) {
	out := make([]biz.EventDeliveryOutboxRow, 0, capHint)
	for rows.Next() {
		var row biz.EventDeliveryOutboxRow
		var published, created sql.NullString
		if err := rows.Scan(
			&row.ID, &row.SessionID, &row.Seq, &row.EventID, &row.Kind, &row.EntityID,
			&row.Payload, &published, &created,
		); err != nil {
			return nil, entErrToBizErr(err, "EVENT_OUTBOX")
		}
		if published.Valid && published.String != "" {
			if t, err := time.Parse(time.RFC3339Nano, published.String); err == nil {
				row.PublishedAt = &t
			} else if t, err := time.Parse(time.RFC3339, published.String); err == nil {
				row.PublishedAt = &t
			}
		}
		if created.Valid && created.String != "" {
			if t, err := time.Parse(time.RFC3339Nano, created.String); err == nil {
				row.CreatedAt = t
			} else if t, err := time.Parse(time.RFC3339, created.String); err == nil {
				row.CreatedAt = t
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "EVENT_OUTBOX")
	}
	return out, nil
}
