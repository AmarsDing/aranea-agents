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
		{"l0_compress_min_gap_sec", `ALTER TABLE agent_runtime_settings ADD COLUMN l0_compress_min_gap_sec INTEGER NOT NULL DEFAULT 600`},
		{"l0_compress_provider", `ALTER TABLE agent_runtime_settings ADD COLUMN l0_compress_provider TEXT NOT NULL DEFAULT ''`},
		{"l0_compress_model", `ALTER TABLE agent_runtime_settings ADD COLUMN l0_compress_model TEXT NOT NULL DEFAULT ''`},
		{"memory_worker_provider", `ALTER TABLE agent_runtime_settings ADD COLUMN memory_worker_provider TEXT NOT NULL DEFAULT ''`},
		{"memory_worker_model", `ALTER TABLE agent_runtime_settings ADD COLUMN memory_worker_model TEXT NOT NULL DEFAULT ''`},
		{"intent_pass_enabled", `ALTER TABLE agent_runtime_settings ADD COLUMN intent_pass_enabled INTEGER NOT NULL DEFAULT 1`},
		{"tools_retry_enabled", `ALTER TABLE agent_runtime_settings ADD COLUMN tools_retry_enabled INTEGER NOT NULL DEFAULT 0`},
		{"tools_retry_max_attempts", `ALTER TABLE agent_runtime_settings ADD COLUMN tools_retry_max_attempts INTEGER NOT NULL DEFAULT 2`},
		{"tools_retry_initial_interval_ms", `ALTER TABLE agent_runtime_settings ADD COLUMN tools_retry_initial_interval_ms INTEGER NOT NULL DEFAULT 500`},
		{"tools_retry_backoff_factor", `ALTER TABLE agent_runtime_settings ADD COLUMN tools_retry_backoff_factor REAL NOT NULL DEFAULT 2.0`},
		{"tools_retry_max_interval_ms", `ALTER TABLE agent_runtime_settings ADD COLUMN tools_retry_max_interval_ms INTEGER NOT NULL DEFAULT 5000`},
		{"tools_retry_jitter", `ALTER TABLE agent_runtime_settings ADD COLUMN tools_retry_jitter INTEGER NOT NULL DEFAULT 1`},
		{"tools_parallel_enabled", `ALTER TABLE agent_runtime_settings ADD COLUMN tools_parallel_enabled INTEGER NOT NULL DEFAULT 0`},
		{"tools_streaming_enabled", `ALTER TABLE agent_runtime_settings ADD COLUMN tools_streaming_enabled INTEGER NOT NULL DEFAULT 0`},
		{"channel_id", `ALTER TABLE agent_runtime_settings ADD COLUMN channel_id TEXT NOT NULL DEFAULT ''`},
		{"chat_id", `ALTER TABLE agent_runtime_settings ADD COLUMN chat_id TEXT NOT NULL DEFAULT ''`},
		{"workspace", `ALTER TABLE agent_runtime_settings ADD COLUMN workspace TEXT NOT NULL DEFAULT ''`},
		{"reasoning_mode", `ALTER TABLE agent_runtime_settings ADD COLUMN reasoning_mode TEXT NOT NULL DEFAULT 'provider_default'`},
		{"reasoning_level", `ALTER TABLE agent_runtime_settings ADD COLUMN reasoning_level TEXT NOT NULL DEFAULT 'off'`},
		{"code_executor_type", `ALTER TABLE agent_runtime_settings ADD COLUMN code_executor_type TEXT NOT NULL DEFAULT 'local'`},
		{"planner_kind", `ALTER TABLE agent_runtime_settings ADD COLUMN planner_kind TEXT NOT NULL DEFAULT ''`},
		{"planner_config_json", `ALTER TABLE agent_runtime_settings ADD COLUMN planner_config_json TEXT NOT NULL DEFAULT '{}'`},
		{"ralph_loop_max_iterations", `ALTER TABLE agent_runtime_settings ADD COLUMN ralph_loop_max_iterations INTEGER NOT NULL DEFAULT 0`},
		{"ralph_loop_completion_promise", `ALTER TABLE agent_runtime_settings ADD COLUMN ralph_loop_completion_promise TEXT NOT NULL DEFAULT ''`},
		{"ralph_loop_verify_command", `ALTER TABLE agent_runtime_settings ADD COLUMN ralph_loop_verify_command TEXT NOT NULL DEFAULT ''`},
		{"ralph_loop_verify_timeout_seconds", `ALTER TABLE agent_runtime_settings ADD COLUMN ralph_loop_verify_timeout_seconds INTEGER NOT NULL DEFAULT 0`},
		{"ralph_loop_promise_tag_open", `ALTER TABLE agent_runtime_settings ADD COLUMN ralph_loop_promise_tag_open TEXT NOT NULL DEFAULT ''`},
		{"ralph_loop_promise_tag_close", `ALTER TABLE agent_runtime_settings ADD COLUMN ralph_loop_promise_tag_close TEXT NOT NULL DEFAULT ''`},
		{"ralph_loop_verify_work_dir", `ALTER TABLE agent_runtime_settings ADD COLUMN ralph_loop_verify_work_dir TEXT NOT NULL DEFAULT ''`},
		{"l4_decay_interval_hours", `ALTER TABLE agent_runtime_settings ADD COLUMN l4_decay_interval_hours INTEGER NOT NULL DEFAULT 0`},
		{"l4_decay_overrides_json", `ALTER TABLE agent_runtime_settings ADD COLUMN l4_decay_overrides_json TEXT NOT NULL DEFAULT ''`},
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

func sqliteTableExists(ctx context.Context, c *ent.Client, table string) (bool, error) {
	table = strings.TrimSpace(table)
	if table == "" {
		return false, nil
	}
	rows, err := c.QueryContext(ctx,
		`SELECT COUNT(1) FROM sqlite_master WHERE type IN ('table','view') AND name = ?`, table)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return false, rows.Err()
	}
	var n int
	if err := rows.Scan(&n); err != nil {
		return false, err
	}
	return n > 0, rows.Err()
}

func sqliteColumnExists(ctx context.Context, c *ent.Client, table, column string) (bool, error) {
	table = strings.TrimSpace(table)
	column = strings.TrimSpace(column)
	if table == "" || column == "" {
		return false, nil
	}
	rows, err := c.QueryContext(ctx, `SELECT 1 FROM pragma_table_info(?) WHERE name = ? LIMIT 1`, table, column)
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
