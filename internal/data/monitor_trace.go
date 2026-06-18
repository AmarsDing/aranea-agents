package data

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"

	"github.com/google/uuid"
)

var traceSchemaOnce sync.Once
var eventSchemaOnce sync.Once

func (r *monitorRepo) EnsureTraceSchema(ctx context.Context) error {
	var firstErr error
	d := r.data.Dialect()
	traceSchemaOnce.Do(func() {
		db := r.data.RWDB().WriteDB(ctx)
		patches := []struct {
			table string
			col   string
			ddl   string
		}{
			{"monitor_traces", "session_id", "ALTER TABLE monitor_traces ADD COLUMN session_id TEXT NOT NULL DEFAULT ''"},
			{"monitor_traces", "run_id", "ALTER TABLE monitor_traces ADD COLUMN run_id TEXT NOT NULL DEFAULT ''"},
			{"monitor_traces", "invocation_id", "ALTER TABLE monitor_traces ADD COLUMN invocation_id TEXT NOT NULL DEFAULT ''"},
			{"monitor_traces", "agent_id", "ALTER TABLE monitor_traces ADD COLUMN agent_id TEXT NOT NULL DEFAULT ''"},
			{"monitor_traces", "provider", "ALTER TABLE monitor_traces ADD COLUMN provider TEXT NOT NULL DEFAULT ''"},
			{"monitor_traces", "model", "ALTER TABLE monitor_traces ADD COLUMN model TEXT NOT NULL DEFAULT ''"},
			{"monitor_traces", "team_id", "ALTER TABLE monitor_traces ADD COLUMN team_id TEXT NOT NULL DEFAULT ''"},
			{"monitor_traces", "parent_trace_id", "ALTER TABLE monitor_traces ADD COLUMN parent_trace_id TEXT NOT NULL DEFAULT ''"},
			{"monitor_traces", "duration_ms", "ALTER TABLE monitor_traces ADD COLUMN duration_ms INTEGER NOT NULL DEFAULT 0"},
			{"monitor_traces", "span_count", "ALTER TABLE monitor_traces ADD COLUMN span_count INTEGER NOT NULL DEFAULT 0"},
			{"monitor_traces", "error_count", "ALTER TABLE monitor_traces ADD COLUMN error_count INTEGER NOT NULL DEFAULT 0"},
			{"monitor_traces", "total_tokens", "ALTER TABLE monitor_traces ADD COLUMN total_tokens INTEGER NOT NULL DEFAULT 0"},
			{"monitor_traces", "total_cost_usd", "ALTER TABLE monitor_traces ADD COLUMN total_cost_usd REAL NOT NULL DEFAULT 0.0"},
		}
		for _, p := range patches {
			has, err := columnExistsWithDialect(ctx, db, p.table, p.col, d)
			if err != nil {
				firstErr = err
				return
			}
			if has {
				continue
			}
			if _, err := db.ExecContext(ctx, p.ddl); err != nil {
				if !d.AlreadyExistsErr(err) {
					firstErr = err
					return
				}
			}
		}

		if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_monitor_traces_session_id ON monitor_traces(session_id)`); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("create index idx_monitor_traces_session_id: %w", err)
		}
		if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_monitor_traces_run_id ON monitor_traces(run_id)`); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("create index idx_monitor_traces_run_id: %w", err)
		}
		if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_monitor_traces_agent_id ON monitor_traces(agent_id)`); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("create index idx_monitor_traces_agent_id: %w", err)
		}
		if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_monitor_traces_provider ON monitor_traces(provider)`); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("create index idx_monitor_traces_provider: %w", err)
		}
		if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_monitor_traces_model ON monitor_traces(model)`); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("create index idx_monitor_traces_model: %w", err)
		}

		// Backfill agent_id / provider / model from metadata_json for rows where
		// the column is empty but the JSON blob carries the value. This is idempotent.
		// COALESCE(NULLIF(...), col) preserves the original value when json_extract
		// returns NULL (missing key) or empty string, preventing NOT NULL constraint violations.
		agentIDExpr := d.JSONExtract("metadata_json", "agent_id")
		providerExpr := d.JSONExtract("metadata_json", "provider")
		modelExpr := d.JSONExtract("metadata_json", "model")
		if _, err := db.ExecContext(ctx, fmt.Sprintf(`
UPDATE monitor_traces SET
  agent_id = COALESCE(NULLIF(%s, ''), agent_id),
  provider = COALESCE(NULLIF(%s, ''), provider),
  model    = COALESCE(NULLIF(%s, ''), model)
