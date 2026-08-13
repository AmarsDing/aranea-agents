package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/preview"
)

// handleStreamQueuedOutcome 统一处理 turn 被排队（前面有活跃 run）的流式路径收尾：
// 发送 queued ACK + 记录 queued delivery，并标记 turnQueued。
func (h *ChannelIngress) handleStreamQueuedOutcome(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform string, ltCfg biz.ChannelLongTaskConfig, sessionID string, result biz.TurnResult, turnQueued *bool) error {
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

func (h *ChannelIngress) processInboundStreaming(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform string, ltCfg biz.ChannelLongTaskConfig, sessionID string, turnInput biz.TurnInput, contentPreview *string, previewMessageID *string, turnQueued *bool) error {
	meta := ev.OutboundMeta
	if meta == nil {
		meta = map[string]string{}
	}
	updater, err := h.newStreamUpdater(ctx, chRow, platform, meta)
	if err != nil {
		return err
	}
	if updater == nil {
		preview, msgID, queued, err := h.processInboundUnaryWithOutcome(ctx, chRow, ev, platform, ltCfg, sessionID, turnInput)
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
			return h.handleStreamQueuedOutcome(ctx, chRow, ev, platform, ltCfg, sessionID, result, turnQueued)
		}
		h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "stream", "error": err.Error()}, err.Error())
		return err
	}
	if result.Outcome == biz.TurnOutcomeQueued {
		return h.handleStreamQueuedOutcome(ctx, chRow, ev, platform, ltCfg, sessionID, result, turnQueued)
	}
	_ = interrupted // admission may have cancelled prior run; preview still valid

	// Priority: LLM actual reply > transcript render (transcript no longer
	// subscribes to EventBus after Blocker F Stage 1; it only holds the
	// initial ACK system message, not the assistant reply).
	rendered := strings.TrimSpace(result.AssistantMsg.ContentMarkdown)
	if rendered != "" {
		rendered = preview.FormatAssistantReplyForIM(platform, rendered)
	}
	if rendered == "" {
		rendered = strings.TrimSpace(previewCoord.RenderedText())
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
		*contentPreview = truncateForLog(rendered, 200)
	}
	if previewMessageID != nil {
		*previewMessageID = previewCoord.PreviewMessageID()
	}
	if err := previewCoord.FlushFinalText(ctx, rendered); err != nil {
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
