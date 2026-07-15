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

const defaultEventOutboxListLimit = 100

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

// EnsureEventDeliveryOutboxSchema creates the outbox table when missing (tests / safety net).
func EnsureEventDeliveryOutboxSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return nil
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS event_delivery_outbox (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			seq INTEGER NOT NULL,
			event_id TEXT NOT NULL,
			kind TEXT NOT NULL DEFAULT '',
			entity_id TEXT NOT NULL DEFAULT '',
			payload BLOB NOT NULL,
			published_at TEXT,
			created_at TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_event_delivery_outbox_session_seq
			ON event_delivery_outbox(session_id, seq)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_event_delivery_outbox_session_event_id
			ON event_delivery_outbox(session_id, event_id)`,
		`CREATE INDEX IF NOT EXISTS idx_event_delivery_outbox_session_id
			ON event_delivery_outbox(session_id)`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return entErrToBizErr(err, "EVENT_OUTBOX")
		}
	}
	return nil
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
	if r == nil || r.data == nil || r.data.RWDB() == nil {
		return nil
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	if publishedAt.IsZero() {
		publishedAt = time.Now().UTC()
	}
	q := r.data.Dialect().RenumberPlaceholders(
		`UPDATE event_delivery_outbox SET published_at=? WHERE id=? AND (published_at IS NULL OR published_at='')`,
	)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q, publishedAt.UTC().Format(time.RFC3339Nano), id)
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
	out := make([]biz.EventDeliveryOutboxRow, 0, limit)
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
