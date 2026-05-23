package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/event"
)

func (h *ChannelIngress) processInboundStreaming(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform string, ltCfg biz.ChannelLongTaskConfig, sessionID string, contentPreview *string, previewMessageID *string, turnQueued *bool) error {
	meta := ev.OutboundMeta
	if meta == nil {
		meta = map[string]string{}
	}
	updater, err := h.newStreamUpdater(ctx, chRow, platform, meta)
	if err != nil {
		return err
	}
	if updater == nil {
		preview, msgID, queued, err := h.processInboundUnaryWithOutcome(ctx, chRow, ev, platform, ltCfg, sessionID)
		if contentPreview != nil {
			*contentPreview = preview
		}
		if previewMessageID != nil {
			*previewMessageID = msgID
		}
		if turnQueued != nil {
			*turnQueued = queued
		}
		return err
	}

	routing, err := biz.ParseChannelRouting(chRow.ConfigJSON)
	if err != nil {
		return err
	}
	peerKey := ev.PeerKey
	if peerKey == "" {
		peerKey = biz.PeerKeyForSession(routing.DMScope, ev.PeerID)
	}
	req, err := h.prepareChannelChatRequest(ctx, chRow, platform, peerKey, ev.PeerID, ev.Text)
	if err != nil {
		return err
	}
	if sessionID == "" {
		sessionID = strings.TrimSpace(req.GetSessionId())
	}
	wasActive := h.chat != nil && h.chat.HasActiveRun(sessionID)

	recipient := outboundRecipient(ev)
	previewCoord, stopPreview := h.startTurnPreview(ctx, sessionID, platform, recipient, updater, chRow, ev, ltCfg)
	defer stopPreview()

	_, _, err = h.chat.RunNativeTurnUnary(event.WithChannelEnvelopeContext(ctx, platform, chRow.Key), req)
	if err != nil {
		_ = h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "stream", "error": err.Error()}, err.Error())
		return err
	}

	rendered := previewCoord.RenderedText()
	if wasActive && strings.TrimSpace(rendered) == "" {
		pendingID := ""
		if h.chat != nil {
			pendingID = h.chat.LastPendingMessageID(sessionID)
		}
		if err := h.sendInboundQueuedAck(ctx, chRow, ev, platform, ltCfg, pendingID); err != nil {
			_ = h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "queued_ack", "error": err.Error()}, err.Error())
			return err
		}
		_ = h.recordDelivery(ctx, chRow.ID, "queued", map[string]any{"peer_id": ev.PeerID, "pending_id": pendingID}, "")
		if turnQueued != nil {
			*turnQueued = true
		}
		return nil
	}

	if strings.TrimSpace(rendered) != "" {
		if contentPreview != nil {
			*contentPreview = previewCoord.ContentPreview(200)
		}
		if previewMessageID != nil {
			*previewMessageID = previewCoord.PreviewMessageID()
		}
		if err := previewCoord.Flush(ctx, true); err != nil {
			recordStreamUpdate(platform, "flush", err)
			_ = h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "stream_flush", "error": err.Error()}, err.Error())
			return err
		}
		recordStreamUpdate(platform, "flush", nil)
	}
	_ = h.recordDelivery(ctx, chRow.ID, "streamed", map[string]any{"peer_id": ev.PeerID, "platform": platform}, "")
	return nil
}
