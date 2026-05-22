package biz

import "testing"

func TestParseChannelIMRenderPolicy_defaults(t *testing.T) {
	lt := ParseChannelLongTaskConfig(`{"config":{}}`)
	p := ParseChannelIMRenderPolicy(`{"config":{}}`, lt)
	if p.Mode != ChannelIMRenderModeReplyOnly {
		t.Fatalf("Mode=%q", p.Mode)
	}
	if p.ToolDetail != ChannelIMToolDetailLabelSummary {
		t.Fatalf("ToolDetail=%q", p.ToolDetail)
	}
}

func TestParseChannelIMRenderPolicy_legacyProgressText(t *testing.T) {
	raw := `{"config":{"progress_mode":"text"}}`
	lt := ParseChannelLongTaskConfig(raw)
	p := ParseChannelIMRenderPolicy(raw, lt)
	if p.Mode != ChannelIMRenderModeTranscript {
		t.Fatalf("Mode=%q", p.Mode)
	}
}

func TestParseChannelIMRenderPolicy_explicit(t *testing.T) {
	raw := `{"config":{
		"im_render_mode":"transcript_with_reasoning",
		"im_tool_detail":"off",
		"im_team_mode":"steps",
		"im_max_preview_chars":8000
	}}`
	lt := ParseChannelLongTaskConfig(raw)
	p := ParseChannelIMRenderPolicy(raw, lt)
	if !p.ShowReasoning {
		t.Fatal("expected show reasoning")
	}
	if p.ToolDetail != ChannelIMToolDetailOff || p.TeamMode != ChannelIMTeamModeSteps {
		t.Fatalf("tool=%q team=%q", p.ToolDetail, p.TeamMode)
	}
	if p.MaxPreviewRunes != 8000 {
		t.Fatalf("max=%d", p.MaxPreviewRunes)
	}
}

func TestParseChannelIMRenderPolicy_toolCardAndSplit(t *testing.T) {
	raw := `{"config":{"im_tool_card_mode":"feishu_append","im_split_overflow":true}}`
	lt := ParseChannelLongTaskConfig(raw)
	p := ParseChannelIMRenderPolicy(raw, lt)
	if p.ToolCardMode != ChannelIMToolCardModeFeishuAppend {
		t.Fatalf("tool card=%q", p.ToolCardMode)
	}
	if !p.SplitOverflow {
		t.Fatal("expected split overflow")
	}
}

func TestChannelACKDeferredToPreview(t *testing.T) {
	cfg := `{"config":{"streaming_enabled":true}}`
	if !ChannelACKDeferredToPreview(cfg, "feishu") {
		t.Fatal("feishu streaming should defer ack")
	}
	if ChannelACKDeferredToPreview(cfg, "dingtalk") {
		t.Fatal("dingtalk should not defer ack")
	}
	if ChannelACKDeferredToPreview(`{"config":{}}`, "feishu") {
		t.Fatal("non-streaming should not defer")
	}
}