WHERE (agent_id = '' AND %s != '')
   OR (provider = '' AND %s != '')
   OR (model    = '' AND %s    != '')`,
			agentIDExpr, providerExpr, modelExpr,
			agentIDExpr, providerExpr, modelExpr)); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("backfill monitor_traces columns: %w", err)
		}

		// Create monitor_trace_spans table with dialect-aware auto-increment syntax.
		// SQLite: INTEGER PRIMARY KEY AUTOINCREMENT
		// Postgres: BIGSERIAL PRIMARY KEY (or BIGINT GENERATED ALWAYS AS IDENTITY)
		idColDef := "id INTEGER PRIMARY KEY AUTOINCREMENT"
		if d.IsPostgres() {
			idColDef = "id BIGSERIAL PRIMARY KEY"
		}
		// Dialect-aware timestamp column type:
		// SQLite INTEGER is 8 bytes (stores nanosecond timestamps fine).
		// Postgres INTEGER is 4 bytes (max ~2.1B) — overflows on nanosecond timestamps.
		// Use BIGINT on Postgres to match SQLite's capacity.
		tsType := "INTEGER"
		if d.IsPostgres() {
			tsType = "BIGINT"
		}
		_, firstErr = db.ExecContext(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS monitor_trace_spans (
    %s,
    trace_id TEXT NOT NULL,
    span_id TEXT NOT NULL,
    parent_span_id TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL,
    name TEXT NOT NULL,
    started_at %s NOT NULL,
    ended_at %s NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'running',
    attributes_json TEXT NOT NULL DEFAULT '',
    error_json TEXT NOT NULL DEFAULT '',
    UNIQUE(trace_id, span_id)
)`, idColDef, tsType, tsType))
		if firstErr != nil {
			return
		}
		if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_monitor_trace_spans_trace_id ON monitor_trace_spans(trace_id)`); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("create index idx_monitor_trace_spans_trace_id: %w", err)
		}
		if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_monitor_trace_spans_kind ON monitor_trace_spans(kind)`); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("create index idx_monitor_trace_spans_kind: %w", err)
		}
	})

	eventSchemaOnce.Do(func() {
		db := r.data.RWDB().WriteDB(ctx)
		// GENERATED VIRTUAL columns are SQLite-only. Postgres supports only
		// GENERATED STORED. Use dialect-aware DDL.
		// SQLite: GENERATED ALWAYS AS (json_extract(...)) VIRTUAL
		// Postgres: GENERATED ALWAYS AS (metadata_json ->> '...') STORED
		//   (Postgres ->> operator is IMMUTABLE since Postgres 12, so it can
		//   be used in GENERATED STORED columns.)
		var eventPatches []struct {
			table string
			col   string
			ddl   string
		}
		if d.IsPostgres() {
			eventPatches = []struct {
				table string
				col   string
				ddl   string
			}{
				{"monitor_events", "meta_session_id", "ALTER TABLE monitor_events ADD COLUMN meta_session_id TEXT GENERATED ALWAYS AS (metadata_json ->> 'session_id') STORED"},
				{"monitor_events", "meta_invocation_id", "ALTER TABLE monitor_events ADD COLUMN meta_invocation_id TEXT GENERATED ALWAYS AS (metadata_json ->> 'invocation_id') STORED"},
				{"monitor_events", "meta_agent_id", "ALTER TABLE monitor_events ADD COLUMN meta_agent_id TEXT GENERATED ALWAYS AS (metadata_json ->> 'agent_id') STORED"},
				{"monitor_events", "meta_trace_id", "ALTER TABLE monitor_events ADD COLUMN meta_trace_id TEXT GENERATED ALWAYS AS (metadata_json ->> 'trace_id') STORED"},
				{"monitor_events", "meta_duration_ms", "ALTER TABLE monitor_events ADD COLUMN meta_duration_ms DOUBLE PRECISION GENERATED ALWAYS AS (CAST(metadata_json ->> 'duration_ms' AS DOUBLE PRECISION)) STORED"},
			}
		} else {
			eventPatches = []struct {
				table string
				col   string
				ddl   string
			}{
				{"monitor_events", "meta_session_id", "ALTER TABLE monitor_events ADD COLUMN meta_session_id TEXT GENERATED ALWAYS AS (json_extract(metadata_json, '$.session_id')) VIRTUAL"},
				{"monitor_events", "meta_invocation_id", "ALTER TABLE monitor_events ADD COLUMN meta_invocation_id TEXT GENERATED ALWAYS AS (json_extract(metadata_json, '$.invocation_id')) VIRTUAL"},
				{"monitor_events", "meta_agent_id", "ALTER TABLE monitor_events ADD COLUMN meta_agent_id TEXT GENERATED ALWAYS AS (json_extract(metadata_json, '$.agent_id')) VIRTUAL"},
				{"monitor_events", "meta_trace_id", "ALTER TABLE monitor_events ADD COLUMN meta_trace_id TEXT GENERATED ALWAYS AS (json_extract(metadata_json, '$.trace_id')) VIRTUAL"},
				{"monitor_events", "meta_duration_ms", "ALTER TABLE monitor_events ADD COLUMN meta_duration_ms REAL GENERATED ALWAYS AS (CAST(json_extract(metadata_json, '$.duration_ms') AS REAL)) VIRTUAL"},
			}
		}
		for _, p := range eventPatches {
			has, err := columnExistsWithDialect(ctx, db, p.table, p.col, d)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			if has {
				continue
			}
			if _, err := db.ExecContext(ctx, p.ddl); err != nil {
				if !d.AlreadyExistsErr(err) {
					if firstErr == nil {
						firstErr = err
					}
					return
				}
			}
		}
		if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_monitor_events_meta_session_id ON monitor_events(meta_session_id)`); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("create index idx_monitor_events_meta_session_id: %w", err)
		}
		if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_monitor_events_meta_invocation_id ON monitor_events(meta_invocation_id, event_key)`); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("create index idx_monitor_events_meta_invocation_id: %w", err)
		}
		if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_monitor_events_meta_trace_id ON monitor_events(meta_trace_id)`); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("create index idx_monitor_events_meta_trace_id: %w", err)
		}
	})

	return firstErr
}

// columnExists checks whether a column exists in the given table.
// Uses dialect-aware system catalog queries.
func columnExists(ctx context.Context, db execer, table, column string) (bool, error) {
	return columnExistsWithDialect(ctx, db, table, column, DialectSQLite)
}

// columnExistsWithDialect is the dialect-aware variant of columnExists.
// SQLite: pragma_table_info(table) WHERE name = ?
// Postgres: information_schema.columns WHERE table_name = $1 AND column_name = $2
func columnExistsWithDialect(ctx context.Context, db execer, table, column string, d Dialect) (bool, error) {
	var query string
	var args []any
	if d.IsPostgres() {
		query = `SELECT column_name FROM information_schema.columns WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2 LIMIT 1`
		args = []any{table, column}
	} else {
		query = `SELECT 1 FROM pragma_table_info(?) WHERE name = ? LIMIT 1`
		args = []any{table, column}
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	return rows.Next(), nil
}

func (r *monitorRepo) InsertMonitorTrace(ctx context.Context, tw biz.MonitorTraceWrite) error {
	id := strings.TrimSpace(tw.TraceID)
	if id == "" {
		id = uuid.NewString()
	}
	status := strings.TrimSpace(tw.Status)
	if status == "" {
		status = "running"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	d := r.data.Dialect()
	// SQLite: INSERT OR IGNORE; Postgres: INSERT ... ON CONFLICT DO NOTHING.
	// The conflict target is the primary key (id).
	var sql string
	if d.IsPostgres() {
		sql = `INSERT INTO monitor_traces
		 (id, trace_key, name, description, status, metadata_json, created_at, updated_at, deleted_at,
		  session_id, run_id, invocation_id, agent_id, provider, model, team_id, parent_trace_id,
		  duration_ms, span_count, error_count, total_tokens, total_cost_usd)
		 VALUES ($1, $2, $3, '', $4, $5, $6, $7, '', $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
		 ON CONFLICT (id) DO NOTHING`
	} else {
		sql = `INSERT OR IGNORE INTO monitor_traces
		 (id, trace_key, name, description, status, metadata_json, created_at, updated_at, deleted_at,
		  session_id, run_id, invocation_id, agent_id, provider, model, team_id, parent_trace_id,
		  duration_ms, span_count, error_count, total_tokens, total_cost_usd)
		 VALUES (?, ?, ?, '', ?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, sql,
		id, tw.TraceID, tw.Name, status, tw.MetadataJSON, now, now,
		tw.SessionID, tw.RunID, tw.InvocationID, tw.AgentID, tw.Provider, tw.Model, tw.TeamID, tw.ParentTraceID,
		tw.DurationMs, tw.SpanCount, tw.ErrorCount, tw.TotalTokens, tw.TotalCostUsd,
	)
	return entErrToBizErr(err, "monitor")
}

