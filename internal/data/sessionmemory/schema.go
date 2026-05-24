package sessionmemory

import (
	"context"
	"fmt"

	"aranea-agents/internal/data/ent"
)

func EnsurePatches(ctx context.Context, client *ent.Client) error {
	if client == nil {
		return nil
	}
	patches := []struct {
		table string
		col   string
		ddl   string
	}{
		{
			"tool_invocation_params", "param_name",
			"ALTER TABLE tool_invocation_params ADD COLUMN param_name TEXT NOT NULL DEFAULT ''",
		},
		{
			"tool_invocation_params", "param_type",
			"ALTER TABLE tool_invocation_params ADD COLUMN param_type TEXT NOT NULL DEFAULT 'string'",
		},
		{
			"tool_invocation_params", "value_preview",
			"ALTER TABLE tool_invocation_params ADD COLUMN value_preview TEXT NOT NULL DEFAULT ''",
		},
		{
			"tool_invocation_params", "value_hash",
			"ALTER TABLE tool_invocation_params ADD COLUMN value_hash TEXT NOT NULL DEFAULT ''",
		},
		{
			"tool_invocation_params", "value_size_bytes",
			"ALTER TABLE tool_invocation_params ADD COLUMN value_size_bytes INTEGER NOT NULL DEFAULT 0",
		},
		{
			"tool_invocation_params", "is_required",
			"ALTER TABLE tool_invocation_params ADD COLUMN is_required INTEGER NOT NULL DEFAULT 0",
		},
		{
			"tool_invocation_params", "is_sensitive",
			"ALTER TABLE tool_invocation_params ADD COLUMN is_sensitive INTEGER NOT NULL DEFAULT 0",
		},
		{
			"tool_invocation_params", "redaction_reason",
			"ALTER TABLE tool_invocation_params ADD COLUMN redaction_reason TEXT NOT NULL DEFAULT ''",
		},
	}
	for _, p := range patches {
		has, err := sqliteColumnExists(ctx, client, p.table, p.col)
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

// EnsureMonitorSchemaPatches adds audit_logs columns missing on DBs created before monitor extended fields.
// Call after EnsureSchema so audit_logs exists.
func EnsureMonitorSchemaPatches(ctx context.Context, client *ent.Client) error {
	if client == nil {
		return nil
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
		has, err := sqliteColumnExists(ctx, client, p.table, p.col)
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
	return nil
}

// EnsureMemoryRelationPatches adds bi-temporal columns to memory_relations on existing DBs.
func EnsureMemoryRelationPatches(ctx context.Context, client *ent.Client) error {
	if client == nil {
		return nil
	}
	patches := []struct {
		table string
		col   string
		ddl   string
	}{
		{"memory_relations", "valid_from", "ALTER TABLE memory_relations ADD COLUMN valid_from TEXT NOT NULL DEFAULT ''"},
		{"memory_relations", "valid_to", "ALTER TABLE memory_relations ADD COLUMN valid_to TEXT NOT NULL DEFAULT ''"},
	}
	for _, p := range patches {
		has, err := sqliteColumnExists(ctx, client, p.table, p.col)
		if err != nil {
			return fmt.Errorf("memory relation patch check %s.%s: %w", p.table, p.col, err)
		}
		if has {
			continue
		}
		if _, err := client.ExecContext(ctx, p.ddl); err != nil {
			return fmt.Errorf("memory relation patch %s.%s: %w", p.table, p.col, err)
		}
	}
	return nil
}

func sqliteColumnExists(ctx context.Context, client *ent.Client, table, column string) (bool, error) {
	rows, err := client.QueryContext(ctx, "SELECT 1 FROM pragma_table_info(?) WHERE name = ? LIMIT 1", table, column)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return false, nil
	}
	var one int
	if err := rows.Scan(&one); err != nil {
		return false, err
	}
	return true, nil
}
