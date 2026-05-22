package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
)

type streamPreviewUpdater interface {
	Update(ctx context.Context, recipient, text string, force bool) error
}

func (h *ChannelIngress) processInboundStreaming(ctx context.Context, chRow biz.Channel, ev port.InboundEvent) error {
	platform := strings.TrimSpace(ev.PlatformType)
	if platform == "" {
		platform = channelTypeFromConfig(chRow.ConfigJSON)
	}
	meta := ev.OutboundMeta
	if meta == nil {
		meta = map[string]string{}
	}
	updater, err := h.newStreamUpdater(ctx, chRow, platform, meta)
	if err != nil {
		return err
	}
	if updater == nil {
		return h.processInboundUnary(ctx, chRow, ev, platform)
	}

	recipient := strings.TrimSpace(meta["recipient"])
	if recipient == "" {
		recipient = ev.PeerID
	}
	var lastText string
	_, _, err = h.runChatTurnStreaming(ctx, chRow, platform, ev, func(accumulated string) error {
		lastText = accumulated
		uerr := updater.Update(ctx, recipient, accumulated, false)
		recordStreamUpdate(platform, "delta", uerr)
		return uerr
	})
	if err != nil {
		_ = h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "stream", "error": err.Error()}, err.Error())
		return err
	}
	if strings.TrimSpace(lastText) != "" {
		if err := updater.Update(ctx, recipient, lastText, true); err != nil {
			recordStreamUpdate(platform, "flush", err)
			_ = h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "stream_flush", "error": err.Error()}, err.Error())
			return err
		}
		recordStreamUpdate(platform, "flush", nil)
	}
	_ = h.recordDelivery(ctx, chRow.ID, "streamed", map[string]any{"peer_id": ev.PeerID, "platform": platform}, "")
	return nil
}

func (h *ChannelIngress) runChatTurnStreaming(
	ctx context.Context,
	chRow biz.Channel,
	platform string,
	ev port.InboundEvent,
	onDelta ChannelStreamCallback,
) (biz.ChatMessage, biz.ChatMessage, error) {
	if h == nil || h.chat == nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, nil
	}
	routing, err := biz.ParseChannelRouting(chRow.ConfigJSON)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	peerKey := ev.PeerKey
	if peerKey == "" {
		peerKey = biz.PeerKeyForSession(routing.DMScope, ev.PeerID)
	}
	req, err := h.prepareChannelChatRequest(ctx, chRow, platform, peerKey, ev.PeerID, ev.Text)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	return h.chat.RunNativeTurnStreaming(ctx, req, onDelta)
}
