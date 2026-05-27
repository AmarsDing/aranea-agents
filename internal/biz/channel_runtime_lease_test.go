package biz

import (
	"testing"
	"time"
)

func TestChannelRuntimeLeaseKey(t *testing.T) {
	got := ChannelRuntimeLeaseKey(" ch-1 ", " Slack ")
	want := "channel-runtime:slack:ch-1"
	if got != want {
		t.Fatalf("ChannelRuntimeLeaseKey = %q, want %q", got, want)
	}
}

func TestNewChannelRuntimeLease(t *testing.T) {
	now := time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC)
	lease := NewChannelRuntimeLease("ch-1", "Telegram", "node-a", time.Minute, now)
	if !lease.Valid() {
		t.Fatalf("lease should be valid: %+v", lease)
	}
	if lease.Platform != "telegram" {
		t.Fatalf("Platform = %q, want telegram", lease.Platform)
	}
	if !lease.ExpiresAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("ExpiresAt = %v, want %v", lease.ExpiresAt, now.Add(time.Minute))
	}
}
