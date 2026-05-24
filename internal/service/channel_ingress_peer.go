package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/lark"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/event"
)

const flowStepChannelReaction = "channel.feishu.reaction"

func (h *ChannelIngress) inboundPeerKey(chRow biz.Channel, ev port.InboundEvent) (string, error) {
	if k := strings.TrimSpace(ev.PeerKey); k != "" {
		return k, nil
	}
	routing, err := biz.ParseChannelRouting(chRow.ConfigJSON)
	if err != nil {
		return "", err
	}
	if channelTypeFromConfig(chRow.ConfigJSON) == "feishu" {
		fc := lark.ParseFeishuChannelConfig(chRow.ConfigJSON)
		threadID, chatID := "", ""
		if ev.OutboundMeta != nil {
			threadID = strings.TrimSpace(ev.OutboundMeta["thread_id"])
			chatID = strings.TrimSpace(ev.OutboundMeta["chat_id"])
		}
		if fc.ThreadSessionsPerUser && threadID != "" && chatID != "" {
			return chatID + ":" + threadID, nil
		}
	}
	return biz.PeerKeyForSession(routing.DMScope, ev.PeerID), nil
}

func (h *ChannelIngress) inboundPeerKeyForPeer(chRow biz.Channel, peerID string, meta map[string]string) (string, error) {
	return h.inboundPeerKey(chRow, port.InboundEvent{PeerID: peerID, OutboundMeta: meta})
}

// maybeInterruptActiveTurn cancels an in-flight run when busy_input_mode=interrupt.
// Returns true when a cancel was requested (caller must not treat session as queued-only).
func (h *ChannelIngress) maybeInterruptActiveTurn(ctx context.Context, chRow biz.Channel, sessionID string) bool {
	if h == nil || h.chat == nil || sessionID == "" {
		return false
	}
	if !biz.ChannelBusyInputInterrupt(chRow.ConfigJSON) {
		return false
	}
	if !h.chat.HasActiveRun(sessionID) {
		return false
	}
	h.chat.CancelRun(ctx, sessionID)
	return true
}

func (h *ChannelIngress) wasActiveBeforeTurn(ctx context.Context, chRow biz.Channel, sessionID string, interrupted bool) bool {
	if interrupted || h == nil || h.chat == nil || sessionID == "" {
		return false
	}
	return h.chat.HasActiveRun(sessionID)
}

func (h *ChannelIngress) applyFeishuOutboundMeta(chRow biz.Channel, extra map[string]string) map[string]string {
	if extra == nil {
		extra = map[string]string{}
	}
	if channelTypeFromConfig(chRow.ConfigJSON) != "feishu" {
		return extra
	}
	fc := lark.ParseFeishuChannelConfig(chRow.ConfigJSON)
	if fc.ReplyInThread && strings.TrimSpace(extra["thread_id"]) != "" {
		extra["reply_in_thread"] = "true"
	}
	return extra
}

func (h *ChannelIngress) startFeishuProcessingReaction(ctx context.Context, chRow biz.Channel, ev port.InboundEvent) func() {
	if channelTypeFromConfig(chRow.ConfigJSON) != "feishu" {
		return func() {}
	}
	fc := lark.ParseFeishuChannelConfig(chRow.ConfigJSON)
	if !fc.ProcessingReaction || ev.OutboundMeta == nil {
		return func() {}
	}
	msgID := strings.TrimSpace(ev.OutboundMeta["inbound_message_id"])
	if msgID == "" {
		return func() {}
	}
	creds, err := h.channels.ListCredentialsRaw(ctx, chRow.ID)
	if err != nil {
		return func() {}
	}
	region, appID, err := lark.AppAndRegionFromConfig(chRow.ConfigJSON)
	if err != nil {
		return func() {}
	}
	sec, err := resolveCredentialPlain(ctx, creds, "app_secret")
	if err != nil || strings.TrimSpace(sec) == "" {
		return func() {}
	}
	rc := &lark.ReactionController{
		Region:    region,
		AppID:     appID,
		AppSecret: strings.TrimSpace(sec),
		HTTP:      h.http,
	}
	reactionID, err := rc.Add(ctx, msgID)
	if err != nil {
		event.SysLogWarn(flowStepChannelReaction, "飞书处理中 Reaction 添加失败",
			event.P("channel_id", chRow.ID),
			event.P("message_id", msgID),
			event.P("error", err.Error()),
		)
		return func() {}
	}
	return func() {
		if err := rc.Remove(context.WithoutCancel(ctx), msgID, reactionID); err != nil {
			event.SysLogWarn(flowStepChannelReaction, "飞书处理中 Reaction 移除失败",
				event.P("channel_id", chRow.ID),
				event.P("message_id", msgID),
				event.P("reaction_id", reactionID),
				event.P("error", err.Error()),
			)
		}
	}
}
