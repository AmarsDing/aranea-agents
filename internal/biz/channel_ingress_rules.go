package biz

import (
	"strings"
	"time"
)

// IngressMessageDedupeKey constructs the dedup key for an inbound message.
// Returns empty string if either channelID or messageID is empty.
func IngressMessageDedupeKey(channelID, messageID string) string {
	if channelID == "" || messageID == "" {
		return ""
	}
	return channelID + ":" + messageID
}

// ShouldSkipRecentDuplicate returns true when the last-seen timestamp is within TTL.
func ShouldSkipRecentDuplicate(lastSeen time.Time, ttl time.Duration, now time.Time) bool {
	if lastSeen.IsZero() || ttl <= 0 {
		return false
	}
	return now.Sub(lastSeen) < ttl
}

// IngressDebounceEnabled returns whether debouncing is enabled for the given platform.
// Feishu/Lark disables debounce; all others enable it.
func IngressDebounceEnabled(platform string) bool {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "feishu", "lark":
		return false
	default:
		return true
	}
}

// MergeIngressIdempotencyKeys merges multiple idempotency keys into one.
// Empty/whitespace keys are ignored. Multiple keys are joined with "+".
func MergeIngressIdempotencyKeys(keys []string) string {
	var parts []string
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k != "" {
			parts = append(parts, k)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return strings.Join(parts, "+")
}

// InboundEventIsGroup determines if the outbound metadata indicates a group chat.
func InboundEventIsGroup(outboundMeta map[string]string) bool {
	if outboundMeta == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(outboundMeta["chat_type"])) {
	case "group", "supergroup":
		return true
	default:
		return strings.TrimSpace(outboundMeta["group_id"]) != ""
	}
}
