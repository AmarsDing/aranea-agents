package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/event"
)

const (
	flowStepChannelTurnBackground     = "channel.turn.background"
	channelBackgroundReplyOK          = "已转入后台继续执行。"
	channelBackgroundReplyAlready     = "任务已在后台执行中。"
	channelBackgroundReplyNoActiveRun = "当前没有可转入后台的任务。"
)

// tryBackgroundInboundTurn handles IM /background without starting a new Turn (CC-R-02).
func (h *ChannelIngress) tryBackgroundInboundTurn(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform string) (handled bool, err error) {
	handled, reply, err := h.resolveBackgroundInboundTurn(ctx, chRow, ev, platform)
	if !handled {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	idempotency := ackIdempotencyKey(platform, ev, "background")
	if err := h.enqueueOutboundReply(ctx, chRow, platform, outboundRecipient(ev), reply, ev.OutboundMeta, idempotency); err != nil {
		return true, err
	}
	return true, nil
}

func (h *ChannelIngress) resolveBackgroundInboundTurn(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform string) (handled bool, reply string, err error) {
	if h == nil || !biz.IsChannelBackgroundCommand(ev.Text) {
		return false, "", nil
	}
	routing, err := biz.ParseChannelRouting(chRow.ConfigJSON)
	if err != nil {
		return true, "", err
	}
	peerKey := ev.PeerKey
	if peerKey == "" {
		peerKey = biz.PeerKeyForSession(routing.DMScope, ev.PeerID)
	}
	req, err := h.prepareChannelChatRequest(ctx, chRow, platform, peerKey, ev.PeerID, ev.Text)
	if err != nil {
		return true, "", err
	}
	sessionID := strings.TrimSpace(req.GetSessionId())
	reply = channelBackgroundReplyNoActiveRun
	escalated := false
	if h.chat != nil && sessionID != "" {
		escalated, reply, err = h.chat.EscalateActiveSessionRun(ctx, sessionID)
		if err != nil {
			return true, "", err
		}
	}
	h.logTurnFlow(ctx, sessionID, flowStepChannelTurnBackground, "Channel 入站后台继续",
		nil,
		event.P("channel_id", chRow.ID),
		event.P("peer_id", ev.PeerID),
		event.P("escalated", escalated),
	)
	_ = h.recordDelivery(ctx, chRow.ID, "background", map[string]any{
		"peer_id":    ev.PeerID,
		"session_id": sessionID,
		"escalated":  escalated,
	}, "")
	return true, reply, nil
}
