package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
)

// processInboundCore resolves routing, runs the agent turn, and returns the reply text.
func (h *ChannelIngress) processInboundCore(ctx context.Context, chRow biz.Channel, ev port.InboundEvent) (reply string, err error) {
	if h == nil || h.chat == nil || h.peers == nil || h.sessions == nil {
		return "", nil
	}
	platform := strings.TrimSpace(ev.PlatformType)
	if platform == "" {
		platform = channelTypeFromConfig(chRow.ConfigJSON)
	}
	routing, err := biz.ParseChannelRouting(chRow.ConfigJSON)
	if err != nil {
		_ = h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "routing", "error": err.Error()}, err.Error())
		return "", err
	}
	peerKey := ev.PeerKey
	if peerKey == "" {
		peerKey = biz.PeerKeyForSession(routing.DMScope, ev.PeerID)
	}
	reply, err = h.runChatTurn(ctx, chRow, platform, peerKey, ev.PeerID, ev.Text)
	if err != nil {
		_ = h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "chat", "error": err.Error()}, err.Error())
		return "", err
	}
	return reply, nil
}

// ProcessInbound runs agent turn + outbound enqueue for a normalized channel message.
func (h *ChannelIngress) ProcessInbound(ctx context.Context, chRow biz.Channel, ev port.InboundEvent) error {
	allowed, reason, err := h.checkInboundAccess(ctx, chRow, ev)
	if err != nil {
		_ = h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "access", "error": err.Error()}, err.Error())
		return err
	}
	if !allowed {
		return h.rejectInboundAccess(ctx, chRow, ev, reason)
	}
	if biz.ChannelStreamingEnabled(chRow.ConfigJSON) {
		return h.processInboundStreaming(ctx, chRow, ev)
	}
	return h.processInboundUnary(ctx, chRow, ev, "")
}

func (h *ChannelIngress) processInboundUnary(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platformHint string) error {
	reply, err := h.processInboundCore(ctx, chRow, ev)
	if err != nil {
		return err
	}
	platform := strings.TrimSpace(platformHint)
	if platform == "" {
		platform = strings.TrimSpace(ev.PlatformType)
	}
	if platform == "" {
		platform = channelTypeFromConfig(chRow.ConfigJSON)
	}
	recipient := strings.TrimSpace(ev.OutboundMeta["recipient"])
	if recipient == "" {
		recipient = ev.PeerID
	}
	idempotency := strings.TrimSpace(ev.IdempotencyKey)
	if idempotency == "" {
		idempotency = platform + ":" + ev.PeerID
	}
	if err := h.enqueueOutboundReply(ctx, chRow, platform, recipient, reply, ev.OutboundMeta, idempotency); err != nil {
		_ = h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "enqueue", "error": err.Error()}, err.Error())
		return err
	}
	_ = h.recordDelivery(ctx, chRow.ID, "queued", map[string]any{"peer_id": ev.PeerID}, "")
	return nil
}
