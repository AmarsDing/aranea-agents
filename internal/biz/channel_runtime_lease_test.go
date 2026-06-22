package biz_test

import (
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

func TestChannelRuntimeLeaseKey(t *testing.T) {
	cases := []struct {
		name      string
		channelID string
		platform  string
		want      string
	}{
		{"normal", "ch1", "feishu", "channel-runtime:feishu:ch1"},
		{"platform lowercased", "ch1", "Feishu", "channel-runtime:feishu:ch1"},
		{"empty channel id", "", "feishu", ""},
		{"empty platform", "ch1", "", ""},
		{"both empty", "", "", ""},
		{"with spaces trimmed", "  ch1  ", "  feishu  ", "channel-runtime:feishu:ch1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.ChannelRuntimeLeaseKey(tc.channelID, tc.platform)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNewChannelRuntimeLease(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	ttl := 5 * time.Minute

	lease := biz.NewChannelRuntimeLease("ch1", "Feishu", "owner1", ttl, now)
	if lease.Key != "channel-runtime:feishu:ch1" {
		t.Fatalf("Key = %q, want %q", lease.Key, "channel-runtime:feishu:ch1")
	}
	if lease.ChannelID != "ch1" {
		t.Fatalf("ChannelID = %q, want %q", lease.ChannelID, "ch1")
	}
	if lease.Platform != "feishu" {
		t.Fatalf("Platform = %q, want %q", lease.Platform, "feishu")
	}
	if lease.OwnerID != "owner1" {
		t.Fatalf("OwnerID = %q, want %q", lease.OwnerID, "owner1")
	}
	if !lease.ExpiresAt.Equal(now.Add(ttl)) {
		t.Fatalf("ExpiresAt = %v, want %v", lease.ExpiresAt, now.Add(ttl))
	}
}

func TestNewChannelRuntimeLease_ZeroNow(t *testing.T) {
	ttl := 5 * time.Minute
	lease := biz.NewChannelRuntimeLease("ch1", "feishu", "owner1", ttl, time.Time{})
	if lease.ExpiresAt.IsZero() {
		t.Fatalf("ExpiresAt should not be zero when now is zero")
	}
}

func TestRuntimeLease_Valid(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name  string
		lease biz.RuntimeLease
		want  bool
	}{
		{
			name: "valid lease",
			lease: biz.RuntimeLease{
				Key:       "channel-runtime:feishu:ch1",
				ChannelID: "ch1",
				Platform:  "feishu",
				OwnerID:   "owner1",
				ExpiresAt: now.Add(5 * time.Minute),
			},
			want: true,
		},
		{
			name: "empty key",
			lease: biz.RuntimeLease{
				Key:       "",
				ChannelID: "ch1",
				Platform:  "feishu",
				OwnerID:   "owner1",
				ExpiresAt: now.Add(5 * time.Minute),
			},
			want: false,
		},
		{
			name: "empty channel id",
			lease: biz.RuntimeLease{
				Key:       "channel-runtime:feishu:ch1",
				ChannelID: "",
				Platform:  "feishu",
				OwnerID:   "owner1",
				ExpiresAt: now.Add(5 * time.Minute),
			},
			want: false,
		},
		{
			name: "empty platform",
			lease: biz.RuntimeLease{
				Key:       "channel-runtime:feishu:ch1",
				ChannelID: "ch1",
				Platform:  "",
				OwnerID:   "owner1",
				ExpiresAt: now.Add(5 * time.Minute),
			},
			want: false,
		},
		{
			name: "empty owner id",
			lease: biz.RuntimeLease{
				Key:       "channel-runtime:feishu:ch1",
				ChannelID: "ch1",
				Platform:  "feishu",
				OwnerID:   "",
				ExpiresAt: now.Add(5 * time.Minute),
			},
			want: false,
		},
		{
			name: "zero expires at",
			lease: biz.RuntimeLease{
				Key:       "channel-runtime:feishu:ch1",
				ChannelID: "ch1",
				Platform:  "feishu",
				OwnerID:   "owner1",
				ExpiresAt: time.Time{},
			},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.lease.Valid()
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
