package service

import (
	"context"
	"errors"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
)

const (
	channelTurnErrorTimeoutMsg = "任务超时，请稍后重试或联系管理员。"
	channelTurnErrorGenericMsg = "任务执行失败，请稍后重试。"
)

// deliverTurnErrorReply enqueues a user-visible IM error for failed Channel turns (LT-06).
func (h *ChannelIngress) deliverTurnErrorReply(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform string, execErr error) error {
	if h == nil || execErr == nil {
		return nil
	}
	text := formatChannelTurnErrorMessage(execErr)
	if strings.TrimSpace(text) == "" {
		return nil
	}
	idempotency := ackIdempotencyKey(platform, ev, "error")
	if err := h.enqueueOutboundReply(ctx, chRow, platform, outboundRecipient(ev), text, ev.OutboundMeta, idempotency); err != nil {
		_ = h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "turn_error_reply", "error": err.Error()}, err.Error())
		return err
	}
	_ = h.recordDelivery(ctx, chRow.ID, "turn_error", map[string]any{
		"peer_id": ev.PeerID,
		"cause":   truncateForLog(execErr.Error(), 200),
	}, execErr.Error())
	return nil
}

func formatChannelTurnErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	if turnErrorIsTimeout(err) {
		return channelTurnErrorTimeoutMsg
	}
	return channelTurnErrorGenericMsg
}

func turnErrorIsTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded")
}
