package data

import (
	"context"
	"fmt"
)

func sessionMemoryEnsurePatches(ctx context.Context, client execer, d Dialect) error {
	if client == nil {
		return nil
	}
	patches := []struct {
		table string
		col   string
		ddl   string
	}{
		{"tool_invocation_params", "param_name", "ALTER TABLE tool_invocation_params ADD COLUMN param_name TEXT NOT NULL DEFAULT ''"},
		{"tool_invocation_params", "param_type", "ALTER TABLE tool_invocation_params ADD COLUMN param_type TEXT NOT NULL DEFAULT 'string'"},
		{"tool_invocation_params", "value_preview", "ALTER TABLE tool_invocation_params ADD COLUMN value_preview TEXT NOT NULL DEFAULT ''"},
		{"tool_invocation_params", "value_hash", "ALTER TABLE tool_invocation_params ADD COLUMN value_hash TEXT NOT NULL DEFAULT ''"},
		{"tool_invocation_params", "value_size_bytes", "ALTER TABLE tool_invocation_params ADD COLUMN value_size_bytes INTEGER NOT NULL DEFAULT 0"},
		{"tool_invocation_params", "is_required", "ALTER TABLE tool_invocation_params ADD COLUMN is_required INTEGER NOT NULL DEFAULT 0"},
		{"tool_invocation_params", "is_sensitive", "ALTER TABLE tool_invocation_params ADD COLUMN is_sensitive INTEGER NOT NULL DEFAULT 0"},
		{"tool_invocation_params", "redaction_reason", "ALTER TABLE tool_invocation_params ADD COLUMN redaction_reason TEXT NOT NULL DEFAULT ''"},
	}
	for _, p := range patches {
		has, err := memColumnExists(ctx, client, d, p.table, p.col)
		if err != nil {
			return fmt.Errorf("sessionmemory patch check %s.%s: %w", p.table, p.col, err)
		}
		if has {
			continue
		}
		if _, err := client.ExecContext(ctx, p.ddl); err != nil {
			return fmt.Errorf("sessionmemory patch %s.%s: %w", p.table, p.col, err)
		}
	}
	return nil
}

func sessionMemoryEnsureMonitorSchemaPatches(ctx context.Context, client execer, d Dialect) error {
	if client == nil {
		return nil
	}

	if _, err := client.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS audit_logs (
		id TEXT PRIMARY KEY,
		action TEXT NOT NULL DEFAULT '',
		resource TEXT NOT NULL DEFAULT '',
		resource_id TEXT NOT NULL DEFAULT '',
		request_id TEXT NOT NULL DEFAULT '',
		detail TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		return fmt.Errorf("monitor patch create audit_logs: %w", err)
	}

	if _, err := client.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS monitor_events (
		id TEXT PRIMARY KEY,
		event_key TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'ok',
		metadata_json TEXT NOT NULL DEFAULT '{}',
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '',
		deleted_at TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		return fmt.Errorf("monitor patch create monitor_events: %w", err)
	}

	if _, err := client.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS monitor_traces (
		id TEXT PRIMARY KEY,
		trace_key TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'ok',
		metadata_json TEXT NOT NULL DEFAULT '{}',
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '',
		deleted_at TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		return fmt.Errorf("monitor patch create monitor_traces: %w", err)
	}
	patches := []struct {
		table string
		col   string
		ddl   string
	}{
		{"audit_logs", "actor", "ALTER TABLE audit_logs ADD COLUMN actor TEXT NOT NULL DEFAULT ''"},
		{"audit_logs", "ip", "ALTER TABLE audit_logs ADD COLUMN ip TEXT NOT NULL DEFAULT ''"},
		{"audit_logs", "user_agent", "ALTER TABLE audit_logs ADD COLUMN user_agent TEXT NOT NULL DEFAULT ''"},
		{"audit_logs", "severity", "ALTER TABLE audit_logs ADD COLUMN severity TEXT NOT NULL DEFAULT ''"},
		{"audit_logs", "metadata_json", "ALTER TABLE audit_logs ADD COLUMN metadata_json TEXT NOT NULL DEFAULT ''"},
	}
	for _, p := range patches {
		has, err := memColumnExists(ctx, client, d, p.table, p.col)
		if err != nil {
			return fmt.Errorf("monitor patch check %s.%s: %w", p.table, p.col, err)
		}
		if has {
			continue
		}
		if _, err := client.ExecContext(ctx, p.ddl); err != nil {
			return fmt.Errorf("monitor patch %s.%s: %w", p.table, p.col, err)
		}
	}

	for _, tbl := range []string{"monitor_events", "monitor_traces"} {
		exists, err := memTableExists(ctx, client, d, tbl)
		if err != nil {
			return fmt.Errorf("monitor patch verify %s: %w", tbl, err)
		}
		if !exists {
			return fmt.Errorf("monitor patch verify: table %s not found after CREATE TABLE IF NOT EXISTS", tbl)
		}
	}
	return nil
}

// memColumnExists reports whether table.column exists, via Postgres
// information_schema.columns.
func memColumnExists(ctx context.Context, client execer, d Dialect, table, column string) (bool, error) {
	query := `SELECT column_name FROM information_schema.columns WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2 LIMIT 1`
	args := []any{table, column}
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	return rows.Next(), nil
}

// memTableExists reports whether table exists, via Postgres
// information_schema.tables.
func memTableExists(ctx context.Context, client execer, d Dialect, table string) (bool, error) {
	query := `SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1 LIMIT 1`
	args := []any{table}
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	return rows.Next(), nil
}
