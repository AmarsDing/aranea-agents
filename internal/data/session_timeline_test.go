package data

import (
	"strings"
	"testing"
)

func TestBuildTimelineUnionSQL_allKinds(t *testing.T) {
	sql, args := buildTimelineUnionSQL("sess-1", "", DialectPostgres)
	if !strings.Contains(sql, "steps_v2") || !strings.Contains(sql, "tool_invocations") || !strings.Contains(sql, "skill_invocation") {
		t.Fatalf("expected all branches, got %q", sql)
	}
	if len(args) != 3 {
		t.Fatalf("expected 3 session args, got %d", len(args))
	}
}

func TestBuildTimelineUnionSQL_toolOnly(t *testing.T) {
	sql, args := buildTimelineUnionSQL("sess-1", "tool", DialectPostgres)
	if strings.Contains(sql, "skill_invocation") || strings.Contains(sql, "steps_v2") {
		t.Fatalf("unexpected branches in tool-only query: %q", sql)
	}
	if !strings.Contains(sql, "NOT") {
		t.Fatalf("expected mcp exclusion, got %q", sql)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
}

func TestBuildTimelineUnionSQL_mcpOnly(t *testing.T) {
	sql, _ := buildTimelineUnionSQL("sess-1", "mcp", DialectPostgres)
	if !strings.Contains(sql, "lower(source) = 'mcp'") {
		t.Fatalf("expected mcp filter, got %q", sql)
	}
	if strings.Contains(sql, "NOT") {
		t.Fatalf("did not expect NOT filter for mcp-only query")
	}
}

func TestBuildTimelineUnionSQL_messageBranchUsesStepsV2(t *testing.T) {
	// Postgres: uses to_char(... AT TIME ZONE 'UTC', ...) to format started_at
	pgSQL, _ := buildTimelineUnionSQL("sess-1", "message", DialectPostgres)
	if !strings.Contains(pgSQL, "steps_v2") {
		t.Fatalf("postgres message branch should query steps_v2, got %q", pgSQL)
	}
	if !strings.Contains(pgSQL, "spirit_session_id") {
		t.Fatalf("postgres message branch should filter by spirit_session_id, got %q", pgSQL)
	}
	if !strings.Contains(pgSQL, "to_char") {
		t.Fatalf("postgres message branch should format timestamp via to_char, got %q", pgSQL)
	}
	if !strings.Contains(pgSQL, "AT TIME ZONE 'UTC'") {
		t.Fatalf("postgres message branch should normalize to UTC, got %q", pgSQL)
	}
	if !strings.Contains(pgSQL, "kind IN ('task', 'reply')") {
		t.Fatalf("postgres message branch should filter kind IN task/reply, got %q", pgSQL)
	}

	// SQLite: started_at is TEXT already; no to_char needed
	sqliteSQL, _ := buildTimelineUnionSQL("sess-1", "message", DialectSQLite)
	if !strings.Contains(sqliteSQL, "steps_v2") {
		t.Fatalf("sqlite message branch should query steps_v2, got %q", sqliteSQL)
	}
	if strings.Contains(sqliteSQL, "to_char") {
		t.Fatalf("sqlite message branch should not use to_char, got %q", sqliteSQL)
	}
}
