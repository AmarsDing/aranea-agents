package data

import (
	"context"
	"database/sql"

	"aranea-agents/internal/data/ent"
)

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
// Otherwise, it returns the read-only *sql.DB.
func (db *ReadWriteDB) ReadDB(ctx context.Context) execer {
	if tx, ok := ctx.Value(rawTxKey{}).(*ent.Tx); ok {
		return tx.Client()
	}
	return db.read
}

// WriteDB returns the appropriate database handle for write operations.
// If a transaction is active in the context, it returns the transaction's
// Ent client (which satisfies execer) so raw SQL writes participate in the tx.
// Otherwise, it returns the write *sql.DB.
func (db *ReadWriteDB) WriteDB(ctx context.Context) execer {
	if tx, ok := ctx.Value(rawTxKey{}).(*ent.Tx); ok {
		return tx.Client()
	}
	return db.write
}
