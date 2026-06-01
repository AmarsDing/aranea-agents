package service_test

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/service"
	"aranea-agents/pkg/loggateway"
)

func TestTrimKey(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"spaces_only", "   ", ""},
		{"tabs_only", "\t\t", ""},
		{"leading_spaces", "  hello", "hello"},
		{"trailing_spaces", "hello  ", "hello"},
		{"both_sides", "  hello  ", "hello"},
		{"mixed_whitespace", "\t hello \t", "hello"},
		{"no_trim_needed", "hello", "hello"},
		{"internal_spaces", "hello world", "hello world"},
		{"internal_tab", "hello\tworld", "hello\tworld"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := service.TrimKey(tt.in); got != tt.want {
				t.Errorf("TrimKey(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestAllowWebhookRequest(t *testing.T) {
	tests := []struct {
		name       string
		channelKey string
		want       bool
	}{
		{"empty_key_allowed", "", true},
		{"whitespace_key_allowed", "   ", true},
		{"valid_key_first_request", "test-channel-allow-1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := service.AllowWebhookRequest(tt.channelKey, loggateway.NewNoop()); got != tt.want {
				t.Errorf("AllowWebhookRequest(%q) = %v, want %v", tt.channelKey, got, tt.want)
			}
		})
	}
}

