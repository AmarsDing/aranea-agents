package event

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// defaultReplayLimit is the default maximum number of events returned by Replay
// when the caller does not specify a limit.
const defaultReplayLimit = 100

// PostgresEventStore persists event envelopes to Postgres for cross-process
// replay (WS reconnect) and durability. Complements the in-process event.Bus
// and WAL (which only covers Critical events).
//
// The store is idempotent on event_id: saving the same envelope twice is a
// no-op (ON CONFLICT DO NOTHING). Replay returns envelopes ordered by
// created_at ASC, optionally filtered to those created after a given event.
//
// Stability:evolving
type PostgresEventStore struct {
	db *sql.DB
	lg loggateway.Logger
}

// NewPostgresEventStore creates a Postgres-backed event store.
// db must be a Postgres connection (from data.Data.Postgres()).
// The schema is created idempotently on construction.
func NewPostgresEventStore(db *sql.DB, lg loggateway.Logger) (*PostgresEventStore, error) {
	if db == nil {
		return nil, apierror.BadRequest(apierror.DomainEventStore, "db is nil")
	}
	s := &PostgresEventStore{
		db: db,
	}
	if lg != nil {
		s.lg = lg.With(loggateway.Domain("event_store"))
	}
	if err := s.EnsureSchema(context.Background()); err != nil {
		return nil, err
	}
	if s.lg != nil {
		s.lg.Info("event_store: postgres-backed store initialized")
	}
	return s, nil
}

// EnsureSchema creates the event_store table and indexes if they don't exist
// (idempotent). Safe to call multiple times.
func (s *PostgresEventStore) EnsureSchema(ctx context.Context) error {
	const ddl = `
	CREATE TABLE IF NOT EXISTS event_store (
		id BIGSERIAL PRIMARY KEY,
		event_id VARCHAR(64) NOT NULL,
		session_id VARCHAR(64) NOT NULL,
		run_id VARCHAR(64),
		envelope_type VARCHAR(64) NOT NULL,
		payload JSONB NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_event_store_event_id ON event_store (event_id);
	CREATE INDEX IF NOT EXISTS idx_event_store_session_created ON event_store (session_id, created_at);
	CREATE INDEX IF NOT EXISTS idx_event_store_type ON event_store (envelope_type);
	`
	if _, err := s.db.ExecContext(ctx, ddl); err != nil {
		return apierror.Wrap(err, apierror.CodeInternal, apierror.DomainEventStore)
	}
	return nil
}

// Save persists an envelope to Postgres. Idempotent on event_id
// (ON CONFLICT DO NOTHING — duplicate saves are silently ignored).
func (s *PostgresEventStore) Save(ctx context.Context, env *contract.Envelope) error {
	if env == nil {
		return apierror.BadRequest(apierror.DomainEventStore, "envelope is nil")
	}
	payload, err := json.Marshal(env)
	if err != nil {
		return apierror.Wrap(err, apierror.CodeInternal, apierror.DomainEventStore)
	}
	const insertSQL = `
	INSERT INTO event_store (event_id, session_id, run_id, envelope_type, payload, created_at)
	VALUES ($1, $2, $3, $4, $5, $6)
	ON CONFLICT (event_id) DO NOTHING`
	createdAt := envelopeCreatedAt(env)
	if _, err := s.db.ExecContext(ctx, insertSQL,
		env.ID, env.SessionID, env.RequestID, string(env.Type), payload, createdAt,
	); err != nil {
		return apierror.Wrap(err, apierror.CodeInternal, apierror.DomainEventStore)
	}
	return nil
}

// Replay returns envelopes for a session created after afterEventID (exclusive).
// If afterEventID is empty, returns the most recent N events (default 100).
// Results are ordered by created_at ASC.
func (s *PostgresEventStore) Replay(ctx context.Context, sessionID string, afterEventID string, limit int) ([]*contract.Envelope, error) {
	if limit <= 0 {
		limit = defaultReplayLimit
	}
	const replaySQL = `
	SELECT payload FROM event_store
	WHERE session_id = $1
	  AND ($2 = '' OR created_at > (SELECT created_at FROM event_store WHERE event_id = $2))
	ORDER BY created_at ASC
	LIMIT $3`
	rows, err := s.db.QueryContext(ctx, replaySQL, sessionID, afterEventID, limit)
	if err != nil {
		return nil, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainEventStore)
	}
	defer rows.Close()

	envelopes := make([]*contract.Envelope, 0)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainEventStore)
		}
		env := &contract.Envelope{}
		if err := json.Unmarshal(payload, env); err != nil {
			return nil, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainEventStore)
		}
		envelopes = append(envelopes, env)
	}
	if err := rows.Err(); err != nil {
		return nil, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainEventStore)
	}
	return envelopes, nil
}

// Cleanup deletes events older than the given time.
func (s *PostgresEventStore) Cleanup(ctx context.Context, before time.Time) error {
	const cleanupSQL = `DELETE FROM event_store WHERE created_at < $1`
	if _, err := s.db.ExecContext(ctx, cleanupSQL, before.UTC()); err != nil {
		return apierror.Wrap(err, apierror.CodeInternal, apierror.DomainEventStore)
	}
	return nil
}

// Exists checks if an event with the given event_id has already been persisted
// to the EventStore. Implements EventStoreExistChecker for idempotent WAL
// recovery — when Recover replays an unpublished Critical event, this check
// avoids re-publishing if the downstream consumer already persisted it before
// the crash (AS-EVT-01 post-publish failure scenario).
//
// Returns false on query error to fall back to republishing (safer than
// silently dropping a Critical event).
//
// Stability:evolving
func (s *PostgresEventStore) Exists(ctx context.Context, eventID string) bool {
	if eventID == "" {
		return false
	}
	const existsSQL = `SELECT 1 FROM event_store WHERE event_id = $1 LIMIT 1`
	var one int
	err := s.db.QueryRowContext(ctx, existsSQL, eventID).Scan(&one)
	if err != nil {
		// sql.ErrNoRows → not found, return false.
		// Other errors → log and return false to fall back to republishing
		// (republishing is safer than dropping a Critical event).
		if s.lg != nil && err != sql.ErrNoRows {
			s.lg.Warn("event_store: Exists query failed, falling back to republish",
				loggateway.Str("event_id", eventID),
				loggateway.Err(err),
			)
		}
		return false
	}
	return true
}

// Compile-time check that PostgresEventStore implements EventStoreExistChecker.
var _ EventStoreExistChecker = (*PostgresEventStore)(nil)

// envelopeCreatedAt parses env.Timestamp (RFC3339Nano) into a time.Time.
// Falls back to time.Now().UTC() when the timestamp is missing or unparseable.
func envelopeCreatedAt(env *contract.Envelope) time.Time {
	if env.Timestamp == "" {
		return time.Now().UTC()
	}
	t, err := time.Parse(time.RFC3339Nano, env.Timestamp)
	if err != nil {
		return time.Now().UTC()
	}
	return t.UTC()
}
