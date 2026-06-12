package biz

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// ChannelInboundReceiptRepo records processed inbound events (idempotency).
// Stability:stable
type ChannelInboundReceiptRepo interface {
	// TryClaim inserts a receipt; returns claimed=false when the key already exists.
	TryClaim(ctx context.Context, channelID, idempotencyKey, peerID, textPreview string) (claimed bool, err error)
}

// InboundIdempotencyKey returns the deduplication key for an inbound message.
// It includes the platform prefix to avoid cross-platform key collisions
// (e.g. feishu and dingtalk both using numeric message IDs).
// Empty when the adapter did not provide a stable id — ingress must reject before Turn.
func InboundIdempotencyKey(platform, messageKey string) string {
	messageKey = strings.TrimSpace(messageKey)
	if messageKey == "" {
		return ""
	}
	platform = strings.TrimSpace(platform)
	if platform != "" && !strings.Contains(messageKey, ":") {
		return platform + ":" + messageKey
	}
	return messageKey
}

func inboundTextPreview(text string) string {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) <= 120 {
		return text
	}
	return string(runes[:120]) + "…"
}

// TryClaimInbound records idempotency before running an agent turn.
func TryClaimInbound(ctx context.Context, repo ChannelInboundReceiptRepo, channelID, platform, messageKey, peerID, text string) (bool, error) {
	if repo == nil {
		return true, nil
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return true, nil
	}
	key := InboundIdempotencyKey(platform, messageKey)
	return repo.TryClaim(ctx, channelID, key, peerID, inboundTextPreview(text))
}

// NewInboundReceiptID generates a receipt row id.
func NewInboundReceiptID() string {
	return uuid.NewString()
}
