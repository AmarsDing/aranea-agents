package main

// framework_schema.go — Creates Postgres schema for framework-managed tables
// that are not in Ent Schema nor in the DDL migration registry.
//
// These tables are normally created lazily by the application on startup:
//   - trpc_* tables: by trpc-agent-go session/postgres Service.initDB()
//   - graph_checkpoints/graph_checkpoint_writes: by graph/checkpoint/postgres NewSaver()
//   - memory_job_deadletter: by NewMemoryJobDeadLetterRepo().ensureTable()
//   - monitor_trace_spans: by monitorRepo.EnsureTraceSchema()
//   - vector_embeddings: by NewPgVectorStore().ensureTable()
//   - event_wal: by ensurePostgresPhase1Schema()
//
// During migration we cannot start the full app, so we replicate the DDL here.
// The DDL must stay in sync with the sources noted above.

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// sqliteToPostgresTableMap maps SQLite table names to Postgres table names
// when they differ. This is used by the migrator and validator to resolve
// the target table name for migration.
//
// Known mismatches:
//   - checkpoints -> graph_checkpoints (graph/checkpoint/postgres adds "graph_" prefix)
//   - checkpoint_writes -> graph_checkpoint_writes (same reason)
var sqliteToPostgresTableMap = map[string]string{
	"checkpoints":       "graph_checkpoints",
	"checkpoint_writes": "graph_checkpoint_writes",
}

// resolveTargetTableName returns the Postgres table name for a given SQLite table.
// If no mapping exists, the SQLite name is used as-is.
func resolveTargetTableName(sqliteTable string) string {
	if pg, ok := sqliteToPostgresTableMap[sqliteTable]; ok {
		return pg
	}
	return sqliteTable
}

// initFrameworkSchema creates all framework-managed tables on Postgres.
// Idempotent — all statements use IF NOT EXISTS.
//
// Tables created:
//  1. event_wal (from 20260617_postgres_phase1.sql)
//  2. graph_checkpoints, graph_checkpoint_writes (from graph/checkpoint/postgres/saver.go)
//  3. memory_job_deadletter (from internal/data/memory_job_deadletter.go)
//  4. monitor_trace_spans (from internal/data/monitor_trace.go)
//  5. trpc_app_states, trpc_session_events, trpc_session_states,
//     trpc_session_summaries, trpc_session_track_events, trpc_user_states
//     (from pkg/trpc-agent-go/session/postgres/init.go)
//  6. vector_embeddings (from internal/data/vector/pgvector.go)
func initFrameworkSchema(ctx context.Context, pgDB *sql.DB) error {
	if pgDB == nil {
		return fmt.Errorf("postgres db is nil")
	}

	// Execute each DDL block. We use per-statement execution (not multi-statement)
	// because some statements contain semicolons inside string literals
	// (e.g. CHECK constraints) which would confuse a naive splitter.
	for i, stmt := range frameworkSchemaDDL {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		if _, err := pgDB.ExecContext(ctx, stmt); err != nil {
			// Tolerate "already exists" errors for idempotency.
			if isPgAlreadyExists(err) {
				fmt.Printf("  [SKIP] framework DDL #%d (already exists): %s\n", i+1, truncateDDL(stmt))
				continue
			}
			return fmt.Errorf("framework DDL #%d failed: %w\n---\n%s", i+1, err, truncateDDL(stmt))
		}
	}
	return nil
}

// isPgAlreadyExists reports whether err is a Postgres "already exists" error.
func isPgAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// lib/pq error messages for duplicate objects.
	// We match on substrings because we don't want to import lib/pq here
	// (the migrate tool already imports it via the driver).
	return strings.Contains(msg, "already exists") ||
		strings.Contains(msg, "duplicate table") ||
		strings.Contains(msg, "duplicate column") ||
		strings.Contains(msg, "duplicate object") ||
		strings.Contains(msg, "42701") || // duplicate_column
		strings.Contains(msg, "42P07") || // duplicate_table
		strings.Contains(msg, "42710") // duplicate_object
}

// truncateDDL returns the first 80 chars of a DDL statement for logging.
func truncateDDL(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	if len(s) > 80 {
		return s[:80] + "..."
	}
	return s
}

