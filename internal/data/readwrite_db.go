package data

import (
	"context"
	"database/sql"
	"errors"

	"aranea-agents/internal/data/ent"
)

// ErrRawDBUnavailable is returned when a raw SQL operation is attempted on a
// Data instance that was not initialized with a *sql.DB (e.g. seed-pack paths
// that only have an *ent.Client). This prevents nil-pointer panics.
var ErrRawDBUnavailable = errors.New("raw SQL database handle is not available in this context")

// nilExecer returns ErrRawDBUnavailable for every operation so callers get a
// clear error instead of a nil-pointer panic when RWDB is not initialized.
type nilExecer struct{}

func (nilExecer) ExecContext(_ context.Context, _ string, _ ...any) (sql.Result, error) {
	return nil, ErrRawDBUnavailable
}

func (nilExecer) QueryContext(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
	return nil, ErrRawDBUnavailable
}

// ReadWriteDB encapsulates read/write *sql.DB selection with automatic
// transaction awareness. Read operations use the read-only database (readDB),
// Write operations use the write database (rawDB). Both fall back to the
// transaction's Ent client when a transaction is active in the context,
// ensuring raw SQL queries participate in the same Ent-managed transaction.
type ReadWriteDB struct {
	write *sql.DB // rawDB (MaxOpenConns=1, SQLite single-writer)
	read  *sql.DB // readDB (MaxOpenConns=2, WAL concurrent reads)
}

// NewReadWriteDB creates a ReadWriteDB with the given write and read *sql.DB handles.
func NewReadWriteDB(write, read *sql.DB) *ReadWriteDB {
	return &ReadWriteDB{write: write, read: read}
}

// ReadDB returns the appropriate database handle for read operations.
// If a transaction is active in the context, it returns the transaction's
// Ent client (which satisfies execer) so raw SQL reads participate in the tx.
// Otherwise, it returns the read-only *sql.DB. If the ReadWriteDB or its read
// handle is nil (e.g. seed-pack path with only an ent.Client), it returns a
// nilExecer that yields ErrRawDBUnavailable instead of panicking.
func (db *ReadWriteDB) ReadDB(ctx context.Context) execer {
	if tx, ok := ctx.Value(rawTxKey{}).(*ent.Tx); ok {
		return tx.Client()
	}
	if db == nil || db.read == nil {
		return nilExecer{}
	}
	return db.read
}

// WriteDB returns the appropriate database handle for write operations.
// If a transaction is active in the context, it returns the transaction's
// Ent client (which satisfies execer) so raw SQL writes participate in the tx.
// Otherwise, it returns the write *sql.DB. If the ReadWriteDB or its write
// handle is nil (e.g. seed-pack path with only an ent.Client), it returns a
// nilExecer that yields ErrRawDBUnavailable instead of panicking.
func (db *ReadWriteDB) WriteDB(ctx context.Context) execer {
	if tx, ok := ctx.Value(rawTxKey{}).(*ent.Tx); ok {
		return tx.Client()
	}
	if db == nil || db.write == nil {
		return nilExecer{}
	}
	return db.write
}

// WriteHandle returns the underlying write *sql.DB without transaction awareness.
// Use this only for DDL operations (CREATE TABLE, ALTER TABLE) or as TxExecerFromCtx fallback.
func (db *ReadWriteDB) WriteHandle() *sql.DB {
	if db == nil {
		return nil
	}
	return db.write
}

// ReadHandle returns the underlying read *sql.DB without transaction awareness.
// Use this only when a raw *sql.DB is required (e.g., for QueryRowContext compatibility).
func (db *ReadWriteDB) ReadHandle() *sql.DB {
	if db == nil {
		return nil
	}
	return db.read
}
