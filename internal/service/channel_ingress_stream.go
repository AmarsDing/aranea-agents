package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
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

	peerKey, err := h.inboundPeerKey(chRow, ev)
	if err != nil {
		return err
	}
	turnInput, err := h.prepareChannelChatRequest(ctx, chRow, platform, peerKey, ev.PeerID, ev.Text, channelAllowQueueFromConfig(chRow.ConfigJSON))
	if err != nil {
		return err
	}
	if sessionID == "" {
		sessionID = strings.TrimSpace(turnInput.SessionID)
	}
	interrupted := h.maybeInterruptActiveTurn(ctx, chRow, sessionID)

	recipient := outboundRecipient(ev)
	previewCoord, stopPreview := h.startTurnPreview(ctx, sessionID, platform, recipient, updater, chRow, ev, ltCfg)
	defer stopPreview()

	result, err := h.runNativeTurnWithBusyRetry(ctx, chRow, platform, turnInput)
	if err != nil {
		if isTurnMessageQueued(err) || result.Outcome == biz.TurnOutcomeQueued {
			pendingID := strings.TrimSpace(result.PendingID)
			if pendingID == "" && h.chat != nil {
				pendingID = h.chat.LastPendingMessageID(sessionID)
			}
			if err := h.sendInboundQueuedAck(ctx, chRow, ev, platform, ltCfg, pendingID); err != nil {
				h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "queued_ack", "error": err.Error()}, err.Error())
				return err
			}
			h.recordDelivery(ctx, chRow.ID, "queued", map[string]any{"peer_id": ev.PeerID, "pending_id": pendingID}, "")
			if turnQueued != nil {
				*turnQueued = true
			}
			return nil
		}
		h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "stream", "error": err.Error()}, err.Error())
		return err
	}
	if result.Outcome == biz.TurnOutcomeQueued {
		pendingID := strings.TrimSpace(result.PendingID)
		if pendingID == "" && h.chat != nil {
			pendingID = h.chat.LastPendingMessageID(sessionID)
		}
		if err := h.sendInboundQueuedAck(ctx, chRow, ev, platform, ltCfg, pendingID); err != nil {
			h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "queued_ack", "error": err.Error()}, err.Error())
			return err
		}
		h.recordDelivery(ctx, chRow.ID, "queued", map[string]any{"peer_id": ev.PeerID, "pending_id": pendingID}, "")
		if turnQueued != nil {
			*turnQueued = true
		}
		return nil
	}
	_ = interrupted // admission may have cancelled prior run; preview still valid

	rendered := strings.TrimSpace(previewCoord.RenderedText())
	if rendered == "" {
		rendered = strings.TrimSpace(result.AssistantMsg.ContentMarkdown)
	}
	if rendered == "" {
		if previewCoord.PreviewMessageID() != "" {
			if err := previewCoord.FlushFinalText(ctx, channelStreamEmptyPreviewMsg); err != nil {
				h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "stream_empty_fallback", "error": err.Error()}, err.Error())
				return err
			}
		} else if err := h.deliverUnaryReply(ctx, chRow, ev, platform, channelStreamEmptyPreviewMsg); err != nil {
			h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "stream_empty_fallback", "error": err.Error()}, err.Error())
			return err
		}
		h.recordDelivery(ctx, chRow.ID, "streamed", map[string]any{"peer_id": ev.PeerID, "platform": platform, "fallback": true}, "")
		return nil
	}
	if contentPreview != nil {
		*contentPreview = previewCoord.ContentPreview(200)
	}
	if previewMessageID != nil {
		*previewMessageID = previewCoord.PreviewMessageID()
	}
	if err := previewCoord.Flush(ctx, true); err != nil {
		recordStreamUpdate(platform, "flush", err)
		h.recordDelivery(ctx, chRow.ID, "error", map[string]any{
			"phase":    "stream_flush",
			"error":    err.Error(),
			"fallback": "outbox",
		}, err.Error())
		if fallbackErr := h.deliverStreamFlushFallback(ctx, chRow, ev, platform, rendered); fallbackErr != nil {
			return fallbackErr
		}
		h.recordDelivery(ctx, chRow.ID, "streamed", map[string]any{
			"peer_id":  ev.PeerID,
			"platform": platform,
			"fallback": "outbox",
		}, "")
		return nil
	}
	recordStreamUpdate(platform, "flush", nil)
	h.recordDelivery(ctx, chRow.ID, "streamed", map[string]any{"peer_id": ev.PeerID, "platform": platform}, "")
	return nil
}
