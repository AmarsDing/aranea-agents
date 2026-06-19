package data

// memory_job_deadletter.go — MEM-OPT-03: persistent dead-letter store for AutoMemory jobs.
//
// Implements trpcmem.MemoryDeadLetterSink using the raw SQLite database.
// Table is created by DDL migration 20260728 (primary path, DB-R4 compliance).
// The ensureTable() safety net handles the Wire-construction-vs-P1-migration race.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// MemoryJobDeadLetterRepo persists dropped AutoMemory jobs for later replay.
// Implements biz.MemoryDeadLetterSink (H-01: data layer no longer imports memory/trpc).
type MemoryJobDeadLetterRepo struct {
	data *Data
}

var _ biz.MemoryDeadLetterSink = (*MemoryJobDeadLetterRepo)(nil)
var _ biz.MemoryDeadLetterAdminRepo = (*MemoryJobDeadLetterRepo)(nil)

// NewMemoryJobDeadLetterRepo creates the dead-letter repository.
func NewMemoryJobDeadLetterRepo(d *Data) *MemoryJobDeadLetterRepo {
	r := &MemoryJobDeadLetterRepo{data: d}
	r.ensureTable()
	return r
}

func (r *MemoryJobDeadLetterRepo) ensureTable() {
	// Safety net for the Wire-construction-vs-P1-migration race: NewMemoryJobDeadLetterRepo
	// may be called by Wire before the P1 DDL migration (version 20260728) completes.
	// The migration is the primary, versioned creation path (DB-R4); this idempotent
	// CREATE TABLE IF NOT EXISTS ensures the table exists regardless of startup ordering.
	db := r.data.RWDB().WriteHandle()
	if db == nil {
		return
	}
	// Dialect-aware auto-increment syntax:
	// SQLite: INTEGER PRIMARY KEY AUTOINCREMENT
	// Postgres: BIGSERIAL PRIMARY KEY
	d := r.data.Dialect()
	idColDef := "id INTEGER PRIMARY KEY AUTOINCREMENT"
	if d.IsPostgres() {
		idColDef = "id BIGSERIAL PRIMARY KEY"
	}
	_, err := db.Exec(fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS memory_job_deadletter (
    %s,
    enqueued_at      INTEGER NOT NULL,
    failed_at        INTEGER NOT NULL,
    session_id       TEXT    NOT NULL DEFAULT '',
    app_name         TEXT    NOT NULL DEFAULT '',
    user_id          TEXT    NOT NULL DEFAULT '',
    feedback_msg_id  TEXT    NOT NULL DEFAULT '',
    payload_json     TEXT    NOT NULL DEFAULT '{}',
    drop_reason      TEXT    NOT NULL,
    priority         INTEGER NOT NULL DEFAULT 1,
    attempts         INTEGER NOT NULL DEFAULT 0,
    last_error       TEXT    NOT NULL DEFAULT '',
    state            TEXT    NOT NULL DEFAULT 'pending'
                     CHECK(state IN ('pending','replayed','abandoned'))
)`, idColDef))
	if err != nil {
		r.data.lg.Warn("ensureTable: CREATE TABLE memory_job_deadletter failed", loggateway.StepID("memory.extract_fail"), loggateway.Err(err))
		return
	}
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_memory_job_dl_state_enq ON memory_job_deadletter(state, enqueued_at)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_memory_job_dl_session   ON memory_job_deadletter(session_id)`)
}

// WriteMemoryDeadLetter implements biz.MemoryDeadLetterSink (H-01).
// Persists a dropped job (goroutine-safe, best-effort).
func (r *MemoryJobDeadLetterRepo) WriteMemoryDeadLetter(
	req biz.MemoryDeadLetterRequest,
	reason biz.MemoryDeadLetterReason,
	lastErr string,
) {
	if r == nil || r.data == nil {
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"app_name":            req.AppName,
		"session_id":          req.SessionID,
		"user_id":             req.UserID,
		"feedback_message_id": req.FeedbackMessageID,
		"feedback_rating":     req.FeedbackRating,
		"feedback_comment":    req.FeedbackComment,
		"priority":            req.Priority,
		"tenant_id":           req.TenantID,
	})
	now := time.Now().UnixMilli()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(`
UPDATE memory_job_deadletter
   SET attempts = attempts + 1,
       last_error = ?,
       failed_at = ?
 WHERE session_id = ?
   AND app_name = ?
   AND priority = ?
   AND state = 'pending'
   AND attempts < 3`), lastErr, now, req.SessionID, req.AppName, int(req.Priority))
	if err == nil {
		if n, _ := res.RowsAffected(); n > 0 {
			return
		}
	}
	_, err = r.data.RWDB().WriteDB(ctx).ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(`
INSERT INTO memory_job_deadletter
  (enqueued_at, failed_at, session_id, app_name, user_id, feedback_msg_id,
   payload_json, drop_reason, priority, attempts, last_error, state)
VALUES (?,?,?,?,?,?,?,?,?,0,?,'pending')`),
		now, now,
		req.SessionID, req.AppName, req.UserID, req.FeedbackMessageID,
		string(payload), string(reason), int(req.Priority), lastErr,
	)
	if err != nil {
		r.data.lg.Warn("WriteMemoryDeadLetter: insert failed", loggateway.StepID("memory.extract_fail"), loggateway.Str("reason", string(reason)), loggateway.Err(err))
	}
}

