package preview

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
)

func TestRenderPlainText_compactWhenToolCardAppend(t *testing.T) {
	tr := NewTranscript()
	tr.Apply(event.Envelope{
		Type: event.EnvelopeTypeToolCall,
		ToolCall: &event.EnvelopeToolCall{
			ID: "t1", Name: "read_file", Status: "ok",
			ActivityKind: "mcp", DisplayLabel: "读取文件", Summary: "a.go",
			DurationMS: 500,
		},
	})
	out := RenderPlainText(tr, biz.ChannelIMRenderPolicy{
		Mode:         biz.ChannelIMRenderModeTranscript,
		ToolDetail:   biz.ChannelIMToolDetailLabelSummary,
		ToolCardMode: biz.ChannelIMToolCardModeFeishuAppend,
	})
	if !strings.Contains(out, "📡") || !strings.Contains(out, "MCP") || !strings.Contains(out, "读取文件") || !strings.Contains(out, "✓") {
		t.Fatalf("expected compact one-line tool preview, got %q", out)
	}
}
