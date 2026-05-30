package biz_test

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestParseChannelIMRenderPolicy(t *testing.T) {
	cases := []struct {
		name        string
		configJSON  string
		ltCfg       biz.ChannelLongTaskConfig
		wantMode    string
		wantDetail  string
		wantTeam    string
	}{
		{
			name:       "defaults with empty config",
			configJSON: `{}`,
			ltCfg:      biz.ChannelLongTaskConfig{},
			wantMode:   biz.ChannelIMRenderModeReplyOnly,
			wantDetail: biz.ChannelIMToolDetailLabelSummary,
			wantTeam:   biz.ChannelIMTeamModeOff,
		},
		{
			name:       "transcript mode",
			configJSON: `{"config":{"im_render_mode":"transcript"}}`,
			ltCfg:      biz.ChannelLongTaskConfig{},
			wantMode:   biz.ChannelIMRenderModeTranscript,
		},
		{
			name:       "transcript with reasoning auto enables show reasoning",
			configJSON: `{"config":{"im_render_mode":"transcript_with_reasoning"}}`,
			ltCfg:      biz.ChannelLongTaskConfig{},
			wantMode:   biz.ChannelIMRenderModeTranscriptWithReasoning,
		},
		{
			name:       "legacy progress mode text",
			configJSON: `{}`,
			ltCfg:      biz.ChannelLongTaskConfig{ProgressMode: "text"},
			wantMode:   biz.ChannelIMRenderModeTranscript,
		},
		{
			name:       "legacy progress mode steps",
			configJSON: `{}`,
			ltCfg:      biz.ChannelLongTaskConfig{ProgressMode: "steps"},
			wantMode:   biz.ChannelIMRenderModeTranscript,
			wantTeam:   biz.ChannelIMTeamModeSteps,
		},
		{
			name:       "im_render_mode overrides legacy",
			configJSON: `{"config":{"im_render_mode":"reply_only"}}`,
			ltCfg:      biz.ChannelLongTaskConfig{ProgressMode: "text"},
			wantMode:   biz.ChannelIMRenderModeReplyOnly,
		},
		{
			name:       "im_tool_detail off",
			configJSON: `{"config":{"im_tool_detail":"off"}}`,
			ltCfg:      biz.ChannelLongTaskConfig{},
			wantDetail: biz.ChannelIMToolDetailOff,
		},
		{
			name:       "im_team_mode inline",
			configJSON: `{"config":{"im_team_mode":"inline"}}`,
			ltCfg:      biz.ChannelLongTaskConfig{},
			wantTeam:   biz.ChannelIMTeamModeInline,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			policy := biz.ParseChannelIMRenderPolicy(tc.configJSON, tc.ltCfg)
			if tc.wantMode != "" && policy.Mode != tc.wantMode {
				t.Fatalf("Mode = %q, want %q", policy.Mode, tc.wantMode)
			}
			if tc.wantDetail != "" && policy.ToolDetail != tc.wantDetail {
				t.Fatalf("ToolDetail = %q, want %q", policy.ToolDetail, tc.wantDetail)
			}
			if tc.wantTeam != "" && policy.TeamMode != tc.wantTeam {
				t.Fatalf("TeamMode = %q, want %q", policy.TeamMode, tc.wantTeam)
			}
		})
	}
}

func TestParseChannelIMRenderPolicy_ShowReasoning(t *testing.T) {
	cases := []struct {
		name       string
		configJSON string
		ltCfg      biz.ChannelLongTaskConfig
		want       bool
	}{
		{
			name:       "default no reasoning",
			configJSON: `{}`,
			ltCfg:      biz.ChannelLongTaskConfig{},
			want:       false,
		},
		{
			name:       "explicit show reasoning true",
			configJSON: `{"config":{"im_show_reasoning":true}}`,
			ltCfg:      biz.ChannelLongTaskConfig{},
			want:       true,
		},
		{
			name:       "transcript_with_reasoning auto enables",
			configJSON: `{"config":{"im_render_mode":"transcript_with_reasoning"}}`,
			ltCfg:      biz.ChannelLongTaskConfig{},
			want:       true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			policy := biz.ParseChannelIMRenderPolicy(tc.configJSON, tc.ltCfg)
			if policy.ShowReasoning != tc.want {
				t.Fatalf("ShowReasoning = %v, want %v", policy.ShowReasoning, tc.want)
			}
		})
	}
}

func TestParseChannelIMRenderPolicy_CustomValues(t *testing.T) {
	configJSON := `{"config":{"im_reasoning_max_chars":1000,"im_max_preview_chars":8000,"im_tool_card_mode":"feishu_append","im_split_overflow":true,"progress_quiet_sec":30,"heartbeat_message":"custom heartbeat"}}`
	policy := biz.ParseChannelIMRenderPolicy(configJSON, biz.ChannelLongTaskConfig{})
	if policy.ReasoningMaxRunes != 1000 {
		t.Fatalf("ReasoningMaxRunes = %d, want 1000", policy.ReasoningMaxRunes)
	}
	if policy.MaxPreviewRunes != 8000 {
		t.Fatalf("MaxPreviewRunes = %d, want 8000", policy.MaxPreviewRunes)
	}
	if policy.ToolCardMode != biz.ChannelIMToolCardModeFeishuAppend {
		t.Fatalf("ToolCardMode = %q, want %q", policy.ToolCardMode, biz.ChannelIMToolCardModeFeishuAppend)
	}
	if !policy.SplitOverflow {
		t.Fatalf("SplitOverflow = false, want true")
	}
	if policy.HeartbeatQuietSec != 30 {
		t.Fatalf("HeartbeatQuietSec = %d, want 30", policy.HeartbeatQuietSec)
	}
	if policy.HeartbeatMessage != "custom heartbeat" {
		t.Fatalf("HeartbeatMessage = %q, want %q", policy.HeartbeatMessage, "custom heartbeat")
	}
}

