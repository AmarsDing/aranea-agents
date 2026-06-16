package event

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"aranea-agents/pkg/loggateway"

	frameworkwal "trpc.group/trpc-go/trpc-agent-go/event/wal"
)

// sqliteWALStorage adapts *sql.DB to the framework wal.Storage interface.
type sqliteWALStorage struct {
	db *sql.DB
	lg loggateway.Logger
}

// newSQLiteWALStorage creates a new SQLite-backed WAL storage.
func newSQLiteWALStorage(ctx context.Context, db *sql.DB, lg loggateway.Logger) (*sqliteWALStorage, error) {
	s := &sqliteWALStorage{db: db, lg: lg}
	if err := s.ensureSchema(ctx); err != nil {
		return nil, fmt.Errorf("event_wal: ensure schema: %w", err)
	}
	return s, nil
}

func (s *sqliteWALStorage) ensureSchema(ctx context.Context) error {
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
	_, err := s.db.ExecContext(ctx, ddl)
	return err
}

func (s *sqliteWALStorage) Insert(ctx context.Context, entry frameworkwal.Entry) error {
	const insertSQL = `INSERT OR IGNORE INTO event_wal (id, envelope_json, created_at) VALUES (?, ?, ?)`
	now := entry.CreatedAt.Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, insertSQL, entry.ID, entry.EventJSON, now)
	return err
}

func (s *sqliteWALStorage) MarkPublished(ctx context.Context, id string, publishedAt time.Time) error {
	const updateSQL = `UPDATE event_wal SET published = 1, published_at = ? WHERE id = ?`
	now := publishedAt.Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, updateSQL, now, id)
	return err
}

func (s *sqliteWALStorage) ListUnpublished(ctx context.Context) ([]frameworkwal.Entry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, envelope_json, created_at FROM event_wal WHERE published = 0 ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []frameworkwal.Entry
	for rows.Next() {
		var id, envJSON string
		var createdAtStr string
		if err := rows.Scan(&id, &envJSON, &createdAtStr); err != nil {
			if s.lg != nil {
				s.lg.Warn("event_wal: scan row failed", loggateway.Err(err))
			}
			continue
		}
		createdAt, err := time.Parse(time.RFC3339Nano, createdAtStr)
		if err != nil {
			if s.lg != nil {
				s.lg.Warn("event_wal: parse created_at failed",
					loggateway.Str("id", id),
					loggateway.Str("created_at", createdAtStr),
					loggateway.Err(err),
				)
			}
			createdAt = time.Now().UTC()
		}
		entries = append(entries, frameworkwal.Entry{
			ID:        id,
			EventJSON: envJSON,
			CreatedAt: createdAt,
		})
	}
	return entries, nil
}

func (s *sqliteWALStorage) PurgePublished(ctx context.Context, cutoff time.Time) (int64, error) {
	cutoffStr := cutoff.Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM event_wal WHERE published = 1 AND created_at < ?`, cutoffStr)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *sqliteWALStorage) Close() error {
	// Don't close the shared *sql.DB
	return nil
}
