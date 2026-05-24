package data

import (
	"strings"
	"testing"
)

func TestBuildTimelineUnionSQL_allKinds(t *testing.T) {
	sql, args := buildTimelineUnionSQL("sess-1", "")
	if !strings.Contains(sql, "messages") || !strings.Contains(sql, "tool_invocations") || !strings.Contains(sql, "skill_invocation") {
		t.Fatalf("expected all branches, got %q", sql)
	}
	if len(args) != 3 {
		t.Fatalf("expected 3 session args, got %d", len(args))
	}
}

func TestBuildTimelineUnionSQL_toolOnly(t *testing.T) {
	sql, args := buildTimelineUnionSQL("sess-1", "tool")
	if strings.Contains(sql, "skill_invocation") || strings.Contains(sql, "messages") {
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
	sql, _ := buildTimelineUnionSQL("sess-1", "mcp")
	if !strings.Contains(sql, "lower(source) = 'mcp'") {
		t.Fatalf("expected mcp filter, got %q", sql)
	}
	if strings.Contains(sql, "NOT") {
		t.Fatalf("did not expect NOT filter for mcp-only query")
	}
}
