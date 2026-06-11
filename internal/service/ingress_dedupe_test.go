package service

import (
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

func TestIngressMessageDedupeKey(t *testing.T) {
	cases := []struct {
		channelID string
		messageID string
		want      string
	}{
		{"ch1", "msg1", "ch1:msg1"},
		{"", "msg1", ""},
		{"ch1", "", ""},
		{"", "", ""},
	}
	for _, tc := range cases {
		got := biz.IngressMessageDedupeKey(tc.channelID, tc.messageID)
		if got != tc.want {
			t.Errorf("IngressMessageDedupeKey(%q, %q) = %q, want %q", tc.channelID, tc.messageID, got, tc.want)
		}
	}
}

func TestIngressDebounceEnabled(t *testing.T) {
	cases := []struct {
		platform string
		want     bool
	}{
		{"feishu", false},
		{"lark", false},
		{"Feishu", false},
		{"LARK", false},
		{"  feishu  ", false},
		{"dingtalk", true},
		{"telegram", true},
		{"slack", true},
		{"", true},
	}
	for _, tc := range cases {
		got := biz.IngressDebounceEnabled(tc.platform)
		if got != tc.want {
			t.Errorf("IngressDebounceEnabled(%q) = %v, want %v", tc.platform, got, tc.want)
		}
	}
}

func TestShouldSkipRecentDuplicate(t *testing.T) {
	now := time.Now()
	if !biz.ShouldSkipRecentDuplicate(now.Add(-30*time.Second), time.Minute, now) {
		t.Fatal("expected skip within TTL")
	}
	if biz.ShouldSkipRecentDuplicate(now.Add(-2*time.Minute), time.Minute, now) {
		t.Fatal("expected allow after TTL")
	}
}

func TestMergeIngressIdempotencyKeys(t *testing.T) {
	got := biz.MergeIngressIdempotencyKeys([]string{"a", "b"})
	if got != "a+b" {
		t.Fatalf("got %q", got)
	}
}
