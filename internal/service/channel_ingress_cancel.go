package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/event"
)

const (
	flowStepChannelTurnCancel     = "channel.turn.cancel"
	channelCancelReplyOK          = "已取消当前任务。"
	channelCancelReplyNoActiveRun = "当前没有进行中的任务。"
)

// tryCancelInboundTurn handles IM cancel commands without starting a new Turn.
func (h *ChannelIngress) tryCancelInboundTurn(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform string) (handled bool, err error) {
	handled, reply, err := h.resolveCancelInboundTurn(ctx, chRow, ev, platform)
	if !handled {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	idempotency := ackIdempotencyKey(platform, ev, "cancel")
	if err := h.enqueueOutboundReply(ctx, chRow, platform, outboundRecipient(ev), reply, ev.OutboundMeta, idempotency); err != nil {
		return true, err
	}
	return true, nil
}

func (h *ChannelIngress) resolveCancelInboundTurn(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform string) (handled bool, reply string, err error) {
	if h == nil || !biz.IsChannelCancelCommand(ev.Text) {
		return false, "", nil
	}
	peerKey, err := h.inboundPeerKey(chRow, ev)
	if err != nil {
		return true, "", err
	}
	req, err := h.prepareChannelChatRequest(ctx, chRow, platform, peerKey, ev.PeerID, ev.Text)
	if err != nil {
		return true, "", err
	}
	sessionID := strings.TrimSpace(req.GetSessionId())
	reply = channelCancelReplyNoActiveRun
	cancelled := false
	if h.chat != nil && sessionID != "" {
		cancelled = h.chat.CancelRun(ctx, sessionID)
	}
	if cancelled {
		reply = channelCancelReplyOK
		if h.turnJobs != nil {
			_ = h.turnJobs.CancelRunningForSession(ctx, chRow.ID, sessionID)
		}
	}
	h.logTurnFlow(ctx, sessionID, flowStepChannelTurnCancel, "Channel 入站取消",
		nil,
		event.P("channel_id", chRow.ID),
		event.P("peer_id", ev.PeerID),
		event.P("cancelled", cancelled),
	)
	_ = h.recordDelivery(ctx, chRow.ID, "cancel", map[string]any{
		"peer_id":    ev.PeerID,
		"session_id": sessionID,
		"cancelled":  cancelled,
	}, "")
	return true, reply, nil
}
