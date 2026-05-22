package preview

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildFeishuToolCardJSON_mcpSuccessOneLine(t *testing.T) {
	raw, err := BuildFeishuToolCardJSON(Segment{
		Kind:   SegmentTool,
		ID:     "t1",
		Status: ToolStatusOK,
		Meta: ToolSegmentMeta{
			ActivityKind: "mcp",
			DisplayLabel: "read_file",
			Summary:      "path/to/file.go",
			Name:         "read_file",
			DurationMS:   1250,
		},
	}, ToolCardBuildOpts{
		SessionID: "sess-1",
		WebOrigin: "https://app.example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	var card map[string]any
	if err := json.Unmarshal([]byte(raw), &card); err != nil {
		t.Fatal(err)
	}
	header, _ := card["header"].(map[string]any)
	if header["template"] != "green" {
		t.Fatalf("template=%v", header["template"])
	}
	if !strings.Contains(raw, "Web 详情") {
		t.Fatal("missing web button")
	}
	if !strings.Contains(raw, "sessions/sess-1") || !strings.Contains(raw, "tool_id=t1") {
		t.Fatalf("missing session url: %s", raw)
	}
	if !strings.Contains(raw, "color='green'") || !strings.Contains(raw, "✓") {
		t.Fatalf("missing green check: %s", raw)
	}
	if !strings.Contains(raw, "**MCP**") || !strings.Contains(raw, "read_file") {
		t.Fatalf("missing one-line content: %s", raw)
	}
}

func TestBuildFeishuToolCardJSON_inFlight(t *testing.T) {
	raw, err := BuildFeishuToolCardJSON(Segment{
		Kind:   SegmentTool,
		ID:     "t1",
		Status: ToolStatusCalling,
		Meta: ToolSegmentMeta{
			ActivityKind: "skill",
			DisplayLabel: "run_skill",
			Summary:      "deploy",
		},
	}, ToolCardBuildOpts{SessionID: "s1", WebOrigin: "https://x.com"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, `"template":"orange"`) {
		t.Fatalf("expected orange header: %s", raw)
	}
	if !strings.Contains(raw, "🔄") || !strings.Contains(raw, "进行中") {
		t.Fatalf("expected in-flight marker: %s", raw)
	}
	if !strings.Contains(raw, "**Skill**") {
		t.Fatalf("expected skill template: %s", raw)
	}
}

func TestBuildFeishuToolCardJSON_error(t *testing.T) {
	raw, err := BuildFeishuToolCardJSON(Segment{
		Kind:   SegmentTool,
		ID:     "t2",
		Status: ToolStatusError,
		Meta: ToolSegmentMeta{
			DisplayLabel:  "exec_command",
			ErrorCode:     "E_TIMEOUT",
			ResultExcerpt: "timed out",
		},
	}, ToolCardBuildOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, `"template":"red"`) {
		t.Fatalf("expected red template: %s", raw)
	}
	if !strings.Contains(raw, "color='red'") || !strings.Contains(raw, "✕") {
		t.Fatalf("expected red x: %s", raw)
	}
}

func TestBuildSessionWebURL_encodesToolID(t *testing.T) {
	got := BuildSessionWebURL("https://app.test", "abc", "tool/id+1")
	if !strings.Contains(got, "tool_id=tool%2Fid%2B1") {
		t.Fatalf("expected encoded tool_id, got %q", got)
	}
}

func TestBuildSessionWebURL(t *testing.T) {
	got := BuildSessionWebURL("https://app.test/", "abc", "tool-1")
	want := "https://app.test/sessions/abc?focus=tool&tool_id=tool-1"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFormatToolOneLinePlain(t *testing.T) {
	out := FormatToolOneLinePlain(Segment{
		Status: ToolStatusOK,
		Meta: ToolSegmentMeta{
			ActivityKind: "mcp",
			DisplayLabel: "grep",
			DurationMS:   500,
		},
	})
	if !strings.Contains(out, "📡") || !strings.Contains(out, "✓") || !strings.Contains(out, "500ms") {
		t.Fatalf("out=%q", out)
	}
}
