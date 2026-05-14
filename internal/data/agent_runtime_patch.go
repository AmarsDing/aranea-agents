package data

import (
	"context"
	"strings"

	"aranea-agents/internal/data/ent"
)

// ensureAgentRuntimePatches applies SQLite-safe ALTERs for columns introduced after first DB creation.
// Ent Schema.Create does not migrate existing SQLite rows for new fields.
func ensureAgentRuntimePatches(ctx context.Context, c *ent.Client) error {
	if c == nil {
		return nil
	}
	patches := []struct {
		col string
		ddl string
	}{
		{"skill_runtime_json", `ALTER TABLE agent_runtime_settings ADD COLUMN skill_runtime_json TEXT NOT NULL DEFAULT '{}'`},
		{"l0_compress_provider", `ALTER TABLE agent_runtime_settings ADD COLUMN l0_compress_provider TEXT NOT NULL DEFAULT ''`},
		{"l0_compress_model", `ALTER TABLE agent_runtime_settings ADD COLUMN l0_compress_model TEXT NOT NULL DEFAULT ''`},
		{"intent_pass_enabled", `ALTER TABLE agent_runtime_settings ADD COLUMN intent_pass_enabled INTEGER NOT NULL DEFAULT 1`},
		{"tools_retry_enabled", `ALTER TABLE agent_runtime_settings ADD COLUMN tools_retry_enabled INTEGER NOT NULL DEFAULT 0`},
		{"tools_retry_max_attempts", `ALTER TABLE agent_runtime_settings ADD COLUMN tools_retry_max_attempts INTEGER NOT NULL DEFAULT 2`},
		{"tools_retry_initial_interval_ms", `ALTER TABLE agent_runtime_settings ADD COLUMN tools_retry_initial_interval_ms INTEGER NOT NULL DEFAULT 500`},
		{"tools_retry_backoff_factor", `ALTER TABLE agent_runtime_settings ADD COLUMN tools_retry_backoff_factor REAL NOT NULL DEFAULT 2.0`},
		{"tools_retry_max_interval_ms", `ALTER TABLE agent_runtime_settings ADD COLUMN tools_retry_max_interval_ms INTEGER NOT NULL DEFAULT 5000`},
		{"tools_retry_jitter", `ALTER TABLE agent_runtime_settings ADD COLUMN tools_retry_jitter INTEGER NOT NULL DEFAULT 1`},
		{"tools_parallel_enabled", `ALTER TABLE agent_runtime_settings ADD COLUMN tools_parallel_enabled INTEGER NOT NULL DEFAULT 0`},
		{"tools_streaming_enabled", `ALTER TABLE agent_runtime_settings ADD COLUMN tools_streaming_enabled INTEGER NOT NULL DEFAULT 0`},
	}
	for _, p := range patches {
		has, err := sqliteColumnExists(ctx, c, "agent_runtime_settings", p.col)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		if _, err := c.ExecContext(ctx, p.ddl); err != nil {
			return err
		}
	}
	return nil
}

func sqliteColumnExists(ctx context.Context, c *ent.Client, table, column string) (bool, error) {
	table = strings.TrimSpace(table)
	column = strings.TrimSpace(column)
	if table != "agent_runtime_settings" || column == "" {
		return false, nil
	}
	rows, err := c.QueryContext(ctx, `SELECT 1 FROM pragma_table_info('agent_runtime_settings') WHERE name = ? LIMIT 1`, column)
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
