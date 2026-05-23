package preview

import (
	"strings"
	"testing"
)

func TestFormatAssistantReplyForIM_reactFinalAnswer(t *testing.T) {
	raw := "/*PLANNING*/\nplan\n/*REASONING*/\nthink\n/*FINAL_ANSWER*/\n**结论**：完成。"
	out := FormatAssistantReplyForIM("feishu", raw)
	if strings.Contains(out, "PLANNING") || strings.Contains(out, "REASONING") {
		t.Fatalf("react tags leaked: %q", out)
	}
	if !strings.Contains(out, "结论") {
		t.Fatalf("missing final answer: %q", out)
	}
}

func TestFormatAssistantReplyForIM_markdownCleanup(t *testing.T) {
	raw := "# 标题\n\n- item one\n\n[文档](https://example.com)\n\n```go\nfmt.Println(\"hi\")\n```"
	out := FormatAssistantReplyForIM("feishu", raw)
	if strings.Contains(out, "# ") {
		t.Fatalf("header markdown left: %q", out)
	}
	if !strings.Contains(out, "【标题】") {
		t.Fatalf("expected title formatting: %q", out)
	}
	if !strings.Contains(out, "• item one") {
		t.Fatalf("expected bullet: %q", out)
	}
	if !strings.Contains(out, "文档 (https://example.com)") {
		t.Fatalf("expected link formatting: %q", out)
	}
	if !strings.Contains(out, "fmt.Println") {
		t.Fatalf("expected code body: %q", out)
	}
}

func TestFormatAssistantReplyForIM_stripsA2UI(t *testing.T) {
	raw := "结果如下\n{\"type\":\"surfaceUpdate\",\"surface\":\"main\"}\n谢谢"
	out := FormatAssistantReplyForIM("feishu", raw)
	if strings.Contains(out, "surfaceUpdate") {
		t.Fatalf("a2ui json leaked: %q", out)
	}
	if !strings.Contains(out, "结果如下") || !strings.Contains(out, "谢谢") {
		t.Fatalf("unexpected: %q", out)
	}
}

func TestFormatRenderedTranscriptForIM_keepsToolLines(t *testing.T) {
	raw := "▸ MCP · web_search\n  ✓ 完成\n\n/*PLANNING*/ plan\n/*FINAL_ANSWER*/\n【结论】完成"
	out := FormatRenderedTranscriptForIM("feishu", raw)
	if !strings.Contains(out, "▸ MCP") {
		t.Fatalf("tool line stripped: %q", out)
	}
	if strings.Contains(out, "PLANNING") {
		t.Fatalf("react tag markers should remain or be md-only: %q", out)
	}
}

func TestFormatRenderedTranscriptForIM_preservesPlainJSON(t *testing.T) {
	raw := "说明\n{\"type\":\"note\",\"text\":\"hello\"}\n结束"
	out := FormatRenderedTranscriptForIM("feishu", raw)
	if !strings.Contains(out, `"type":"note"`) {
		t.Fatalf("non-a2ui json removed: %q", out)
	}
}
