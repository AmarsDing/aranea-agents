package preview

import (
	"strings"
	"testing"
)

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

// Phase 1c-5: TestTranscriptApplyTextAndToolOrder removed — tested deleted
// EnvelopeType TextDelta/ToolCall rendering behavior.
// Phase 1c-5: TestTranscriptApply_sanitizesEmptyThinking removed — tested deleted
// EnvelopeType TextDelta rendering behavior.
// Phase 1c-5: TestRenderReplyOnly removed — tested deleted EnvelopeType TextDone
// rendering behavior.
