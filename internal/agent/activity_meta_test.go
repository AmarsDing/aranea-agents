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
	summary := BuildSummary("tool", "read_file", []byte(`{"file_name":"/tmp/a.txt"}`))
	if !strings.Contains(summary, "/tmp/a.txt") {
		t.Fatalf("summary: %q", summary)
	}
	summary = BuildSummary("tool", "diff_edit", []byte(`{"file_name":"foo.go","edits":[{"search":"a","replace":"b"}]}`))
	if !strings.Contains(summary, "foo.go") || !strings.Contains(summary, "1 edit") {
		t.Fatalf("diff_edit summary: %q", summary)
	}
	summary = BuildSummary("mcp", "mcp_call", []byte(`{"server_key":"github","tool_name":"list_issues"}`))
	if summary != "github/list_issues" {
		t.Fatalf("mcp summary: %q", summary)
	}
}

func TestFileEditResultSummary(t *testing.T) {
	got := fileEditResultSummary("diff_edit", `{"applied_edits":2,"total_replacements":3,"structured_patch":[]}`)
	if got != "2 applied · 3 repl" {
		t.Fatalf("got %q", got)
	}
	got = fileEditResultSummary("patch_file", `{"applied_hunks":2,"structured_patch":[{"old_start":1}]}`)
	if got != "2 hunk(s) applied" {
		t.Fatalf("patch_file applied_hunks: got %q", got)
	}
	got = fileEditResultSummary("patch_file", `{"structured_patch":[{"old_start":1}]}`)
	if got != "1 hunk(s)" {
		t.Fatalf("got %q", got)
	}
}

func TestBuildActivityMetaFileEditSummary(t *testing.T) {
	meta := BuildActivityMeta(context.Background(), ActivityMetaInput{
		ToolName:      "diff_edit",
		ArgumentsJSON: `{"file_name":"x.go","edits":[{"search":"a","replace":"b"}]}`,
		ResultJSON:    `{"applied_edits":1,"total_replacements":1}`,
		Status:        "success",
	}, nil)
	if !strings.Contains(meta.Summary, "x.go") || !strings.Contains(meta.Summary, "applied") {
		t.Fatalf("summary: %q", meta.Summary)
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
