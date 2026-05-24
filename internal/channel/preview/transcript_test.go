package preview

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
)

func TestTranscriptApplyTextAndToolOrder(t *testing.T) {
	tr := NewTranscript()
	tr.Apply(event.Envelope{
		Type: event.EnvelopeTypeTextDelta,
		Content: &event.EnvelopeContent{
			Text:      "hello",
			IsPartial: true,
		},
	})
	tr.Apply(event.Envelope{
		Type: event.EnvelopeTypeToolCall,
		ToolCall: &event.EnvelopeToolCall{
			ID:           "tc1",
			Name:         "mcp_call",
			Status:       "calling",
			ActivityKind: "mcp",
			DisplayLabel: "MCP 调用",
			Summary:      "srv/tool",
		},
	})
	tr.Apply(event.Envelope{
		Type: event.EnvelopeTypeTextDelta,
		Content: &event.EnvelopeContent{
			Text:      " world",
			IsPartial: true,
		},
	})

	policy := biz.ChannelIMRenderPolicy{
		Mode:       biz.ChannelIMRenderModeTranscript,
		ToolDetail: biz.ChannelIMToolDetailLabelSummary,
	}
	out := RenderPlainText(tr, policy)
	idxHello := strings.Index(out, "hello")
	idxMCP := strings.Index(out, "MCP 调用")
	idxWorld := strings.Index(out, "world")
	if idxHello < 0 || idxMCP < 0 || idxWorld < 0 || !(idxHello < idxMCP && idxMCP < idxWorld) {
		t.Fatalf("order wrong:\n%s", out)
	}
}

func TestTranscriptApply_sanitizesEmptyThinking(t *testing.T) {
	tr := NewTranscript()
	tr.Apply(event.Envelope{
		Type: event.EnvelopeTypeTextDelta,
		Content: &event.EnvelopeContent{
			Reasoning: "<thinking></thinking>",
			IsPartial: true,
		},
	})
	tr.Apply(event.Envelope{
		Type: event.EnvelopeTypeTextDelta,
		Content: &event.EnvelopeContent{
			Text:      "answer",
			IsPartial: true,
		},
	})
	out := RenderPlainText(tr, biz.ChannelIMRenderPolicy{Mode: biz.ChannelIMRenderModeTranscript})
	if strings.Contains(out, "thinking") || strings.Contains(out, "【思考过程】") {
		t.Fatalf("thinking leaked: %q", out)
	}
	if !strings.Contains(out, "answer") {
		t.Fatalf("missing answer: %q", out)
	}
}

func TestFormatRenderedTranscriptForIM_stripsInlineThinkingTags(t *testing.T) {
	raw := "【思考过程】\n<thinking></thinking>\n【正文】\nok"
	out := FormatRenderedTranscriptForIM("feishu", raw)
	if strings.Contains(out, "<thinking>") {
		t.Fatalf("tag leaked: %q", out)
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("missing body: %q", out)
	}
}
func TestRenderReplyOnly(t *testing.T) {
	tr := NewTranscript()
	tr.SetSystem("收到")
	tr.Apply(event.Envelope{
		Type: event.EnvelopeTypeTextDone,
		Content: &event.EnvelopeContent{Text: "final answer"},
	})
	out := RenderPlainText(tr, biz.ChannelIMRenderPolicy{Mode: biz.ChannelIMRenderModeReplyOnly})
	if out != "final answer" {
		t.Fatalf("got %q", out)
	}
}
