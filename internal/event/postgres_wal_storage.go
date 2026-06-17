package event

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"aranea-agents/pkg/loggateway"

	frameworkwal "trpc.group/trpc-go/trpc-agent-go/event/wal"
)

// postgresWALStorage adapts *sql.DB (Postgres) to the framework wal.Storage interface.
// Uses Postgres-specific SQL syntax (ON CONFLICT, TIMESTAMPTZ, $N placeholders).
type postgresWALStorage struct {
	db *sql.DB
	lg loggateway.Logger
}

// newPostgresWALStorage creates a new Postgres-backed WAL storage.
func newPostgresWALStorage(ctx context.Context, db *sql.DB, lg loggateway.Logger) (*postgresWALStorage, error) {
	s := &postgresWALStorage{db: db, lg: lg}
	if err := s.ensureSchema(ctx); err != nil {
		return nil, fmt.Errorf("event_wal: ensure postgres schema: %w", err)
	}
	return s, nil
}

func (s *postgresWALStorage) ensureSchema(ctx context.Context) error {
	const ddl = `
	CREATE TABLE IF NOT EXISTS event_wal (
		id TEXT PRIMARY KEY,
		envelope_json TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		published_at TIMESTAMPTZ,
		published INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_event_wal_unpublished ON event_wal(published, created_at);
	`
	_, err := s.db.ExecContext(ctx, ddl)
	return err
}

func (s *postgresWALStorage) Insert(ctx context.Context, entry frameworkwal.Entry) error {
	// ON CONFLICT DO NOTHING provides idempotent insert (equivalent to SQLite's INSERT OR IGNORE).
	const insertSQL = `INSERT INTO event_wal (id, envelope_json, created_at) VALUES ($1, $2, $3) ON CONFLICT (id) DO NOTHING`
	_, err := s.db.ExecContext(ctx, insertSQL, entry.ID, entry.EventJSON, entry.CreatedAt.UTC())
	return err
}

func (s *postgresWALStorage) MarkPublished(ctx context.Context, id string, publishedAt time.Time) error {
	const updateSQL = `UPDATE event_wal SET published = 1, published_at = $1 WHERE id = $2`
	_, err := s.db.ExecContext(ctx, updateSQL, publishedAt.UTC(), id)
	return err
}

func (s *postgresWALStorage) ListUnpublished(ctx context.Context) ([]frameworkwal.Entry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, envelope_json, created_at FROM event_wal WHERE published = 0 ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []frameworkwal.Entry
	for rows.Next() {
		var id, envJSON string
		var createdAt time.Time
		if err := rows.Scan(&id, &envJSON, &createdAt); err != nil {
			if s.lg != nil {
				s.lg.Warn("event_wal: postgres scan row failed", loggateway.Err(err))
			}
			continue
		}
		entries = append(entries, frameworkwal.Entry{
			ID:        id,
			EventJSON: envJSON,
			CreatedAt: createdAt.UTC(),
		})
	}
	return entries, rows.Err()
}

func (s *postgresWALStorage) PurgePublished(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM event_wal WHERE published = 1 AND created_at < $1`, cutoff.UTC())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *postgresWALStorage) Close() error {
	// Don't close the shared *sql.DB
	return nil
}