// ListDeadLetters returns pending dead-letter jobs ordered by enqueued_at.
func (r *MemoryJobDeadLetterRepo) ListDeadLetters(ctx context.Context, state string, limit int) ([]biz.MemoryDeadLetterEntry, error) {
	if state == "" {
		state = "pending"
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(`
SELECT id, enqueued_at, failed_at, session_id, app_name, drop_reason, priority, attempts, state, last_error
FROM memory_job_deadletter
WHERE state = ?
ORDER BY enqueued_at ASC
LIMIT ?`), state, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []biz.MemoryDeadLetterEntry
	for rows.Next() {
		var row biz.MemoryDeadLetterEntry
		var enqueuedMs, failedMs int64
		if err := rows.Scan(&row.ID, &enqueuedMs, &failedMs,
			&row.SessionID, &row.AppName, &row.DropReason,
			&row.Priority, &row.Attempts, &row.State, &row.LastError); err != nil {
			return nil, err
		}
		row.EnqueuedAt = time.UnixMilli(enqueuedMs).UTC()
		row.FailedAt = time.UnixMilli(failedMs).UTC()
		out = append(out, row)
	}
	return out, rows.Err()
}

// MarkDeadLetterReplayed marks a dead-letter job as replayed.
func (r *MemoryJobDeadLetterRepo) MarkDeadLetterReplayed(ctx context.Context, id int64) error {
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE memory_job_deadletter SET state='replayed', attempts=attempts+1 WHERE id=?`), id)
	return err
}

// MarkDeadLetterAbandoned marks a dead-letter job as permanently abandoned.
func (r *MemoryJobDeadLetterRepo) MarkDeadLetterAbandoned(ctx context.Context, id int64, reason string) error {
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE memory_job_deadletter SET state='abandoned', last_error=?, attempts=attempts+1 WHERE id=?`), reason, id)
	return err
}

func (r *MemoryJobDeadLetterRepo) GetDeadLetter(ctx context.Context, id int64) (biz.MemoryDeadLetterEntry, error) {
	db := r.data.RWDB().ReadDB(ctx)
	if db == nil {
		return biz.MemoryDeadLetterEntry{}, apierror.NotFound(apierror.DomainMemory, "not found")
	}
	var row biz.MemoryDeadLetterEntry
	var enqueuedMs, failedMs int64
	err := queryRowScan(ctx, db, r.data.Dialect().RenumberPlaceholders(`
SELECT id, enqueued_at, failed_at, session_id, app_name, drop_reason, priority, attempts, state, last_error
FROM memory_job_deadletter WHERE id=?`), []any{id},
		&row.ID, &enqueuedMs, &failedMs,
		&row.SessionID, &row.AppName, &row.DropReason,
		&row.Priority, &row.Attempts, &row.State, &row.LastError)
	if err != nil {
		return biz.MemoryDeadLetterEntry{}, err
	}
	row.EnqueuedAt = time.UnixMilli(enqueuedMs).UTC()
	row.FailedAt = time.UnixMilli(failedMs).UTC()
	return row, nil
}

// CountDeadLettersByState returns counts grouped by state.
func (r *MemoryJobDeadLetterRepo) CountDeadLettersByState(ctx context.Context) (pending, replayed, abandoned int64, err error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT state, COUNT(*) FROM memory_job_deadletter GROUP BY state`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var state string
		var n int64
		if scanErr := rows.Scan(&state, &n); scanErr != nil {
			continue
		}
		switch state {
		case "pending":
			pending = n
		case "replayed":
			replayed = n
		case "abandoned":
			abandoned = n
		}
	}
	err = rows.Err()
	return
}

// ReplayDeadLetterIntoQueue re-enqueues a pending dead-letter job via the provided
// enqueue function. On success marks the row as 'replayed'.
//
// The enqueue func accepts (sessionID, appName, userID, feedbackMsgID string,
// priority biz.MemoryJobPriority) so the data layer does not import memory/trpc (H-01).
func (r *MemoryJobDeadLetterRepo) ReplayDeadLetterIntoQueue(
	ctx context.Context,
	id int64,
	enqueue func(sessionID, appName, userID, feedbackMsgID string, priority biz.MemoryJobPriority),
) error {
	db := r.data.RWDB().ReadDB(ctx)
	if db == nil {
		return nil
	}
	var payloadJSON, sessionID, appName, userID, feedbackMsgID string
	var priority int
	var enqueuedMs int64
	err := queryRowScan(ctx, db,
		r.data.Dialect().RenumberPlaceholders(`SELECT payload_json, session_id, app_name, user_id, feedback_msg_id, priority, enqueued_at
         FROM memory_job_deadletter WHERE id=? AND state='pending'`), []any{id},
		&payloadJSON, &sessionID, &appName, &userID, &feedbackMsgID, &priority, &enqueuedMs)
	if ae, ok := apierror.From(err); ok && ae.Code == apierror.CodeNotFound {
		return nil // already replayed or abandoned
	}
	if err != nil {
		return err
	}
	enqueue(sessionID, appName, userID, feedbackMsgID, biz.MemoryJobPriority(priority))
	return r.MarkDeadLetterReplayed(ctx, id)
}