func (r *monitorRepo) UpsertMonitorTraceSpan(ctx context.Context, sw biz.MonitorTraceSpanWrite) error {
	d := r.data.Dialect()
	var sql string
	if d.IsPostgres() {
		sql = `INSERT INTO monitor_trace_spans (trace_id, span_id, parent_span_id, kind, name, started_at, ended_at, status, attributes_json, error_json)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT (trace_id, span_id) DO UPDATE SET
		   ended_at = EXCLUDED.ended_at,
		   status = EXCLUDED.status,
		   attributes_json = EXCLUDED.attributes_json,
		   error_json = EXCLUDED.error_json`
	} else {
		sql = `INSERT INTO monitor_trace_spans (trace_id, span_id, parent_span_id, kind, name, started_at, ended_at, status, attributes_json, error_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(trace_id, span_id) DO UPDATE SET
		   ended_at = excluded.ended_at,
		   status = excluded.status,
		   attributes_json = excluded.attributes_json,
		   error_json = excluded.error_json`
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, sql,
		sw.TraceID, sw.SpanID, sw.ParentSpanID, sw.Kind, sw.Name,
		sw.StartedAt, sw.EndedAt, sw.Status, sw.AttributesJSON, sw.ErrorJSON,
	)
	return entErrToBizErr(err, "monitor")
}

