package sessionmemory

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"aranea-agents/internal/data/ent"
)

//go:embed memory_chain.sql
var memoryChainDDL string

// EnsureSchema creates session-memory chain tables (L0–L4, evolution) if missing.
// Safe on existing DBs (**CREATE IF NOT EXISTS**). FTS virtual tables are omitted (list APIs use LIKE).
func EnsureSchema(ctx context.Context, client *ent.Client) error {
	if client == nil {
		return nil
	}
	ddl := strings.TrimPrefix(memoryChainDDL, "\ufeff")
	ddl = stripLineComments(ddl)
	for _, stmt := range splitStatements(ddl) {
		if stmt == "" {
			continue
		}
		if _, err := client.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("sessionmemory ddl: %w\n---\n%s\n---", err, snippet(stmt, 500))
		}
	}
	return nil
}

func stripLineComments(ddl string) string {
	var b strings.Builder
	for _, line := range strings.Split(ddl, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "--") {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func splitStatements(ddl string) []string {
	parts := strings.Split(ddl, ";")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func snippet(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func EnsurePatches(ctx context.Context, client *ent.Client) error {
	if client == nil {
		return nil
	}
	patches := []struct {
		table  string
		col    string
		ddl    string
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
