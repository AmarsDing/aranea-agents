package biz_test

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestInboundIdempotencyKey(t *testing.T) {
	cases := []struct {
		name       string
		platform   string
		messageKey string
		peerID     string
		text       string
		want       string
	}{
		{"returns platform-prefixed key", "feishu", "om_123", "u1", "hello", "feishu:om_123"},
		{"trims spaces and prefixes", "feishu", "  om_456  ", "u1", "hello", "feishu:om_456"},
		{"empty message key", "feishu", "", "u1", "hello", ""},
		{"already prefixed key not double-prefixed", "feishu", "feishu:om_789", "u1", "hello", "feishu:om_789"},
		{"dingtalk prefixed key preserved", "dingtalk", "dingtalk:msg:abc", "u1", "hello", "dingtalk:msg:abc"},
		{"no platform returns raw key", "", "om_123", "u1", "hello", "om_123"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.InboundIdempotencyKey(tc.platform, tc.messageKey)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestInboundTextPreview(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"short text unchanged", "hello world", "hello world"},
		{"empty string", "", ""},
		{"whitespace only", "   ", ""},
		{"text at 120 chars", repeatStr("a", 120), repeatStr("a", 120)},
		{"text at 121 chars truncated", repeatStr("a", 121), repeatStr("a", 120) + "…"},
		{"long text truncated", repeatStr("x", 200), repeatStr("x", 120) + "…"},
		{"text with leading spaces trimmed", "  hello  ", "hello"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.InboundTextPreview(tc.text)
			if got != tc.want {
				t.Fatalf("got %q (len=%d), want %q (len=%d)", got, len(got), tc.want, len(tc.want))
			}
		})
	}
}

func repeatStr(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
