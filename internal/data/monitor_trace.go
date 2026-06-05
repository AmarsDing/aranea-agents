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
			{"monitor_traces", "team_id", "ALTER TABLE monitor_traces ADD COLUMN team_id TEXT NOT NULL DEFAULT ''"},
			{"monitor_traces", "parent_trace_id", "ALTER TABLE monitor_traces ADD COLUMN parent_trace_id TEXT NOT NULL DEFAULT ''"},
			{"monitor_traces", "duration_ms", "ALTER TABLE monitor_traces ADD COLUMN duration_ms INTEGER NOT NULL DEFAULT 0"},
			{"monitor_traces", "span_count", "ALTER TABLE monitor_traces ADD COLUMN span_count INTEGER NOT NULL DEFAULT 0"},
			{"monitor_traces", "error_count", "ALTER TABLE monitor_traces ADD COLUMN error_count INTEGER NOT NULL DEFAULT 0"},
			{"monitor_traces", "total_tokens", "ALTER TABLE monitor_traces ADD COLUMN total_tokens INTEGER NOT NULL DEFAULT 0"},
			{"monitor_traces", "total_cost_usd", "ALTER TABLE monitor_traces ADD COLUMN total_cost_usd REAL NOT NULL DEFAULT 0.0"},
		}
		for _, p := range patches {
			has, err := columnExists(ctx, db, p.table, p.col)
			if err != nil {
				firstErr = err
				return
			}
			if has {
				continue
			}
			if _, err := db.ExecContext(ctx, p.ddl); err != nil {
				if !strings.Contains(err.Error(), "duplicate column") {
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

		_, firstErr = db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS monitor_trace_spans (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    trace_id TEXT NOT NULL,
    span_id TEXT NOT NULL,
    parent_span_id TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL,
    name TEXT NOT NULL,
    started_at INTEGER NOT NULL,
    ended_at INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'running',
    attributes_json TEXT NOT NULL DEFAULT '',
    error_json TEXT NOT NULL DEFAULT '',
    UNIQUE(trace_id, span_id)
)`)
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
		eventPatches := []struct {
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
		for _, p := range eventPatches {
			has, err := columnExists(ctx, db, p.table, p.col)
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
				if !strings.Contains(err.Error(), "duplicate column") {
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

func columnExists(ctx context.Context, db execer, table, column string) (bool, error) {
	rows, err := db.QueryContext(ctx, "SELECT 1 FROM pragma_table_info(?) WHERE name = ? LIMIT 1", table, column)
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
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`INSERT OR IGNORE INTO monitor_traces
		 (id, trace_key, name, description, status, metadata_json, created_at, updated_at, deleted_at,
		  session_id, run_id, invocation_id, agent_id, team_id, parent_trace_id,
		  duration_ms, span_count, error_count, total_tokens, total_cost_usd)
		 VALUES (?, ?, ?, '', ?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, tw.TraceID, tw.Name, status, tw.MetadataJSON, now, now,
		tw.SessionID, tw.RunID, tw.InvocationID, tw.AgentID, tw.TeamID, tw.ParentTraceID,
		tw.DurationMs, tw.SpanCount, tw.ErrorCount, tw.TotalTokens, tw.TotalCostUsd,
	)
	return err
}

func (r *monitorRepo) UpsertMonitorTraceSpan(ctx context.Context, sw biz.MonitorTraceSpanWrite) error {
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`INSERT INTO monitor_trace_spans (trace_id, span_id, parent_span_id, kind, name, started_at, ended_at, status, attributes_json, error_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(trace_id, span_id) DO UPDATE SET
		   ended_at = excluded.ended_at,
		   status = excluded.status,
		   attributes_json = excluded.attributes_json,
		   error_json = excluded.error_json`,
		sw.TraceID, sw.SpanID, sw.ParentSpanID, sw.Kind, sw.Name,
		sw.StartedAt, sw.EndedAt, sw.Status, sw.AttributesJSON, sw.ErrorJSON,
	)
	return err
}

func (r *monitorRepo) UpdateMonitorTraceCompletion(ctx context.Context, traceID string, status string, durationMs int64, spanCount, errorCount int, totalTokens int64, totalCostUsd float64) error {
	traceID = strings.TrimSpace(traceID)
	if traceID == "" {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE monitor_traces SET status = ?, duration_ms = ?, span_count = ?, error_count = ?,
		 total_tokens = ?, total_cost_usd = ?, updated_at = ?
		 WHERE id = ? AND deleted_at = ''`,
		status, durationMs, spanCount, errorCount, totalTokens, totalCostUsd, now, traceID,
	)
	return err
}

func (r *monitorRepo) ListRecentRunnerCompletions(ctx context.Context, since time.Duration, limit int) ([]biz.RunnerCompletionRow, error) {
	if limit <= 0 {
		limit = 1000
	}
	sinceStr := time.Now().UTC().Add(-since).Format(time.RFC3339)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT COALESCE(meta_trace_id, json_extract(metadata_json, '$.trace_id'), '') AS trace_id,
		        COALESCE(meta_session_id, json_extract(metadata_json, '$.session_id'), event_key) AS session_id,
		        COALESCE(json_extract(metadata_json, '$.run_id'), '') AS run_id,
		        COALESCE(meta_agent_id, json_extract(metadata_json, '$.agent_id'), '') AS agent_id,
		        CASE WHEN status = 'error' THEN 'error' ELSE 'ok' END AS status,
		        created_at
		 FROM monitor_events
		 WHERE event_key = 'runner.completion' AND created_at >= ? AND deleted_at = ''
		 ORDER BY created_at DESC
		 LIMIT ?`, sinceStr, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []biz.RunnerCompletionRow
	for rows.Next() {
		var row biz.RunnerCompletionRow
		if err := rows.Scan(&row.TraceID, &row.SessionID, &row.RunID, &row.AgentID, &row.Status, &row.CreatedAt); err != nil {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}
