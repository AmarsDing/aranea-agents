package event

// This file previously contained sqliteWALStorage, which was removed in A6
// when SQLite was dropped as a production database. Postgres is now the only
// WAL storage backend. See postgres_wal_storage.go for the active implementation.
