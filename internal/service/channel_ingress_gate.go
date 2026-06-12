package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
)

// gateInboundBeforeTurn applies idempotency and access control without ACK or turn execution.
// When proceed is false, denyReply may carry a user-visible message for sync-reply platforms.
func (h *ChannelIngress) gateInboundBeforeTurn(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, viaWebhook bool) (proceed bool, denyReply string, err error) {
	if h == nil {
		return false, "", nil
	}
	platform := inboundPlatform(chRow, ev, h.lg)
	dedupKey := biz.InboundIdempotencyKey(platform, ev.IdempotencyKey)

	ok, skipReason, err := h.shouldProcessInbound(ctx, chRow, ev, viaWebhook)
	if err != nil {
		h.deduplicator.ReleaseInflight(dedupKey)
		return false, "", err
	}
	if !ok {
		h.deduplicator.ReleaseInflight(dedupKey)
		h.recordDelivery(ctx, chRow.ID, "skipped_"+skipReason, map[string]any{
			"peer_id":         ev.PeerID,
			"idempotency_key": ev.IdempotencyKey,
			"text_preview":    truncateForLog(ev.Text, 80),
		}, "")
		return false, "", nil
	}
	h.logInboundAccepted(ctx, chRow, ev, "passive")

	allowed, reason, err := h.checkInboundAccess(ctx, chRow, ev)
	if err != nil {
		h.deduplicator.ReleaseInflight(dedupKey)
		h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "access", "error": err.Error()}, err.Error())
		return false, "", err
	}
	if !allowed {
		h.deduplicator.ReleaseInflight(dedupKey)
		h.recordDelivery(ctx, chRow.ID, "access_denied", map[string]any{
			"peer_id": ev.PeerID,
			"reason":  reason,
		}, reason)
		text := channelAccessDeniedDefault
		if strings.TrimSpace(reason) != "" {
			text = channelAccessDeniedWithReason + reason
		}
		return false, text, nil
	}
	return true, "", nil
}
