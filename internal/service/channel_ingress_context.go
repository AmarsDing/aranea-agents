package service

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/pkg/loggateway"
)

func (h *ChannelIngress) sessionContextPressure(ctx context.Context, sessionID string) bool {
	if h == nil || h.admission == nil || sessionID == "" {
		return false
	}
	sess, err := h.sessions.Get(ctx, sessionID)
	if err != nil {
		h.lg.Warn("session lookup failed in channel context pressure check, skipping",
			loggateway.Str("session_id", sessionID),
			loggateway.Err(err),
		)
		return false
	}
	result := h.admission.EvaluateContextPressure(ctx, sess, biz.EntryPointChannel)
	return result.Pressure
}

// rejectIfContextPressure blocks new channel turns when session context is near capacity (CH-BOR-11).
func (h *ChannelIngress) rejectIfContextPressure(
	ctx context.Context,
	chRow biz.Channel,
	ev port.InboundEvent,
	platform, sessionID string,
) (blocked bool, err error) {
	if !h.sessionContextPressure(ctx, sessionID) {
		return false, nil
	}
	recordIngressIntentMetric("context_pressure")
	idempotency := ackIdempotencyKey(platform, ev, "context_pressure")
	return true, h.enqueueOutboundReply(ctx, chRow, platform, outboundRecipient(ev), biz.ChannelTurnErrorContextOverflowMsg, ev.OutboundMeta, idempotency)
}