func TestChannelACKDeferredToPreview(t *testing.T) {
	cases := []struct {
		name       string
		configJSON string
		platform   string
		want       bool
	}{
		{"streaming feishu", `{"config":{"streaming_enabled":true}}`, "feishu", true},
		{"streaming lark", `{"config":{"streaming_enabled":true}}`, "lark", true},
		{"streaming slack", `{"config":{"streaming_enabled":true}}`, "slack", true},
		{"streaming telegram", `{"config":{"streaming_enabled":true}}`, "telegram", true},
		{"streaming unknown platform", `{"config":{"streaming_enabled":true}}`, "wechat", false},
		{"no streaming", `{"config":{"streaming_enabled":false}}`, "feishu", false},
		{"empty config", `{}`, "feishu", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.ChannelACKDeferredToPreview(tc.configJSON, tc.platform)
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNormalizeIMRenderMode(t *testing.T) {
	cases := []struct {
		name  string
		mode  string
		want  string
	}{
		{"transcript", "transcript", biz.ChannelIMRenderModeTranscript},
		{"transcript with reasoning", "transcript_with_reasoning", biz.ChannelIMRenderModeTranscriptWithReasoning},
		{"reply only", "reply_only", biz.ChannelIMRenderModeReplyOnly},
		{"unknown defaults reply_only", "unknown", biz.ChannelIMRenderModeReplyOnly},
		{"uppercase", "TRANSCRIPT", biz.ChannelIMRenderModeTranscript},
		{"with spaces", "  transcript  ", biz.ChannelIMRenderModeTranscript},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.NormalizeIMRenderMode(tc.mode)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeIMToolDetail(t *testing.T) {
	cases := []struct {
		name   string
		detail string
		want   string
	}{
		{"off", "off", biz.ChannelIMToolDetailOff},
		{"label", "label", biz.ChannelIMToolDetailLabel},
		{"label_summary", "label_summary", biz.ChannelIMToolDetailLabelSummary},
		{"unknown defaults label_summary", "unknown", biz.ChannelIMToolDetailLabelSummary},
		{"uppercase", "OFF", biz.ChannelIMToolDetailOff},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.NormalizeIMToolDetail(tc.detail)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeIMTeamMode(t *testing.T) {
	cases := []struct {
		name string
		mode string
		want string
	}{
		{"inline", "inline", biz.ChannelIMTeamModeInline},
		{"steps", "steps", biz.ChannelIMTeamModeSteps},
		{"off", "off", biz.ChannelIMTeamModeOff},
		{"unknown defaults off", "unknown", biz.ChannelIMTeamModeOff},
		{"uppercase", "INLINE", biz.ChannelIMTeamModeInline},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.NormalizeIMTeamMode(tc.mode)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeIMToolCardMode(t *testing.T) {
	cases := []struct {
		name string
		mode string
		want string
	}{
		{"feishu_append", "feishu_append", biz.ChannelIMToolCardModeFeishuAppend},
		{"off", "off", biz.ChannelIMToolCardModeOff},
		{"unknown defaults off", "unknown", biz.ChannelIMToolCardModeOff},
		{"uppercase", "FEISHU_APPEND", biz.ChannelIMToolCardModeFeishuAppend},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.NormalizeIMToolCardMode(tc.mode)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestApplyLegacyProgressMode(t *testing.T) {
	cases := []struct {
		name        string
		progressMode string
		wantMode    string
		wantDetail  string
		wantTeam    string
	}{
		{"text mode", "text", biz.ChannelIMRenderModeTranscript, biz.ChannelIMToolDetailLabelSummary, biz.ChannelIMTeamModeOff},
		{"steps mode", "steps", biz.ChannelIMRenderModeTranscript, biz.ChannelIMToolDetailLabelSummary, biz.ChannelIMTeamModeSteps},
		{"off mode", "off", biz.ChannelIMRenderModeReplyOnly, biz.ChannelIMToolDetailLabelSummary, biz.ChannelIMTeamModeOff},
		{"empty mode", "", biz.ChannelIMRenderModeReplyOnly, biz.ChannelIMToolDetailLabelSummary, biz.ChannelIMTeamModeOff},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			policy := &biz.ChannelIMRenderPolicy{
				Mode:       biz.ChannelIMRenderModeReplyOnly,
				ToolDetail: biz.ChannelIMToolDetailLabelSummary,
				TeamMode:   biz.ChannelIMTeamModeOff,
			}
			ltCfg := biz.ChannelLongTaskConfig{ProgressMode: tc.progressMode}
			biz.ApplyLegacyProgressMode(policy, ltCfg)
			if policy.Mode != tc.wantMode {
				t.Fatalf("Mode = %q, want %q", policy.Mode, tc.wantMode)
			}
			if policy.ToolDetail != tc.wantDetail {
				t.Fatalf("ToolDetail = %q, want %q", policy.ToolDetail, tc.wantDetail)
			}
			if policy.TeamMode != tc.wantTeam {
				t.Fatalf("TeamMode = %q, want %q", policy.TeamMode, tc.wantTeam)
			}
		})
	}
}
