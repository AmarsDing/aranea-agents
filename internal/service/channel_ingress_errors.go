package service

import (
	"context"
	"errors"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
)

// errContextPressureRejected signals that a turn was rejected due to session
// context pressure and the user-visible error reply was already sent by
// rejectIfContextPressure. The defer in executeInboundTurn uses this sentinel
// to mark the job as failed without sending a duplicate error reply.
var errContextPressureRejected = errors.New("turn rejected: session context pressure")

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
	if err == nil || turnErrorIsCanceled(err) || errors.Is(err, errContextPressureRejected) {
		return ""
	}
	return biz.FormatChannelTurnErrorMessage(classifyChannelTurnError(err))
}

func turnErrorIsCanceled(err error) bool {
	return err != nil && errors.Is(err, context.Canceled)
}

func turnErrorIsTimeout(err error) bool {
	if err == nil || turnErrorIsCanceled(err) {
		return false
	}
	return errors.Is(err, context.DeadlineExceeded)
}