func (r *monitorRepo) UpdateMonitorTraceCompletion(ctx context.Context, traceID string, status string, durationMs int64, spanCount, errorCount int, totalTokens int64, totalCostUsd float64) error {
	traceID = strings.TrimSpace(traceID)
	if traceID == "" {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	d := r.data.Dialect()
	var sql string
	if d.IsPostgres() {
		sql = `UPDATE monitor_traces SET status = $1, duration_ms = $2, span_count = $3, error_count = $4,
		 total_tokens = $5, total_cost_usd = $6, updated_at = $7
		 WHERE id = $8 AND deleted_at = ''`
	} else {
		sql = `UPDATE monitor_traces SET status = ?, duration_ms = ?, span_count = ?, error_count = ?,
		 total_tokens = ?, total_cost_usd = ?, updated_at = ?
		 WHERE id = ? AND deleted_at = ''`
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, sql,
		status, durationMs, spanCount, errorCount, totalTokens, totalCostUsd, now, traceID,
	)
	return entErrToBizErr(err, "monitor")
}

func (r *monitorRepo) ListRecentRunnerCompletions(ctx context.Context, since time.Duration, limit int) ([]biz.RunnerCompletionRow, error) {
	if limit <= 0 {
		limit = 1000
	}
	sinceStr := time.Now().UTC().Add(-since).Format(time.RFC3339)
	d := r.data.Dialect()
	// Build dialect-aware COALESCE expressions for JSON extraction.
	// SQLite: COALESCE(meta_trace_id, json_extract(metadata_json, '$.trace_id'), '')
	// Postgres: COALESCE(meta_trace_id, metadata_json ->> 'trace_id', '')
	traceIDExpr := d.JSONExtract("metadata_json", "trace_id")
	sessionIDExpr := d.JSONExtract("metadata_json", "session_id")
	runIDExpr := d.JSONExtract("metadata_json", "run_id")
	agentIDExpr := d.JSONExtract("metadata_json", "agent_id")
	var sql string
	var args []any
	if d.IsPostgres() {
		sql = fmt.Sprintf(`SELECT COALESCE(meta_trace_id, %s, '') AS trace_id,
		        COALESCE(meta_session_id, %s, event_key) AS session_id,
		        COALESCE(%s, '') AS run_id,
		        COALESCE(meta_agent_id, %s, '') AS agent_id,
		        CASE WHEN status = 'error' THEN 'error' ELSE 'ok' END AS status,
		        created_at
		 FROM monitor_events
		 WHERE event_key = 'runner.completion' AND created_at >= $1 AND deleted_at = ''
		 ORDER BY created_at DESC
		 LIMIT $2`, traceIDExpr, sessionIDExpr, runIDExpr, agentIDExpr)
		args = []any{sinceStr, limit}
	} else {
		sql = fmt.Sprintf(`SELECT COALESCE(meta_trace_id, %s, '') AS trace_id,
		        COALESCE(meta_session_id, %s, event_key) AS session_id,
		        COALESCE(%s, '') AS run_id,
		        COALESCE(meta_agent_id, %s, '') AS agent_id,
		        CASE WHEN status = 'error' THEN 'error' ELSE 'ok' END AS status,
		        created_at
		 FROM monitor_events
		 WHERE event_key = 'runner.completion' AND created_at >= ? AND deleted_at = ''
		 ORDER BY created_at DESC
		 LIMIT ?`, traceIDExpr, sessionIDExpr, runIDExpr, agentIDExpr)
		args = []any{sinceStr, limit}
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, entErrToBizErr(err, "monitor")
	}
	defer rows.Close()
	var out []biz.RunnerCompletionRow
	for rows.Next() {
		var row biz.RunnerCompletionRow
		if err := rows.Scan(&row.TraceID, &row.SessionID, &row.RunID, &row.AgentID, &row.Status, &row.CreatedAt); err != nil {
			return nil, entErrToBizErr(err, "monitor")
		}
		out = append(out, row)
	}
	return out, entErrToBizErr(rows.Err(), "monitor")
}
