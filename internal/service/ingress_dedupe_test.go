package service

import (
	"testing"
	"time"
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
		got := ingressMessageDedupeKey(tc.channelID, tc.messageID)
		if got != tc.want {
			t.Errorf("ingressMessageDedupeKey(%q, %q) = %q, want %q", tc.channelID, tc.messageID, got, tc.want)
		}
	}
}

func TestIngressMessageDedupe_Claim(t *testing.T) {
	d := newIngressMessageDedupe(5 * time.Minute)
	now := time.Now()

	if !d.claim("key1", now) {
		t.Fatal("first claim should succeed")
	}
	if d.claim("key1", now) {
		t.Fatal("duplicate claim should fail within TTL")
	}
	if !d.claim("key2", now) {
		t.Fatal("different key should succeed")
	}
}

func TestIngressMessageDedupe_ClaimExpired(t *testing.T) {
	d := newIngressMessageDedupe(1 * time.Second)
	past := time.Now().Add(-2 * time.Second)

	if !d.claim("key1", past) {
		t.Fatal("first claim should succeed")
	}
	now := time.Now()
	if !d.claim("key1", now) {
		t.Fatal("claim after TTL expiry should succeed")
	}
}

func TestIngressMessageDedupe_NilReceiver(t *testing.T) {
	var d *ingressMessageDedupe
	if !d.claim("key1", time.Now()) {
		t.Fatal("nil dedupe should always allow")
	}
}

func TestIngressMessageDedupe_EmptyKey(t *testing.T) {
	d := newIngressMessageDedupe(5 * time.Minute)
	if !d.claim("", time.Now()) {
		t.Fatal("empty key should always allow")
	}
}

func TestIngressMessageDedupe_ZeroTTL(t *testing.T) {
	d := newIngressMessageDedupe(0)
	if d.ttl != defaultMessageDedupeTTL {
		t.Fatalf("zero TTL should default to %v, got %v", defaultMessageDedupeTTL, d.ttl)
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
		got := ingressDebounceEnabled(tc.platform)
		if got != tc.want {
			t.Errorf("ingressDebounceEnabled(%q) = %v, want %v", tc.platform, got, tc.want)
		}
	}
}
