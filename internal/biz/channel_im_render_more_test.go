package biz

import "testing"

func TestNormalizeIMRenderMode(t *testing.T) {
	cases := []struct {
		mode string
		want string
	}{
		{"transcript", ChannelIMRenderModeTranscript},
		{"TRANSCRIPT", ChannelIMRenderModeTranscript},
		{"  transcript  ", ChannelIMRenderModeTranscript},
		{"transcript_with_reasoning", ChannelIMRenderModeTranscriptWithReasoning},
		{"reply_only", ChannelIMRenderModeReplyOnly},
		{"unknown", ChannelIMRenderModeReplyOnly},
		{"", ChannelIMRenderModeReplyOnly},
	}
	for _, tc := range cases {
		got := normalizeIMRenderMode(tc.mode)
		if got != tc.want {
			t.Errorf("normalizeIMRenderMode(%q) = %q, want %q", tc.mode, got, tc.want)
		}
	}
}

func TestNormalizeIMToolDetail(t *testing.T) {
	cases := []struct {
		detail string
		want   string
	}{
		{"off", ChannelIMToolDetailOff},
		{"OFF", ChannelIMToolDetailOff},
		{"label", ChannelIMToolDetailLabel},
		{"label_summary", ChannelIMToolDetailLabelSummary},
		{"unknown", ChannelIMToolDetailLabelSummary},
		{"", ChannelIMToolDetailLabelSummary},
	}
	for _, tc := range cases {
		got := normalizeIMToolDetail(tc.detail)
		if got != tc.want {
			t.Errorf("normalizeIMToolDetail(%q) = %q, want %q", tc.detail, got, tc.want)
		}
	}
}

func TestNormalizeIMTeamMode(t *testing.T) {
	cases := []struct {
		mode string
		want string
	}{
		{"inline", ChannelIMTeamModeInline},
		{"INLINE", ChannelIMTeamModeInline},
		{"steps", ChannelIMTeamModeSteps},
		{"off", ChannelIMTeamModeOff},
		{"unknown", ChannelIMTeamModeOff},
		{"", ChannelIMTeamModeOff},
	}
	for _, tc := range cases {
		got := normalizeIMTeamMode(tc.mode)
		if got != tc.want {
			t.Errorf("normalizeIMTeamMode(%q) = %q, want %q", tc.mode, got, tc.want)
		}
	}
}

func TestNormalizeIMToolCardMode(t *testing.T) {
	cases := []struct {
		mode string
		want string
	}{
		{"feishu_append", ChannelIMToolCardModeFeishuAppend},
		{"FEISHU_APPEND", ChannelIMToolCardModeFeishuAppend},
		{"off", ChannelIMToolCardModeOff},
		{"unknown", ChannelIMToolCardModeOff},
		{"", ChannelIMToolCardModeOff},
	}
	for _, tc := range cases {
		got := normalizeIMToolCardMode(tc.mode)
		if got != tc.want {
			t.Errorf("normalizeIMToolCardMode(%q) = %q, want %q", tc.mode, got, tc.want)
		}
	}
}

func TestApplyLegacyProgressMode(t *testing.T) {
	cases := []struct {
		progressMode string
		wantMode     string
		wantTeam     string
	}{
		{"text", ChannelIMRenderModeTranscript, ""},
		{"steps", ChannelIMRenderModeTranscript, ChannelIMTeamModeSteps},
		{"off", ChannelIMRenderModeReplyOnly, ""},
		{"", ChannelIMRenderModeReplyOnly, ""},
	}
	for _, tc := range cases {
		policy := &ChannelIMRenderPolicy{Mode: ChannelIMRenderModeReplyOnly}
		ltCfg := ChannelLongTaskConfig{ProgressMode: tc.progressMode}
		applyLegacyProgressMode(policy, ltCfg)
		if policy.Mode != tc.wantMode {
			t.Errorf("applyLegacyProgressMode(mode=%q) Mode=%q, want %q", tc.progressMode, policy.Mode, tc.wantMode)
		}
		if policy.TeamMode != tc.wantTeam {
			t.Errorf("applyLegacyProgressMode(mode=%q) TeamMode=%q, want %q", tc.progressMode, policy.TeamMode, tc.wantTeam)
		}
	}
}

func TestApplyLegacyProgressMode_NilPolicy(t *testing.T) {
	ltCfg := ChannelLongTaskConfig{ProgressMode: "text"}
	applyLegacyProgressMode(nil, ltCfg)
}

func TestNormalizeOrigin(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"https://admin.test/", "https://admin.test"},
		{"https://admin.test", "https://admin.test"},
		{"  https://admin.test/  ", "https://admin.test"},
		{"", ""},
		{"  ", ""},
	}
	for _, tc := range cases {
		got := normalizeOrigin(tc.raw)
		if got != tc.want {
			t.Errorf("normalizeOrigin(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestIsChannelCancelCommand(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"取消", true},
		{"停止", true},
		{"cancel", true},
		{"stop", true},
		{"/cancel", true},
		{"/stop", true},
		{"CANCEL", true},
		{" 取消 ", true},
		{"取消/123", true},
		{"取消 something", true},
		{"你好", false},
		{"", false},
		{"   ", false},
		{"取消后", false},
	}
	for _, tc := range cases {
		got := IsChannelCancelCommand(tc.text)
		if got != tc.want {
			t.Errorf("IsChannelCancelCommand(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

func TestIsChannelBackgroundCommand(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"/background", true},
		{"background", true},
		{"后台", true},
		{"后台继续", true},
		{"BACKGROUND", true},
		{" /background ", true},
		{"你好", false},
		{"", false},
		{"background more", false},
	}
	for _, tc := range cases {
		got := IsChannelBackgroundCommand(tc.text)
		if got != tc.want {
			t.Errorf("IsChannelBackgroundCommand(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}
