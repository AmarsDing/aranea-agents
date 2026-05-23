package preview

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatIMSectionedReply(t *testing.T) {
	out := FormatIMSectionedReply("feishu", "think step", "**结论** ok")
	if !strings.Contains(out, "【思考过程】") || !strings.Contains(out, "【正文】") {
		t.Fatalf("missing section labels: %q", out)
	}
}

func TestBuildFeishuChannelReplyCardJSON_sections(t *testing.T) {
	raw, err := BuildFeishuChannelReplyCardJSON("reasoning text", "body text")
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid([]byte(raw)) {
		t.Fatal("invalid json")
	}
	if !strings.Contains(raw, "思考过程") || !strings.Contains(raw, "正文") {
		t.Fatalf("missing labels: %s", raw)
	}
}

func TestReasoningMarkdownFromOptions(t *testing.T) {
	opts := `{"reasoning_markdown":"step one"}`
	if got := ReasoningMarkdownFromOptions(opts); got != "step one" {
		t.Fatalf("got %q", got)
	}
}
