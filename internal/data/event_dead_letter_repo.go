package data

// event_dead_letter_repo.go — P1-R2b: durable dead-letter store for v2
// sequencer events whose entity persist failed permanently.
//
// Implements biz.EventDeadLetterRepo using the raw SQLite database.
// Table is created by DDL migration 20260826 (primary path, DB-R4 compliance).
// The ensureTable() safety net handles the Wire-construction-vs-P1-migration
// race, mirroring memory_job_deadletter.go.

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// EventDeadLetterRepo persists dead-lettered v2 events for later replay.
type EventDeadLetterRepo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.EventDeadLetterRepo = (*EventDeadLetterRepo)(nil)

// NewEventDeadLetterRepo creates the repository. Returns the biz interface
// directly so no wire.Bind is needed (v2 repos convention).
func NewEventDeadLetterRepo(d *Data, lg loggateway.Logger) biz.EventDeadLetterRepo {
	r := &EventDeadLetterRepo{data: d, lg: lg.With(loggateway.Domain("event_dead_letter"))}
	r.ensureTable()
	return r
}

func (r *EventDeadLetterRepo) ensureTable() {
	// Safety net for the Wire-construction-vs-P1-migration race (see
	// memory_job_deadletter.go): the versioned DDL migration 20260826 is the
	// primary creation path; this idempotent CREATE TABLE IF NOT EXISTS
	// ensures the table exists regardless of startup ordering.
	db := r.data.RWDB().WriteHandle()
	if db == nil {
		return
	}
	d := r.data.Dialect()
	idColDef := "id INTEGER PRIMARY KEY AUTOINCREMENT"
	if d.IsPostgres() {
		idColDef = "id BIGSERIAL PRIMARY KEY"
	}
	_, err := db.Exec(fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS event_dead_letter (
    %s,
    event_kind   TEXT    NOT NULL,
    entity_kind  TEXT    NOT NULL,
    entity_op    TEXT    NOT NULL DEFAULT 'upsert',
    entity_id    TEXT    NOT NULL DEFAULT '',
    session_id   TEXT    NOT NULL DEFAULT '',
    payload_json TEXT    NOT NULL DEFAULT '{}',
    attempts     INTEGER NOT NULL DEFAULT 0,
    last_error   TEXT    NOT NULL DEFAULT '',
    state        TEXT    NOT NULL DEFAULT 'pending'
                 CHECK(state IN ('pending','replayed','abandoned')),
    failed_at    INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL
)`, idColDef))
	if err != nil {
		r.lg.Warn("ensureTable: CREATE TABLE event_dead_letter failed", loggateway.Err(err))
		return
	}
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_event_dl_state ON event_dead_letter(state, failed_at)`)
	db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_event_dl_unique_pending ON event_dead_letter(entity_kind, entity_id) WHERE state = 'pending'`)
}

// SaveEventDeadLetter upserts a pending dead-letter row. Atomic
// INSERT ... ON CONFLICT dedups per (entity_kind, entity_id) among pending
// rows, mirroring the in-memory ring's entity-ID dedup semantics: a repeated
// failure for the same entity refreshes the record instead of stacking rows.
func (r *EventDeadLetterRepo) SaveEventDeadLetter(ctx context.Context, rec biz.EventDeadLetter) error {
	now := time.Now().UnixMilli()
	if rec.FailedAt.IsZero() {
		rec.FailedAt = time.Now().UTC()
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(`
INSERT INTO event_dead_letter
  (event_kind, entity_kind, entity_op, entity_id, session_id, payload_json,
   attempts, last_error, state, failed_at, updated_at)
VALUES (?,?,?,?,?,?,0,?,'pending',?,?)
ON CONFLICT(entity_kind, entity_id) WHERE state='pending'
DO UPDATE SET
  event_kind   = excluded.event_kind,
  entity_op    = excluded.entity_op,
  session_id   = excluded.session_id,
  payload_json = excluded.payload_json,
  last_error   = excluded.last_error,
  updated_at   = excluded.updated_at`),
		rec.EventKind, rec.EntityKind, rec.EntityOp, rec.EntityID, rec.SessionID,
		rec.PayloadJSON, rec.LastError, rec.FailedAt.UnixMilli(), now,
	)
	return entErrToBizErr(err, "EVENT_DL")
}

// ListPendingEventDeadLetters returns pending rows ordered by failed_at.
func (r *EventDeadLetterRepo) ListPendingEventDeadLetters(ctx context.Context, limit int) ([]biz.EventDeadLetter, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(`
SELECT id, event_kind, entity_kind, entity_op, entity_id, session_id, payload_json,
       attempts, last_error, failed_at
FROM event_dead_letter
WHERE state = 'pending'
ORDER BY failed_at ASC
LIMIT ?`), limit)
	if err != nil {
		return nil, entErrToBizErr(err, "EVENT_DL")
	}
	defer rows.Close()
	var out []biz.EventDeadLetter
	for rows.Next() {
		var rec biz.EventDeadLetter
		var failedMs int64
		if err := rows.Scan(&rec.ID, &rec.EventKind, &rec.EntityKind, &rec.EntityOp,
			&rec.EntityID, &rec.SessionID, &rec.PayloadJSON,
			&rec.Attempts, &rec.LastError, &failedMs); err != nil {
			return nil, entErrToBizErr(err, "EVENT_DL")
		}
		rec.State = biz.EventDeadLetterStatePending
		rec.FailedAt = time.UnixMilli(failedMs).UTC()
		out = append(out, rec)
	}
	return out, entErrToBizErr(rows.Err(), "EVENT_DL")
}

// MarkEventDeadLetterReplayed marks a row as successfully replayed.
func (r *EventDeadLetterRepo) MarkEventDeadLetterReplayed(ctx context.Context, id int64) error {
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE event_dead_letter SET state='replayed', updated_at=? WHERE id=?`),
		time.Now().UnixMilli(), id)
	return entErrToBizErr(err, "EVENT_DL")
}

// MarkEventDeadLetterAbandoned marks a row as permanently abandoned.
func (r *EventDeadLetterRepo) MarkEventDeadLetterAbandoned(ctx context.Context, id int64, reason string) error {
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE event_dead_letter SET state='abandoned', last_error=?, updated_at=? WHERE id=?`),
		reason, time.Now().UnixMilli(), id)
	return entErrToBizErr(err, "EVENT_DL")
}

// IncrementEventDeadLetterAttempt records a failed replay attempt.
func (r *EventDeadLetterRepo) IncrementEventDeadLetterAttempt(ctx context.Context, id int64, lastErr string) error {
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE event_dead_letter SET attempts=attempts+1, last_error=?, updated_at=? WHERE id=?`),
		lastErr, time.Now().UnixMilli(), id)
	return entErrToBizErr(err, "EVENT_DL")
}
