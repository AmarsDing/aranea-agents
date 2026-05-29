package service

import (
	"context"
	"errors"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
)

// deliverTurnErrorReply enqueues a user-visible IM error for failed Channel turns (LT-06).
func (h *ChannelIngress) deliverTurnErrorReply(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform string, execErr error) error {
	if h == nil || execErr == nil {
		return nil
	}
	if h.shouldSkipTurnErrorReply(ctx, chRow, ev, platform, execErr) {
		return nil
	}
	text := formatChannelTurnErrorMessage(execErr)
	if strings.TrimSpace(text) == "" {
		return nil
	}
	idempotency := ackIdempotencyKey(platform, ev, "error")
	if err := h.enqueueOutboundReply(ctx, chRow, platform, outboundRecipient(ev), text, ev.OutboundMeta, idempotency); err != nil {
		h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "turn_error_reply", "error": err.Error()}, err.Error())
		return err
	}
	h.recordDelivery(ctx, chRow.ID, "turn_error", map[string]any{
		"peer_id": ev.PeerID,
		"cause":   truncateForLog(execErr.Error(), 200),
	}, execErr.Error())
	return nil
}

func formatChannelTurnErrorMessage(err error) string {
	if err == nil || turnErrorIsCanceled(err) {
		return ""
	}
	switch classifyChannelTurnError(err) {
	case channelTurnErrBusy:
		return channelTurnErrorBusyMsg
	case channelTurnErrTimeout:
		return channelTurnErrorSyncCapMsg
	case channelTurnErrRateLimit:
		return channelTurnErrorRateLimitMsg
	case channelTurnErrContextOverflow:
		return channelTurnErrorContextOverflow
	default:
		return channelTurnErrorGenericMsg
	}
}

func turnErrorIsCanceled(err error) bool {
	return err != nil && errors.Is(err, context.Canceled)
}

func turnErrorIsTimeout(err error) bool {
	if err == nil || turnErrorIsCanceled(err) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded")
}
