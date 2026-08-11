package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/lark"
	"aranea-agents/internal/channel/port"
	"aranea-agents/pkg/loggateway"
)

const flowStepChannelCardAction = "channel.card.action"

// HandleFeishuCardAction processes card.action.trigger for session run escalation (CC-R-02).
func (h *ChannelIngress) HandleFeishuCardAction(ctx context.Context, chRow biz.Channel, action lark.CardActionPayload) *lark.CardActionHTTPResponse {
	if h == nil || h.chat == nil {
		return lark.NewCardActionToast(channelCardActionServiceUnavailable)
	}
	action.Action = strings.TrimSpace(strings.ToLower(action.Action))
	switch action.Action {
	case lark.CardActionBackground:
		return h.handleFeishuCardBackground(ctx, chRow, action)
	case lark.CardActionCancel:
		return h.handleFeishuCardCancel(ctx, chRow, action)
	case lark.CardActionGateConfirm:
		return h.handleFeishuCardGateConfirm(ctx, chRow, action)
	case lark.CardActionGateClarify:
		return h.handleFeishuCardGateClarify(ctx, chRow, action)
	default:
		return lark.NewCardActionToast(channelCardActionUnknownOperation)
	}
}

// handleFeishuCardGateConfirm 处理工具确认卡片点击（gate_confirm）。
// 归属由 peer 绑定解析；状态机复用 RPC 同核（ConfirmToolGateForCard）；
// 结果卡 PATCH 由 ChannelGateCards 的 step 事件订阅驱动。
func (h *ChannelIngress) handleFeishuCardGateConfirm(ctx context.Context, chRow biz.Channel, action lark.CardActionPayload) *lark.CardActionHTTPResponse {
	sessionID, ok := h.resolveCardActionSessionID(ctx, chRow, action)
	if !ok {
		return lark.NewCardActionToast(channelBackgroundReplyDenied)
	}
	if h.gateCards == nil {
		return lark.NewCardActionToast(channelCardActionServiceUnavailable)
	}
	reply := h.gateCards.HandleConfirmClick(ctx, sessionID, action.StepID, action.Reply)
	h.lg.Info("飞书卡片工具确认",
		loggateway.StepID(flowStepChannelCardAction),
		loggateway.Str("channel_id", chRow.ID),
		loggateway.Str("session_id", sessionID),
		loggateway.Str("step_id", action.StepID),
		loggateway.Str("reply", action.Reply),
		loggateway.Str("operator_open_id", action.OperatorOpenID),
	)
	h.recordDelivery(ctx, chRow.ID, "card_action", map[string]any{
		"action":    action.Action,
		"session_id": sessionID,
		"step_id":   action.StepID,
		"reply":     action.Reply,
		"operator":  action.OperatorOpenID,
		"result":    reply,
	}, "")
	return lark.NewCardActionToast(reply)
}

// handleFeishuCardGateClarify 处理澄清卡片选项点击（gate_clarify）。
// 选择累积与自动提交由 ChannelGateCards 负责。
func (h *ChannelIngress) handleFeishuCardGateClarify(ctx context.Context, chRow biz.Channel, action lark.CardActionPayload) *lark.CardActionHTTPResponse {
	sessionID, ok := h.resolveCardActionSessionID(ctx, chRow, action)
	if !ok {
		return lark.NewCardActionToast(channelBackgroundReplyDenied)
	}
	if h.gateCards == nil {
		return lark.NewCardActionToast(channelCardActionServiceUnavailable)
	}
	reply := h.gateCards.SelectClarifyOption(ctx, sessionID, action.StepID, action.QuestionIndex, action.Option)
	h.lg.Info("飞书卡片澄清选择",
		loggateway.StepID(flowStepChannelCardAction),
		loggateway.Str("channel_id", chRow.ID),
		loggateway.Str("session_id", sessionID),
		loggateway.Str("step_id", action.StepID),
		loggateway.Int("question_index", action.QuestionIndex),
		loggateway.Str("operator_open_id", action.OperatorOpenID),
	)
	return lark.NewCardActionToast(reply)
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
		h.lg.Warn("飞书卡片回调失败",
			loggateway.StepID(flowStepChannelCardAction),
			loggateway.Str("channel_id", chRow.ID),
			loggateway.Str("session_run_id", sessionRunID),
			loggateway.Err(err),
		)
		return lark.NewCardActionToast(channelCardActionFailedRetry)
	}
	h.lg.Info("飞书卡片后台继续",
		loggateway.StepID(flowStepChannelCardAction),
		loggateway.Str("channel_id", chRow.ID),
		loggateway.Str("session_run_id", sessionRunID),
		loggateway.Str("session_id", sessionID),
		loggateway.Str("operator_open_id", action.OperatorOpenID),
	)
	h.recordDelivery(ctx, chRow.ID, "card_action", map[string]any{
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
	h.lg.Info("飞书卡片取消执行",
		loggateway.StepID(flowStepChannelCardAction),
		loggateway.Str("channel_id", chRow.ID),
		loggateway.Str("session_id", sessionID),
		loggateway.Str("session_run_id", sessionRunID),
		loggateway.Str("operator_open_id", action.OperatorOpenID),
	)
	h.recordDelivery(ctx, chRow.ID, "card_action", map[string]any{
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
	if h == nil || h.channels == nil {
		return "", false
	}
	// Align with message inbound: operator open_id before chat_id (DM peer bind uses ou_*, not oc_*).
	seen := map[string]struct{}{}
	for _, peerID := range []string{
		strings.TrimSpace(action.OperatorOpenID),
		strings.TrimSpace(action.OpenChatID),
	} {
		if peerID == "" {
			continue
		}
		if _, dup := seen[peerID]; dup {
			continue
		}
		seen[peerID] = struct{}{}
		peerKey, perr := h.inboundPeerKey(chRow, port.InboundEvent{PeerID: peerID})
		if perr != nil {
			continue
		}
		bind, err := h.channels.GetPeerSession(ctx, chRow.ID, peerKey)
		if err != nil || strings.TrimSpace(bind.SessionID) == "" {
			continue
		}
		sessionID := strings.TrimSpace(bind.SessionID)
		if _, verr := h.sessions.Get(ctx, sessionID); verr != nil {
			continue
		}
		return sessionID, true
	}
	return "", false
}
