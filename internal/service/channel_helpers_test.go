package service

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/pkg/loggateway"
)

func TestIngressFirstNonEmpty(t *testing.T) {
	cases := []struct {
		parts []string
		want  string
	}{
		{[]string{}, ""},
		{[]string{"", ""}, ""},
		{[]string{"hello", ""}, "hello"},
		{[]string{"", "world"}, "world"},
		{[]string{"a", "b", "c"}, "a"},
		{[]string{"", "", "c"}, "c"},
		{[]string{"  spaced  "}, "spaced"},
		{[]string{"  ", "x"}, "x"},
	}
	for _, tc := range cases {
		got := ingressFirstNonEmpty(tc.parts...)
		if got != tc.want {
			t.Errorf("ingressFirstNonEmpty(%v) = %q, want %q", tc.parts, got, tc.want)
		}
	}
}

func TestTruncateForLog(t *testing.T) {
	cases := []struct {
		s    string
		max  int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello…"},
		{"  hello  ", 5, "hello"},
		{"  hello world  ", 5, "hello…"},
		{"hi", 0, "hi"},
		{"abc", -1, "abc"},
		{"", 5, ""},
	}
	for _, tc := range cases {
		got := truncateForLog(tc.s, tc.max)
		if got != tc.want {
			t.Errorf("truncateForLog(%q, %d) = %q, want %q", tc.s, tc.max, got, tc.want)
		}
	}
}

func TestOutboundRecipient(t *testing.T) {
	cases := []struct {
		ev   port.InboundEvent
		want string
	}{
		{port.InboundEvent{PeerID: "p1", OutboundMeta: map[string]string{"recipient": "r1"}}, "r1"},
		{port.InboundEvent{PeerID: "p1", OutboundMeta: map[string]string{"recipient": "  "}}, "p1"},
		{port.InboundEvent{PeerID: "p1", OutboundMeta: nil}, "p1"},
		{port.InboundEvent{PeerID: "p1"}, "p1"},
	}
	for _, tc := range cases {
		got := outboundRecipient(tc.ev)
		if got != tc.want {
			t.Errorf("outboundRecipient(%+v) = %q, want %q", tc.ev, got, tc.want)
		}
	}
}

func TestInboundPlatform(t *testing.T) {
	cases := []struct {
		chRow biz.Channel
		ev    port.InboundEvent
		want  string
	}{
		{biz.Channel{ConfigJSON: `{"type":"dingtalk"}`}, port.InboundEvent{PlatformType: "feishu"}, "feishu"},
		{biz.Channel{ConfigJSON: `{"type":"dingtalk"}`}, port.InboundEvent{PlatformType: ""}, "dingtalk"},
		{biz.Channel{ConfigJSON: `{"type":"dingtalk"}`}, port.InboundEvent{PlatformType: "  "}, "dingtalk"},
	}
	for _, tc := range cases {
		got := inboundPlatform(tc.chRow, tc.ev, loggateway.NewNoop())
		if got != tc.want {
			t.Errorf("inboundPlatform() = %q, want %q", got, tc.want)
		}
	}
}

func TestChannelTypeFromConfig(t *testing.T) {
	cases := []struct {
		configJSON string
		want       string
	}{
		{`{"type":"Feishu"}`, "feishu"},
		{`{"type":"  DINGTALK  "}`, "dingtalk"},
		{`{}`, ""},
		{`invalid`, ""},
	}
	for _, tc := range cases {
		got := biz.ChannelTypeFromConfig(tc.configJSON)
		if got != tc.want {
			t.Errorf("ChannelTypeFromConfig(%q) = %q, want %q", tc.configJSON, got, tc.want)
		}
	}
}

func TestChannelReceiveModeFromConfig(t *testing.T) {
	cases := []struct {
		configJSON string
		want       string
	}{
		{`{"receive_mode":"WebSocket"}`, "websocket"},
		{`{"receive_mode":"  HTTP  "}`, "http"},
		{`{}`, ""},
	}
	for _, tc := range cases {
		got := biz.ChannelReceiveModeFromConfig(tc.configJSON)
		if got != tc.want {
			t.Errorf("ChannelReceiveModeFromConfig(%q) = %q, want %q", tc.configJSON, got, tc.want)
		}
	}
}

func TestTelegramChatRecipient(t *testing.T) {
	cases := []struct {
		chatID int64
		want   string
	}{
		{0, "0"},
		{-1001234567890, "-1001234567890"},
		{12345, "12345"},
	}
	for _, tc := range cases {
		got := telegramChatRecipient(tc.chatID)
		if got != tc.want {
			t.Errorf("telegramChatRecipient(%d) = %q, want %q", tc.chatID, got, tc.want)
		}
	}
}

func TestOneBotHTTPServer(t *testing.T) {
	cases := []struct {
		configJSON string
		want       string
	}{
		{`{"config":{"onebot_http_server":"http://localhost:8080"}}`, "http://localhost:8080"},
		{`{"config":{}}`, ""},
		{`{}`, ""},
	}
	for _, tc := range cases {
		got := oneBotHTTPServer(tc.configJSON, loggateway.NewNoop())
		if got != tc.want {
			t.Errorf("oneBotHTTPServer(%q) = %q, want %q", tc.configJSON, got, tc.want)
		}
	}
}

func TestQqAppID(t *testing.T) {
	cases := []struct {
		configJSON string
		want       string
	}{
		{`{"config":{"app_id":"12345"}}`, "12345"},
		{`{"config":{}}`, ""},
		{`{}`, ""},
		{`{"config":{"app_id":"  abc  "}}`, "abc"},
	}
	for _, tc := range cases {
		got := qqAppID(tc.configJSON, loggateway.NewNoop())
		if got != tc.want {
			t.Errorf("qqAppID(%q) = %q, want %q", tc.configJSON, got, tc.want)
		}
	}
}