// ensurePgvectorExtension creates the pgvector extension if not present.
// This is required for the vector_embeddings table.
// Returns nil if the extension cannot be created (e.g. not installed on the server);
// the vector_embeddings table creation will then fail, which is non-fatal.
func ensurePgvectorExtension(ctx context.Context, pgDB *sql.DB) error {
	_, err := pgDB.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS vector`)
	if err != nil {
		return fmt.Errorf("create pgvector extension (install pgvector on the server first): %w", err)
	}
	return nil
}

// frameworkSchemaDDL contains all DDL statements for framework-managed tables.
// Each entry is a single SQL statement (no multi-statement strings).
//
// Sources (must stay in sync):
//   - event_wal: internal/data/sql/migrations/20260617_postgres_phase1.sql
//   - graph_checkpoints/graph_checkpoint_writes: pkg/trpc-agent-go/graph/checkpoint/postgres/saver.go
//   - memory_job_deadletter: internal/data/memory_job_deadletter.go
//   - monitor_trace_spans: internal/data/monitor_trace.go
//   - trpc_*: pkg/trpc-agent-go/session/postgres/init.go (with "trpc_" prefix)
//   - vector_embeddings: internal/data/vector/pgvector.go (dim=1536)
var frameworkSchemaDDL = []string{
	// 1. event_wal — Critical event Write-Ahead Log (WBPF)
	`CREATE TABLE IF NOT EXISTS event_wal (
    id TEXT PRIMARY KEY,
    envelope_json TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    published INTEGER NOT NULL DEFAULT 0
)`,
	`CREATE INDEX IF NOT EXISTS idx_event_wal_unpublished
    ON event_wal(published, created_at)`,

	// 2. graph_checkpoints — Graph execution checkpoint storage
	//    Source: pkg/trpc-agent-go/graph/checkpoint/postgres/saver.go pgCreateCheckpoints
	`CREATE TABLE IF NOT EXISTS graph_checkpoints (
    lineage_id TEXT NOT NULL,
    checkpoint_ns TEXT NOT NULL,
    checkpoint_id TEXT NOT NULL,
    parent_checkpoint_id TEXT,
    ts BIGINT NOT NULL,
    checkpoint_json TEXT NOT NULL,
    metadata_json TEXT NOT NULL,
    PRIMARY KEY (lineage_id, checkpoint_ns, checkpoint_id)
)`,

	// 3. graph_checkpoint_writes — Graph execution write log
	//    Source: pkg/trpc-agent-go/graph/checkpoint/postgres/saver.go pgCreateWrites
	`CREATE TABLE IF NOT EXISTS graph_checkpoint_writes (
    lineage_id TEXT NOT NULL,
    checkpoint_ns TEXT NOT NULL,
    checkpoint_id TEXT NOT NULL,
    task_id TEXT NOT NULL,
    idx INTEGER NOT NULL,
    channel TEXT NOT NULL,
    value_json TEXT NOT NULL,
    task_path TEXT,
    seq BIGINT NOT NULL,
    PRIMARY KEY (lineage_id, checkpoint_ns, checkpoint_id, task_id, idx)
)`,

	// 4. memory_job_deadletter — Persistent dead-letter store for AutoMemory jobs
	//    Source: internal/data/memory_job_deadletter.go ensureTable (Postgres variant)
	`CREATE TABLE IF NOT EXISTS memory_job_deadletter (
    id BIGSERIAL PRIMARY KEY,
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
)`,
	`CREATE INDEX IF NOT EXISTS idx_memory_job_dl_state_enq ON memory_job_deadletter(state, enqueued_at)`,
	`CREATE INDEX IF NOT EXISTS idx_memory_job_dl_session   ON memory_job_deadletter(session_id)`,

	// 5. monitor_trace_spans — Trace span storage for observability
	//    Source: internal/data/monitor_trace.go EnsureTraceSchema (Postgres variant)
	//    NOTE: started_at/ended_at store nanosecond timestamps (e.g. 1780850874257).
	//    Postgres INTEGER is 4 bytes (max ~2.1B), which overflows. Must use BIGINT.
	//    This fixes a bug in the source DDL which uses INTEGER for both dialects.
	`CREATE TABLE IF NOT EXISTS monitor_trace_spans (
    id BIGSERIAL PRIMARY KEY,
    trace_id TEXT NOT NULL,
    span_id TEXT NOT NULL,
    parent_span_id TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL,
    name TEXT NOT NULL,
    started_at BIGINT NOT NULL,
    ended_at BIGINT NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'running',
    attributes_json TEXT NOT NULL DEFAULT '',
    error_json TEXT NOT NULL DEFAULT '',
    UNIQUE(trace_id, span_id)
)`,
	`CREATE INDEX IF NOT EXISTS idx_monitor_trace_spans_trace_id ON monitor_trace_spans(trace_id)`,
	`CREATE INDEX IF NOT EXISTS idx_monitor_trace_spans_kind ON monitor_trace_spans(kind)`,

	// 6. trpc_session_states — trpc-agent-go session state storage
	//    Source: pkg/trpc-agent-go/session/postgres/init.go sqlCreateSessionStatesTable
	//    Table prefix "trpc_" is applied (WithTablePrefix("trpc_")).
	`CREATE TABLE IF NOT EXISTS trpc_session_states (
    id BIGSERIAL PRIMARY KEY,
    app_name VARCHAR(255) NOT NULL,
    user_id VARCHAR(255) NOT NULL,
    session_id VARCHAR(255) NOT NULL,
    state JSONB DEFAULT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP DEFAULT NULL,
    deleted_at TIMESTAMP DEFAULT NULL
)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_trpc_session_states_unique_active
    ON trpc_session_states(app_name, user_id, session_id)
    WHERE deleted_at IS NULL`,
	`CREATE INDEX IF NOT EXISTS idx_trpc_session_states_expires
    ON trpc_session_states(expires_at) WHERE expires_at IS NOT NULL`,

	// 7. trpc_session_events
	//    Source: pkg/trpc-agent-go/session/postgres/init.go sqlCreateSessionEventsTable
	`CREATE TABLE IF NOT EXISTS trpc_session_events (
    id BIGSERIAL PRIMARY KEY,
    app_name VARCHAR(255) NOT NULL,
    user_id VARCHAR(255) NOT NULL,
    session_id VARCHAR(255) NOT NULL,
    event JSONB NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP DEFAULT NULL,
    deleted_at TIMESTAMP DEFAULT NULL
)`,
	`CREATE INDEX IF NOT EXISTS idx_trpc_session_events_lookup
    ON trpc_session_events(app_name, user_id, session_id, created_at)`,
	`CREATE INDEX IF NOT EXISTS idx_trpc_session_events_expires
    ON trpc_session_events(expires_at) WHERE expires_at IS NOT NULL`,

	// 8. trpc_session_track_events
	//    Source: pkg/trpc-agent-go/session/postgres/init.go sqlCreateSessionTrackEventsTable
	`CREATE TABLE IF NOT EXISTS trpc_session_track_events (
    id BIGSERIAL PRIMARY KEY,
    app_name VARCHAR(255) NOT NULL,
    user_id VARCHAR(255) NOT NULL,
    session_id VARCHAR(255) NOT NULL,
    track VARCHAR(255) NOT NULL,
    event JSONB NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP DEFAULT NULL,
    deleted_at TIMESTAMP DEFAULT NULL
)`,
	`CREATE INDEX IF NOT EXISTS idx_trpc_session_track_events_lookup
    ON trpc_session_track_events(app_name, user_id, session_id, track, created_at)`,
	`CREATE INDEX IF NOT EXISTS idx_trpc_session_track_events_expires
    ON trpc_session_track_events(expires_at) WHERE expires_at IS NOT NULL`,

	// 9. trpc_session_summaries
	//    Source: pkg/trpc-agent-go/session/postgres/init.go sqlCreateSessionSummariesTable
	`CREATE TABLE IF NOT EXISTS trpc_session_summaries (
    id BIGSERIAL PRIMARY KEY,
    app_name VARCHAR(255) NOT NULL,
    user_id VARCHAR(255) NOT NULL,
    session_id VARCHAR(255) NOT NULL,
    filter_key VARCHAR(255) NOT NULL DEFAULT '',
    summary JSONB DEFAULT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP DEFAULT NULL,
    deleted_at TIMESTAMP DEFAULT NULL
)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_trpc_session_summaries_unique_active
    ON trpc_session_summaries(app_name, user_id, session_id, filter_key)
    WHERE deleted_at IS NULL`,
	`CREATE INDEX IF NOT EXISTS idx_trpc_session_summaries_expires
    ON trpc_session_summaries(expires_at) WHERE expires_at IS NOT NULL`,

	// 10. trpc_app_states
	//     Source: pkg/trpc-agent-go/session/postgres/init.go sqlCreateAppStatesTable
	`CREATE TABLE IF NOT EXISTS trpc_app_states (
    id BIGSERIAL PRIMARY KEY,
    app_name VARCHAR(255) NOT NULL,
    key VARCHAR(255) NOT NULL,
    value TEXT DEFAULT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP DEFAULT NULL,
    deleted_at TIMESTAMP DEFAULT NULL
)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_trpc_app_states_unique_active
    ON trpc_app_states(app_name, key)
    WHERE deleted_at IS NULL`,
	`CREATE INDEX IF NOT EXISTS idx_trpc_app_states_expires
    ON trpc_app_states(expires_at) WHERE expires_at IS NOT NULL`,

	// 11. trpc_user_states
	//     Source: pkg/trpc-agent-go/session/postgres/init.go sqlCreateUserStatesTable
	`CREATE TABLE IF NOT EXISTS trpc_user_states (
    id BIGSERIAL PRIMARY KEY,
    app_name VARCHAR(255) NOT NULL,
    user_id VARCHAR(255) NOT NULL,
    key VARCHAR(255) NOT NULL,
    value TEXT DEFAULT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP DEFAULT NULL,
    deleted_at TIMESTAMP DEFAULT NULL
)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_trpc_user_states_unique_active
    ON trpc_user_states(app_name, user_id, key)
    WHERE deleted_at IS NULL`,
	`CREATE INDEX IF NOT EXISTS idx_trpc_user_states_expires
    ON trpc_user_states(expires_at) WHERE expires_at IS NOT NULL`,

	// 12. vector_embeddings — Vector embedding storage (requires pgvector extension)
	//     Source: internal/data/vector/pgvector.go ensureTable (dim=1536)
	//     NOTE: SQLite schema uses (embedding_json TEXT, meta_json TEXT) which is
	//     incompatible with Postgres (embedding vector, meta JSONB). Data migration
	//     is skipped for this table; vectors must be re-embedded after migration.
	`CREATE TABLE IF NOT EXISTS vector_embeddings (
    id TEXT PRIMARY KEY,
    embedding vector(1536) NOT NULL,
    meta JSONB NOT NULL DEFAULT '{}'
)`,
}