func TestMetaBool(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{"empty", "", false},
		{"spaces", "   ", false},
		{"one", "1", true},
		{"true", "true", true},
		{"TRUE", "TRUE", true},
		{"True", "True", true},
		{"yes", "yes", true},
		{"YES", "YES", true},
		{"on", "on", true},
		{"ON", "ON", true},
		{"zero", "0", false},
		{"false", "false", false},
		{"no", "no", false},
		{"off", "off", false},
		{"random", "maybe", false},
		{"true_with_spaces", " true ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := service.MetaBool(tt.raw); got != tt.want {
				t.Errorf("MetaBool(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestUniqueNonEmptyStrings(t *testing.T) {
	tests := []struct {
		name  string
		parts []string
		want  []string
	}{
		{"nil_input", nil, nil},
		{"empty_input", []string{}, nil},
		{"all_empty", []string{"", " ", "  "}, nil},
		{"single", []string{"a"}, []string{"a"}},
		{"duplicates", []string{"a", "a", "a"}, []string{"a"}},
		{"order_preserved", []string{"c", "b", "a"}, []string{"c", "b", "a"}},
		{"mixed_with_empty", []string{"a", "", "b", " ", "c"}, []string{"a", "b", "c"}},
		{"trimmed_spaces", []string{" a ", "b", " a "}, []string{"a", "b"}},
		{"all_same_after_trim", []string{" x", "x ", "x"}, []string{"x"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.UniqueNonEmptyStrings(tt.parts...)
			if len(got) != len(tt.want) {
				t.Errorf("UniqueNonEmptyStrings() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("UniqueNonEmptyStrings()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestInboundAccessContextFromEvent(t *testing.T) {
	tests := []struct {
		name string
		ev   port.InboundEvent
		want biz.InboundAccessContext
	}{
		{
			name: "empty_event",
			ev:   port.InboundEvent{},
			want: biz.InboundAccessContext{},
		},
		{
			name: "nil_outbound_meta",
			ev:   port.InboundEvent{PeerID: "user1", OutboundMeta: nil},
			want: biz.InboundAccessContext{UserIDs: []string{"user1"}},
		},
		{
			name: "dm_with_user_ids",
			ev: port.InboundEvent{
				PeerID:      "user1",
				OutboundMeta: map[string]string{"sender_open_id": "ou_123", "sender_user_id": "uid_456"},
			},
			want: biz.InboundAccessContext{
				UserIDs: []string{"user1", "ou_123", "uid_456"},
			},
		},
		{
			name: "group_chat",
			ev: port.InboundEvent{
				PeerID:      "peer1",
				OutboundMeta: map[string]string{"chat_id": "oc_abc", "chat_type": "group", "mentioned": "true"},
			},
			want: biz.InboundAccessContext{
				UserIDs:   []string{"peer1"},
				GroupID:   "oc_abc",
				IsGroup:   true,
				Mentioned: true,
			},
		},
		{
			name: "group_fallback_peer_id",
			ev: port.InboundEvent{
				PeerID:      "peer1",
				OutboundMeta: map[string]string{"chat_type": "group"},
			},
			want: biz.InboundAccessContext{
				UserIDs: []string{"peer1"},
				GroupID: "peer1",
				IsGroup: true,
			},
		},
		{
			name: "group_conversation_type",
			ev: port.InboundEvent{
				PeerID:      "peer1",
				OutboundMeta: map[string]string{"conversation_type": "GROUP", "chat_id": "oc_xyz"},
			},
			want: biz.InboundAccessContext{
				UserIDs: []string{"peer1"},
				GroupID: "oc_xyz",
				IsGroup: true,
			},
		},
		{
			name: "mentioned_via_mentions_field",
			ev: port.InboundEvent{
				PeerID:      "peer1",
				OutboundMeta: map[string]string{"chat_type": "group", "mentions": "@bot"},
			},
			want: biz.InboundAccessContext{
				UserIDs:   []string{"peer1"},
				GroupID:   "peer1",
				IsGroup:   true,
				Mentioned: true,
			},
		},
		{
			name: "deduped_user_ids",
			ev: port.InboundEvent{
				PeerID:      "same_id",
				OutboundMeta: map[string]string{"sender_open_id": "same_id", "user_id": "same_id"},
			},
			want: biz.InboundAccessContext{
				UserIDs: []string{"same_id"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.InboundAccessContextFromEvent(tt.ev)
			if got.GroupID != tt.want.GroupID {
				t.Errorf("GroupID = %q, want %q", got.GroupID, tt.want.GroupID)
			}
			if got.IsGroup != tt.want.IsGroup {
				t.Errorf("IsGroup = %v, want %v", got.IsGroup, tt.want.IsGroup)
			}
			if got.Mentioned != tt.want.Mentioned {
				t.Errorf("Mentioned = %v, want %v", got.Mentioned, tt.want.Mentioned)
			}
			if len(got.UserIDs) != len(tt.want.UserIDs) {
				t.Errorf("UserIDs = %v, want %v", got.UserIDs, tt.want.UserIDs)
				return
			}
			for i := range got.UserIDs {
				if got.UserIDs[i] != tt.want.UserIDs[i] {
					t.Errorf("UserIDs[%d] = %q, want %q", i, got.UserIDs[i], tt.want.UserIDs[i])
				}
			}
		})
	}
}

func TestWithChannelTurnJob(t *testing.T) {
	tests := []struct {
		name      string
		jobID     string
		sessionID string
		wantJob   string
		wantSess  string
	}{
		{"both_set", "job-1", "sess-1", "job-1", "sess-1"},
		{"job_only", "job-2", "", "job-2", ""},
		{"session_only", "", "sess-3", "", "sess-3"},
		{"both_empty_returns_original_ctx", "", "", "", ""},
		{"whitespace_trimmed", " job-4 ", " sess-4 ", "job-4", "sess-4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			newCtx := service.WithChannelTurnJob(ctx, tt.jobID, tt.sessionID)
			jobID, sessionID := service.ChannelTurnJobFromContext(newCtx)
			if jobID != tt.wantJob {
				t.Errorf("jobID = %q, want %q", jobID, tt.wantJob)
			}
			if sessionID != tt.wantSess {
				t.Errorf("sessionID = %q, want %q", sessionID, tt.wantSess)
			}
		})
	}
}

func TestChannelTurnJobFromContext_NilContext(t *testing.T) {
	jobID, sessionID := service.ChannelTurnJobFromContext(nil)
	if jobID != "" {
		t.Errorf("jobID = %q, want empty", jobID)
	}
	if sessionID != "" {
		t.Errorf("sessionID = %q, want empty", sessionID)
	}
}

func TestWithChannelTurnJobID(t *testing.T) {
	ctx := service.WithChannelTurnJob(context.Background(), "", "sess-old")
	newCtx := service.WithChannelTurnJobID(ctx, "job-new")
	jobID, sessionID := service.ChannelTurnJobFromContext(newCtx)
	if jobID != "job-new" {
		t.Errorf("jobID = %q, want %q", jobID, "job-new")
	}
	if sessionID != "sess-old" {
		t.Errorf("sessionID = %q, want %q", sessionID, "sess-old")
	}
}

func TestChannelTurnJobIDFromContext(t *testing.T) {
	ctx := service.WithChannelTurnJob(context.Background(), "job-42", "sess-42")
	if got := service.ChannelTurnJobIDFromContext(ctx); got != "job-42" {
		t.Errorf("ChannelTurnJobIDFromContext() = %q, want %q", got, "job-42")
	}
}

func TestStreamPlatformSupported(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		want     bool
	}{
		{"feishu_supported", "feishu", true},
		{"FEISHU_case_insensitive", "FEISHU", true},
		{"slack_supported", "slack", true},
		{"telegram_supported", "telegram", true},
		{"line_supported", "line", true},
		{"mattermost_supported", "mattermost", true},
		{"dingtalk_no_stream", "dingtalk", false},
		{"wecom_no_stream", "wecom", false},
		{"discord_no_stream", "discord", false},
		{"unknown_platform", "unknown", false},
		{"empty_platform", "", false},
		{"spaced_feishu", " feishu ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := service.StreamPlatformSupported(tt.platform); got != tt.want {
				t.Errorf("StreamPlatformSupported(%q) = %v, want %v", tt.platform, got, tt.want)
			}
		})
	}
}

func TestTruncateForLog(t *testing.T) {
	tests := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"short_string", "hello", 10, "hello"},
		{"exact_length", "hello", 5, "hello"},
		{"truncated", "hello world", 5, "hello…"},
		{"zero_max", "hello", 0, "hello"},
		{"negative_max", "hello", -1, "hello"},
		{"empty_string", "", 5, ""},
		{"spaces_trimmed", "  hello  ", 5, "hello"},
		{"spaces_truncated", "  hello world  ", 5, "hello…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := service.TruncateForLog(tt.s, tt.max); got != tt.want {
				t.Errorf("TruncateForLog(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
			}
		})
	}
}

func TestOutboundRecipient(t *testing.T) {
	tests := []struct {
		name string
		ev   port.InboundEvent
		want string
	}{
		{
			name: "recipient_from_meta",
			ev:   port.InboundEvent{PeerID: "peer1", OutboundMeta: map[string]string{"recipient": "ou_target"}},
			want: "ou_target",
		},
		{
			name: "fallback_to_peer_id",
			ev:   port.InboundEvent{PeerID: "peer1", OutboundMeta: map[string]string{}},
			want: "peer1",
		},
		{
			name: "empty_recipient_fallback",
			ev:   port.InboundEvent{PeerID: "peer1", OutboundMeta: map[string]string{"recipient": ""}},
			want: "peer1",
		},
		{
			name: "spaces_recipient_fallback",
			ev:   port.InboundEvent{PeerID: "peer1", OutboundMeta: map[string]string{"recipient": "  "}},
			want: "peer1",
		},
		{
			name: "nil_meta_fallback",
			ev:   port.InboundEvent{PeerID: "peer1", OutboundMeta: nil},
			want: "peer1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := service.OutboundRecipient(tt.ev); got != tt.want {
				t.Errorf("OutboundRecipient() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAckIdempotencyKey(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		ev       port.InboundEvent
		suffix   string
		want     string
	}{
		{
			name:     "with_idempotency_key",
			platform: "feishu",
			ev:       port.InboundEvent{PeerID: "p1", IdempotencyKey: "key-abc"},
			suffix:   "ack",
			want:     "key-abc:ack",
		},
		{
			name:     "without_idempotency_key",
			platform: "feishu",
			ev:       port.InboundEvent{PeerID: "p1", IdempotencyKey: ""},
			suffix:   "ack",
			want:     "feishu:p1:ack",
		},
		{
			name:     "spaces_idempotency_key_fallback",
			platform: "slack",
			ev:       port.InboundEvent{PeerID: "p2", IdempotencyKey: "   "},
			suffix:   "error",
			want:     "slack:p2:error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := service.AckIdempotencyKey(tt.platform, tt.ev, tt.suffix); got != tt.want {
				t.Errorf("AckIdempotencyKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChannelTypeFromConfig(t *testing.T) {
	tests := []struct {
		name       string
		configJSON string
		want       string
	}{
		{"feishu", `{"type":"feishu"}`, "feishu"},
		{"uppercase", `{"type":"FEISHU"}`, "feishu"},
		{"with_spaces", `{"type":" slack "}`, "slack"},
		{"invalid_json", `{not json}`, ""},
		{"empty_json", `{}`, ""},
		{"empty_string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := service.ChannelTypeFromConfig(tt.configJSON, loggateway.NewNoop()); got != tt.want {
				t.Errorf("ChannelTypeFromConfig() = %q, want %q", got, tt.want)
			}
		})
	}
}
