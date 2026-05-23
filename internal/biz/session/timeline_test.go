package session

import "testing"

func TestMessageTimelineItem_userRole(t *testing.T) {
	item := messageTimelineItem(ChatMessage{
		ID:              "m1",
		Role:            "user",
		ContentMarkdown: "hello world",
		CreatedAt:       "2026-05-21T10:00:00Z",
	})
	if item.Kind != "message" || item.Title != "用户消息" {
		t.Fatalf("unexpected item: %+v", item)
	}
	if item.Preview != "hello world" {
		t.Fatalf("preview: %q", item.Preview)
	}
}

func TestToolTimelineItem_mcpTag(t *testing.T) {
	item := toolTimelineItem(ToolInvocationView{
		ID:      "t1",
		ToolKey: "mcp_list_tools",
		Source:  "mcp",
		Status:  "success",
	})
	if item.Kind != "mcp" {
		t.Fatalf("expected mcp kind, got %q", item.Kind)
	}
}

func TestPreviewTimelineText_truncates(t *testing.T) {
	long := "one two three four five six seven eight nine ten eleven"
	got := previewTimelineText(long, 10)
	if len([]rune(got)) <= 10 {
		t.Fatalf("expected truncated preview with ellipsis, got %q", got)
	}
}

func TestTimelineFirstNonEmpty(t *testing.T) {
	if timelineFirstNonEmpty("", "  ", "ok") != "ok" {
		t.Fatal("expected first non-empty value")
	}
}
