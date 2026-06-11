package service

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
)

func (h *ChannelIngress) sessionContextPressure(ctx context.Context, sessionID string, ltCfg biz.ChannelLongTaskConfig) bool {
	if h == nil || h.sessions == nil || sessionID == "" {
		return false
	}
	threshold := ltCfg.ContextAdmissionThreshold
	if threshold <= 0 {
		return false
	}
	sess, err := h.sessions.Get(ctx, sessionID)
	if err != nil {
		return false
	}
	return biz.ContextPressureActive(sess.ContextUsedRatio, threshold)
}

// rejectIfContextPressure blocks new channel turns when session context is near capacity (CH-BOR-11).
func (h *ChannelIngress) rejectIfContextPressure(
	ctx context.Context,
	chRow biz.Channel,
	ev port.InboundEvent,
	platform, sessionID string,
	ltCfg biz.ChannelLongTaskConfig,
) (blocked bool, err error) {
	if !h.sessionContextPressure(ctx, sessionID, ltCfg) {
		return false, nil
	}
	recordIngressIntentMetric("context_pressure")
	idempotency := ackIdempotencyKey(platform, ev, "context_pressure")
	return true, h.enqueueOutboundReply(ctx, chRow, platform, outboundRecipient(ev), biz.ChannelTurnErrorContextOverflowMsg, ev.OutboundMeta, idempotency)
}
