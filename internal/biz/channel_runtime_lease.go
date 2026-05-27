package biz

import (
	"context"
	"strings"
	"time"
)

// ChannelRuntimeLeaseRepo coordinates long-lived channel connectors across replicas.
type ChannelRuntimeLeaseRepo interface {
	// TryAcquireRuntimeLease claims a channel runtime lease until expiresAt.
	TryAcquireRuntimeLease(ctx context.Context, lease RuntimeLease) (claimed bool, err error)
	// RenewRuntimeLease extends a lease only when owner still holds it.
	RenewRuntimeLease(ctx context.Context, key, ownerID string, expiresAt time.Time) (renewed bool, err error)
	// ReleaseRuntimeLease releases a held lease.
	ReleaseRuntimeLease(ctx context.Context, key, ownerID string) error
}

// RuntimeLease identifies the single replica allowed to keep one platform connector open.
type RuntimeLease struct {
	Key       string
	ChannelID string
	Platform  string
	OwnerID   string
	ExpiresAt time.Time
}

// ChannelRuntimeLeaseKey is stable across replicas for the same channel/platform runtime.
func ChannelRuntimeLeaseKey(channelID, platform string) string {
	channelID = strings.TrimSpace(channelID)
	platform = strings.ToLower(strings.TrimSpace(platform))
	if channelID == "" || platform == "" {
		return ""
	}
	return "channel-runtime:" + platform + ":" + channelID
}

// NewChannelRuntimeLease builds the canonical lease record.
func NewChannelRuntimeLease(channelID, platform, ownerID string, ttl time.Duration, now time.Time) RuntimeLease {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return RuntimeLease{
		Key:       ChannelRuntimeLeaseKey(channelID, platform),
		ChannelID: strings.TrimSpace(channelID),
		Platform:  strings.ToLower(strings.TrimSpace(platform)),
		OwnerID:   strings.TrimSpace(ownerID),
		ExpiresAt: now.UTC().Add(ttl),
	}
}

// Valid reports whether the lease has enough information for a persistent compare-and-swap.
func (l RuntimeLease) Valid() bool {
	return strings.TrimSpace(l.Key) != "" &&
		strings.TrimSpace(l.ChannelID) != "" &&
		strings.TrimSpace(l.Platform) != "" &&
		strings.TrimSpace(l.OwnerID) != "" &&
		!l.ExpiresAt.IsZero()
}
