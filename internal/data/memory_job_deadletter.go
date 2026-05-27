package data

// memory_job_deadletter.go — MEM-OPT-03: persistent dead-letter store for AutoMemory jobs.
//
// Implements trpcmem.MemoryDeadLetterSink using the raw SQLite database.
// Table is created lazily on first write.

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
)

// MemoryJobDeadLetterRepo persists dropped AutoMemory jobs for later replay.
// Implements biz.MemoryDeadLetterSink (H-01: data layer no longer imports memory/trpc).
type MemoryJobDeadLetterRepo struct {
	data *Data
}

var _ biz.MemoryDeadLetterSink = (*MemoryJobDeadLetterRepo)(nil)

// NewMemoryJobDeadLetterRepo creates the dead-letter repository.
func NewMemoryJobDeadLetterRepo(d *Data) *MemoryJobDeadLetterRepo {
	r := &MemoryJobDeadLetterRepo{data: d}
	r.ensureTable()
	return r
}

func (r *MemoryJobDeadLetterRepo) ensureTable() {
	db := r.data.RawDB()
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS memory_job_deadletter (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
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
)`)
	if err != nil {
		event.SysLogWarn("system.auto_memory.extract_fail", "ensureTable: CREATE TABLE memory_job_deadletter failed", event.P("error", err.Error()))
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
	_, err := r.data.RawDB().ExecContext(ctx, `
INSERT INTO memory_job_deadletter
  (enqueued_at, failed_at, session_id, app_name, user_id, feedback_msg_id,
   payload_json, drop_reason, priority, attempts, last_error, state)
VALUES (?,?,?,?,?,?,?,?,?,0,?,'pending')`,
		now, now,
		req.SessionID, req.AppName, req.UserID, req.FeedbackMessageID,
		string(payload), string(reason), int(req.Priority), lastErr,
	)
	if err != nil {
		event.SysLogWarn("system.auto_memory.extract_fail", "WriteMemoryDeadLetter: insert failed", event.P("reason", string(reason)), event.P("error", err.Error()))
	}
}

// MemoryDeadLetterListResult is returned by ListDeadLetters.
type MemoryDeadLetterListResult struct {
	ID         int64
	EnqueuedAt time.Time
	FailedAt   time.Time
	SessionID  string
	AppName    string
	DropReason string
	Priority   int
	Attempts   int
	State      string
	LastError  string
}

// ListDeadLetters returns pending dead-letter jobs ordered by enqueued_at.
func (r *MemoryJobDeadLetterRepo) ListDeadLetters(ctx context.Context, state string, limit int) ([]MemoryDeadLetterListResult, error) {
	if state == "" {
		state = "pending"
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.data.RawDB().QueryContext(ctx, `
SELECT id, enqueued_at, failed_at, session_id, app_name, drop_reason, priority, attempts, state, last_error
FROM memory_job_deadletter
WHERE state = ?
ORDER BY enqueued_at ASC
LIMIT ?`, state, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MemoryDeadLetterListResult
	for rows.Next() {
		var row MemoryDeadLetterListResult
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
	_, err := r.data.RawDB().ExecContext(ctx,
		`UPDATE memory_job_deadletter SET state='replayed', attempts=attempts+1 WHERE id=?`, id)
	return err
}

// MarkDeadLetterAbandoned marks a dead-letter job as permanently abandoned.
func (r *MemoryJobDeadLetterRepo) MarkDeadLetterAbandoned(ctx context.Context, id int64, reason string) error {
	_, err := r.data.RawDB().ExecContext(ctx,
		`UPDATE memory_job_deadletter SET state='abandoned', last_error=?, attempts=attempts+1 WHERE id=?`, reason, id)
	return err
}

// CountDeadLettersByState returns counts grouped by state.
func (r *MemoryJobDeadLetterRepo) CountDeadLettersByState(ctx context.Context) (pending, replayed, abandoned int64, err error) {
	rows, err := r.data.RawDB().QueryContext(ctx,
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
	var payloadJSON, sessionID, appName, userID, feedbackMsgID string
	var priority int
	var enqueuedMs int64
	err := r.data.RawDB().QueryRowContext(ctx,
		`SELECT payload_json, session_id, app_name, user_id, feedback_msg_id, priority, enqueued_at
         FROM memory_job_deadletter WHERE id=? AND state='pending'`, id).
		Scan(&payloadJSON, &sessionID, &appName, &userID, &feedbackMsgID, &priority, &enqueuedMs)
	if err == sql.ErrNoRows {
		return nil // already replayed or abandoned
	}
	if err != nil {
		return err
	}
	enqueue(sessionID, appName, userID, feedbackMsgID, biz.MemoryJobPriority(priority))
	return r.MarkDeadLetterReplayed(ctx, id)
}
