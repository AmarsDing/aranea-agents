package agent

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestClassifyActivityKind(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"skill_run", "skill"},
		{"skill_load", "skill"},
		{"mcp_call", "mcp"},
		{"mcp:github/list_issues", "mcp"},
		{"read_file", "tool"},
		{"knowledge_search", "knowledge"},
		{"await_user_reply", "session"},
		{"call_agent", "subagent"},
	}
	for _, tt := range tests {
		if got := ClassifyActivityKind(tt.name); got != tt.want {
			t.Fatalf("%s: got %q want %q", tt.name, got, tt.want)
		}
	}
}

func TestResolveDisplayLabel(t *testing.T) {
	if got := ResolveDisplayLabel("skill_run"); got != "运行 Skill" {
		t.Fatalf("skill_run label: %q", got)
	}
	if got := ResolveDisplayLabel("custom_tool"); got != "custom_tool" {
		t.Fatalf("custom_tool label: %q", got)
	}
}

func TestResolveIconKey(t *testing.T) {
	if got := ResolveIconKey("skill", "skill_run"); got != "play_circle" {
		t.Fatalf("skill_run icon: %q", got)
	}
	if got := ResolveIconKey("mcp", "mcp_call"); got != "hub" {
		t.Fatalf("mcp icon: %q", got)
	}
}

func TestBuildSummary(t *testing.T) {
	summary := BuildSummary("tool", "read_file", []byte(`{"path":"/tmp/a.txt"}`))
	if !strings.Contains(summary, "/tmp/a.txt") {
		t.Fatalf("summary: %q", summary)
	}
	summary = BuildSummary("mcp", "mcp_call", []byte(`{"server_key":"github","tool_name":"list_issues"}`))
	if summary != "github/list_issues" {
		t.Fatalf("mcp summary: %q", summary)
	}
}

func TestSanitizeJSONString(t *testing.T) {
	raw := `{"api_key":"secret123","path":"/tmp/a.txt"}`
	out := SanitizeJSONString(raw)
	if strings.Contains(out, "secret123") {
		t.Fatalf("expected redacted api_key, got %q", out)
	}
	if !strings.Contains(out, "/tmp/a.txt") {
		t.Fatalf("expected path preserved, got %q", out)
	}
}

func TestBuildActivityMetaDuration(t *testing.T) {
	started := parseTestTime(t, "2026-05-20T10:00:00Z")
	finished := parseTestTime(t, "2026-05-20T10:00:01.2Z")
	meta := BuildActivityMeta(context.Background(), ActivityMetaInput{
		ToolName:      "read_file",
		ArgumentsJSON: `{"path":"x"}`,
		Status:        "success",
		Author:        "agent-1",
		StartedAt:     started,
		FinishedAt:    &finished,
		DurationMS:    1200,
	}, nil)
	if meta.ActivityKind != "tool" {
		t.Fatalf("kind: %q", meta.ActivityKind)
	}
	if meta.DurationMS != 1200 {
		t.Fatalf("duration: %d", meta.DurationMS)
	}
	if meta.AgentKey != "agent-1" {
		t.Fatalf("agent key: %q", meta.AgentKey)
	}
}

func parseTestTime(t *testing.T, value string) (tm time.Time) {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
