package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/event"
)

const flowStepChannelInbound = "channel.inbound.receive"

// shouldProcessInbound applies platform idempotency and receive-mode exclusivity before a Turn.
func (h *ChannelIngress) shouldProcessInbound(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, viaWebhook bool) (bool, string, error) {
	if h == nil {
		return false, "ingress not configured", nil
	}
	if viaWebhook && channelReceiveModeFromConfig(chRow.ConfigJSON) == "websocket" {
		return false, "webhook_disabled_ws_mode", nil
	}
	if strings.TrimSpace(ev.IdempotencyKey) == "" {
		return false, "missing_idempotency_key", nil
	}
	platform := strings.TrimSpace(ev.PlatformType)
	if platform == "" {
		platform = channelTypeFromConfig(chRow.ConfigJSON)
	}
	dedupKey := biz.InboundIdempotencyKey(platform, ev.IdempotencyKey, ev.PeerID, ev.Text)
	if !h.inboundInflight.tryAcquire(dedupKey) {
		return false, "duplicate_inflight", nil
	}
	claimed, err := biz.TryClaimInbound(ctx, h.inboundReceipts, chRow.ID, platform, ev.IdempotencyKey, ev.PeerID, ev.Text)
	if err != nil {
		return false, "", err
	}
	if !claimed {
		h.inboundInflight.release(dedupKey)
		return false, "duplicate_inbound", nil
	}
	return true, "", nil
}

func (h *ChannelIngress) logInboundAccepted(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, viaWebhook string) {
	source := strings.TrimSpace(ev.OutboundMeta["ingress_source"])
	if source == "" {
		source = viaWebhook
	}
	event.SysLogInfo(flowStepChannelInbound, "Channel 入站受理",
		event.P("channel_id", chRow.ID),
		event.P("channel_key", chRow.Key),
		event.P("platform", strings.TrimSpace(ev.PlatformType)),
		event.P("ingress_source", source),
		event.P("idempotency_key", ev.IdempotencyKey),
		event.P("peer_id", ev.PeerID),
		event.P("text_len", len(strings.TrimSpace(ev.Text))),
	)
}
