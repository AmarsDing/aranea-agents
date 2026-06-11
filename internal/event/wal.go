package event

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

// EventWAL provides Write-Before-Publish-Fanout (WBPF) for Critical events.
// Critical events are persisted to SQLite BEFORE being published to the Bus,
// ensuring no data loss on process crash. On restart, unpublished entries
// are recovered and republished.
//
// Stability:evolving
type EventWAL struct {
	db *sql.DB
	lg loggateway.Logger
}

// NewEventWAL creates a new WAL backed by the given SQLite database.
// The database must be opened with WAL journal mode for best crash safety.
func NewEventWAL(db *sql.DB, lg loggateway.Logger) (*EventWAL, error) {
	w := &EventWAL{db: db, lg: lg}
	if err := w.ensureSchema(context.Background()); err != nil {
		return nil, fmt.Errorf("event_wal: ensure schema: %w", err)
	}
	return w, nil
}

// ensureSchema creates the WAL table if it doesn't exist.
func (w *EventWAL) ensureSchema(ctx context.Context) error {
	const ddl = `
	CREATE TABLE IF NOT EXISTS event_wal (
		id TEXT PRIMARY KEY,
		envelope_json TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		published_at DATETIME,
		published INTEGER DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_event_wal_unpublished ON event_wal(published, created_at);
	`
	_, err := w.db.ExecContext(ctx, ddl)
	return err
}

// WriteBeforePublish persists a Critical event to WAL before publishing.
// For non-Critical events, it calls publish() directly (no WAL overhead).
// The publish callback is only called after successful WAL write.
func (w *EventWAL) WriteBeforePublish(ctx context.Context, env contract.Envelope, publish func()) error {
	if !IsCriticalWBPFType(env.Type) {
		publish()
		return nil
	}
	// 1. Serialize envelope
	raw, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("event_wal: marshal envelope: %w", err)
	}
	// 2. Write to WAL (INSERT OR IGNORE for idempotency)
	const insertSQL = `INSERT OR IGNORE INTO event_wal (id, envelope_json, created_at) VALUES (?, ?, ?)`
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := w.db.ExecContext(ctx, insertSQL, env.ID, string(raw), now); err != nil {
		return fmt.Errorf("event_wal: insert: %w", err)
	}
	// 3. Publish to Bus
	publish()
	// 4. Mark as published (synchronous, not async — avoids crash-time inconsistency)
	w.markPublished(ctx, env.ID)
	return nil
}

// markPublished marks a WAL entry as published.
func (w *EventWAL) markPublished(ctx context.Context, id string) {
	const updateSQL = `UPDATE event_wal SET published = 1, published_at = ? WHERE id = ?`
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := w.db.ExecContext(ctx, updateSQL, now, id); err != nil {
		if w.lg != nil {
			w.lg.Warn("event_wal: mark published failed",
				loggateway.Str("id", id),
				loggateway.Err(err),
			)
		}
	}
}

// Recover replays unpublished WAL entries after process restart.
// Must be called AFTER Bus and all subscribers are ready.
// Uses idempotent publish — if the event was already processed by a subscriber,
// the subscriber must handle duplicates gracefully.
func (w *EventWAL) Recover(ctx context.Context, bus contract.Bus, store EventStoreExistChecker) {
	if w == nil || w.db == nil {
		return
	}
	rows, err := w.db.QueryContext(ctx,
		`SELECT id, envelope_json FROM event_wal WHERE published = 0 ORDER BY created_at ASC`)
	if err != nil {
		if w.lg != nil {
			w.lg.Warn("event_wal: recover query failed", loggateway.Err(err))
		}
		return
	}

	// Collect entries before modifying — SQLite may lock the table during
	// an active SELECT result set, preventing the UPDATE in markPublished.
	type walEntry struct {
		id      string
		envJSON string
	}
	var entries []walEntry
	for rows.Next() {
		var id, envJSON string
		if err := rows.Scan(&id, &envJSON); err != nil {
			continue
		}
		entries = append(entries, walEntry{id: id, envJSON: envJSON})
	}
	rows.Close()

	recovered := 0
	for _, e := range entries {
		// Idempotency check: if EventStore already has this event, skip
		if store != nil && store.Exists(ctx, e.id) {
			w.markPublished(ctx, e.id)
			continue
		}
		var env contract.Envelope
		if err := json.Unmarshal([]byte(e.envJSON), &env); err != nil {
			if w.lg != nil {
				w.lg.Warn("event_wal: recover unmarshal failed",
					loggateway.Str("id", e.id),
					loggateway.Err(err),
				)
			}
			continue
		}
		bus.Publish(ctx, env)
		w.markPublished(ctx, e.id)
		recovered++
	}
	if recovered > 0 && w.lg != nil {
		w.lg.Info("event_wal: recovered events",
			loggateway.Int("count", recovered),
		)
	}
}

// PurgePublished removes published WAL entries older than the given TTL.
func (w *EventWAL) PurgePublished(ctx context.Context, ttl time.Duration) (int64, error) {
	if w == nil || w.db == nil {
		return 0, nil
	}
	cutoff := time.Now().UTC().Add(-ttl).Format(time.RFC3339)
	result, err := w.db.ExecContext(ctx,
		`DELETE FROM event_wal WHERE published = 1 AND created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// EventStoreExistChecker checks if an event already exists in the EventStore.
// Used for idempotent recovery — if the event was already persisted by the
// async persistHandler before crash, we skip re-publishing.
// Stability:evolving
type EventStoreExistChecker interface {
	Exists(ctx context.Context, eventID string) bool
}
