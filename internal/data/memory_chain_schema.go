package data

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

//go:embed sql/memory_chain.sql
var memoryChainDDL string

// EnsureSessionMemorySchema creates session-memory chain tables (L0–L4, evolution) if missing.
// Safe on existing DBs (**CREATE IF NOT EXISTS**). FTS virtual tables are omitted (list APIs use LIKE).
// When d is Postgres, SQLite-specific types (BLOB) are translated to Postgres equivalents (BYTEA).
func EnsureSessionMemorySchema(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
	if client == nil {
		return nil
	}
	ddl := strings.TrimPrefix(memoryChainDDL, "\ufeff")
	ddl = stripMemoryChainLineComments(ddl)
	if d.IsPostgres() {
		// Translate SQLite BLOB to Postgres BYTEA.
		ddl = strings.ReplaceAll(ddl, "BLOB", "BYTEA")
	}
	for _, stmt := range splitMemoryChainStatements(ddl) {
		if stmt == "" {
			continue
		}
		if _, err := client.ExecContext(ctx, stmt); err != nil {
			lg.Warn("session memory ddl failed", loggateway.StepID("memory.schema_init_fail"), loggateway.Err(err))
			return fmt.Errorf("sessionmemory ddl: %w\n---\n%s\n---", err, memoryChainSnippet(stmt, 500))
		}
	}
	return nil
}

func stripMemoryChainLineComments(ddl string) string {
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

func splitMemoryChainStatements(ddl string) []string {
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

func memoryChainSnippet(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
