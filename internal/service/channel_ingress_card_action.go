package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/lark"
	"aranea-agents/internal/event"
)

const flowStepChannelCardAction = "channel.card.action"

// HandleFeishuCardAction processes card.action.trigger for session run escalation (CC-R-02).
func (h *ChannelIngress) HandleFeishuCardAction(ctx context.Context, chRow biz.Channel, action lark.CardActionPayload) *lark.CardActionHTTPResponse {
	if h == nil || h.chat == nil {
		return lark.NewCardActionToast("服务未就绪")
	}
	action.Action = strings.TrimSpace(strings.ToLower(action.Action))
	switch action.Action {
	case lark.CardActionBackground:
		return h.handleFeishuCardBackground(ctx, chRow, action)
	case lark.CardActionCancel:
		return h.handleFeishuCardCancel(ctx, chRow, action)
	default:
		return lark.NewCardActionToast("未知操作")
	}
}

func (h *ChannelIngress) handleFeishuCardBackground(ctx context.Context, chRow biz.Channel, action lark.CardActionPayload) *lark.CardActionHTTPResponse {
	sessionRunID := strings.TrimSpace(action.SessionRunID)
	if sessionRunID == "" {
		return lark.NewCardActionToast(channelBackgroundReplyNoActiveRun)
	}
	sessionID, ok := h.resolveCardActionSessionID(ctx, chRow, action)
	if !ok {
		return lark.NewCardActionToast(channelBackgroundReplyDenied)
	}
	reply, err := h.chat.EscalateSessionRun(ctx, sessionRunID, sessionID)
	if err != nil {
		event.SysLogWarn(flowStepChannelCardAction, "飞书卡片回调失败",
			event.P("channel_id", chRow.ID),
			event.P("session_run_id", sessionRunID),
			event.P("error", err.Error()),
		)
		return lark.NewCardActionToast("操作失败，请稍后重试")
	}
	event.SysLogInfo(flowStepChannelCardAction, "飞书卡片后台继续",
		event.P("channel_id", chRow.ID),
		event.P("session_run_id", sessionRunID),
		event.P("session_id", sessionID),
		event.P("operator_open_id", action.OperatorOpenID),
	)
	_ = h.recordDelivery(ctx, chRow.ID, "card_action", map[string]any{
		"action":         action.Action,
		"session_run_id": sessionRunID,
		"session_id":     sessionID,
		"operator":       action.OperatorOpenID,
		"reply":          reply,
	}, "")
	return lark.NewCardActionToast(reply)
}

func (h *ChannelIngress) handleFeishuCardCancel(ctx context.Context, chRow biz.Channel, action lark.CardActionPayload) *lark.CardActionHTTPResponse {
	sessionID, ok := h.resolveCardActionSessionID(ctx, chRow, action)
	if !ok {
		return lark.NewCardActionToast(channelBackgroundReplyDenied)
	}
	sessionRunID := strings.TrimSpace(action.SessionRunID)
	cancelled, reply := h.chat.CancelSessionRunForCard(ctx, sessionRunID, sessionID)
	if !cancelled {
		return lark.NewCardActionToast(reply)
	}
	event.SysLogInfo(flowStepChannelCardAction, "飞书卡片取消执行",
		event.P("channel_id", chRow.ID),
		event.P("session_id", sessionID),
		event.P("session_run_id", sessionRunID),
		event.P("operator_open_id", action.OperatorOpenID),
	)
	_ = h.recordDelivery(ctx, chRow.ID, "card_action", map[string]any{
		"action":         action.Action,
		"session_id":     sessionID,
		"session_run_id": sessionRunID,
		"operator":       action.OperatorOpenID,
	}, "")
	return lark.NewCardActionToast(reply)
}

func (h *ChannelIngress) resolveCardActionSessionID(ctx context.Context, chRow biz.Channel, action lark.CardActionPayload) (string, bool) {
	if h == nil {
		return "", false
	}
	peerSession, ok := h.resolvePeerSessionID(ctx, chRow, action)
	if !ok {
		return "", false
	}
	if cardSID := strings.TrimSpace(action.SessionID); cardSID != "" && cardSID != peerSession {
		return "", false
	}
	return peerSession, true
}

func (h *ChannelIngress) resolvePeerSessionID(ctx context.Context, chRow biz.Channel, action lark.CardActionPayload) (string, bool) {
	peerID := ingressFirstNonEmpty(action.OpenChatID, action.OperatorOpenID)
	if peerID == "" {
		return "", false
	}
	routing, err := biz.ParseChannelRouting(chRow.ConfigJSON)
	if err != nil {
		return "", false
	}
	peerKey := biz.PeerKeyForSession(routing.DMScope, peerID)
	platform := channelTypeFromConfig(chRow.ConfigJSON)
	if platform == "" {
		platform = "feishu"
	}
	req, err := h.prepareChannelChatRequest(ctx, chRow, platform, peerKey, peerID, "")
	if err != nil {
		return "", false
	}
	sessionID := strings.TrimSpace(req.GetSessionId())
	return sessionID, sessionID != ""
}
