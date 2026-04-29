// Package sqlite implements Conversation-driven persistence for the shared
// monolithic schema tables sessions and messages (migration #25). The app uses a
// single SQLite file; Store wraps the same *sql.DB as repository.SQLiteRepository.
package sqlite
