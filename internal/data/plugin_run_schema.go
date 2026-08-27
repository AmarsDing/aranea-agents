package data

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"aranea-agents/internal/data/ent"
)

//go:embed sql/plugin_run.sql
var pluginRunDDL string

//go:embed sql/monitor_alert.sql
var monitorAlertDDL string

//go:embed sql/ecosystem_product.sql
var ecosystemProductDDL string

// EnsurePluginRunSchema creates plugin_runs table if missing.
func EnsurePluginRunSchema(ctx context.Context, client *ent.Client) error {
	return execDDLFile(ctx, client, pluginRunDDL, "plugin_run")
}

// EnsureMonitorAlertSchema creates monitor_alert_rules table if missing.
func EnsureMonitorAlertSchema(ctx context.Context, client *ent.Client, d Dialect) error {
	if err := execDDLFile(ctx, client, monitorAlertDDL, "monitor_alert"); err != nil {
		return err
	}
	return ensureMonitorAlertColumns(ctx, client, d)
}

// EnsureEcosystemSchema creates ecosystem marketplace tables if missing.
func EnsureEcosystemSchema(ctx context.Context, client *ent.Client) error {
	return execDDLFile(ctx, client, ecosystemProductDDL, "ecosystem")
}

func ensureMonitorAlertColumns(ctx context.Context, client *ent.Client, d Dialect) error {
	if client == nil {
		return nil
	}
	alters := []string{
		`ALTER TABLE monitor_alert_rules ADD COLUMN notify_webhook_url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE monitor_alert_rules ADD COLUMN notify_channel_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE monitor_alert_rules ADD COLUMN cooldown_minutes INTEGER NOT NULL DEFAULT 60`,
	}
	for _, stmt := range alters {
		if _, err := client.ExecContext(ctx, stmt); err != nil {
			if d.AlreadyExistsErr(err) {
				continue
			}
			return fmt.Errorf("monitor alert column patch: %w", err)
		}
	}
	return nil
}

func execDDLFile(ctx context.Context, client *ent.Client, ddl, label string) error {
	if client == nil {
		return nil
	}
	ddl = strings.TrimPrefix(ddl, "\ufeff")
	for _, stmt := range splitDDLStatements(strings.TrimSpace(ddl)) {
		if stmt == "" {
			continue
		}
		if _, err := client.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%s ddl: %w\n---\n%s", label, err, stmt)
		}
	}
	return nil
}

// splitDDLStatements splits a DDL script into statements on semicolons.
// Single-quoted string literals are honored: semicolons and `--` inside
// strings never fragment statements (20261263 tool_param_rules seed 的正则
// 字符类 `[;&|...]` 含字面量分号）；`''` 视为转义引号。
// `--` line comments outside strings are stripped first so semicolons inside
// comments never fragment statements (fresh-Postgres failure of 20261128_memory_facts_fts_index).
// Contract: scripts fed here must not use `$$` dollar-quoted bodies;
// complex DDL goes through Func-based migrations instead.
func splitDDLStatements(ddl string) []string {
	var (
		stmts []string
		cur   strings.Builder
		inStr bool
	)
	flush := func() {
		if stmt := strings.TrimSpace(cur.String()); stmt != "" {
			stmts = append(stmts, stmt)
		}
		cur.Reset()
	}
	for i := 0; i < len(ddl); i++ {
		c := ddl[i]
		switch {
		case inStr:
			cur.WriteByte(c)
			if c == '\'' {
				if i+1 < len(ddl) && ddl[i+1] == '\'' {
					cur.WriteByte('\'')
					i++
				} else {
					inStr = false
				}
			}
		case c == '\'':
			inStr = true
			cur.WriteByte(c)
		case c == '-' && i+1 < len(ddl) && ddl[i+1] == '-':
			for i < len(ddl) && ddl[i] != '\n' {
				i++
			}
			cur.WriteByte('\n')
		case c == ';':
			flush()
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return stmts
}
