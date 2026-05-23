package service

import (
	"context"
	"errors"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
)

const (
	channelTurnErrorTimeoutMsg   = "任务超时，请稍后重试或联系管理员。"
	channelTurnErrorSyncCapMsg   = "任务执行较慢，建议使用 /background 转入后台继续。"
	channelTurnErrorGenericMsg   = "任务执行失败，请稍后重试。"
	channelPreviewThinkingHint   = "正在思考与执行工具，请稍候…"
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
	switch TurnErrorCodeFromErr(err) {
	case TurnErrTurnTimeout, TurnErrFirstByteTimeout:
		return channelTurnErrorSyncCapMsg
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

func (h *ChannelIngress) shouldSkipTurnErrorReply(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform string, execErr error) bool {
	if h == nil || h.chat == nil || execErr == nil {
		return false
	}
	if TurnErrorCodeFromErr(execErr) != TurnErrTurnTimeout && TurnErrorCodeFromErr(execErr) != TurnErrFirstByteTimeout && !turnErrorIsTimeout(execErr) {
		return false
	}
	sessionID := h.resolveInboundSessionID(ctx, chRow, ev, platform)
	if sessionID == "" {
		return false
	}
	phase := h.chat.ActiveSessionRunPhase(ctx, sessionID)
	switch phase {
	case biz.SessionRunPhaseDurable, biz.SessionRunPhaseEscalating:
		return true
	default:
		return false
	}
}

func (h *ChannelIngress) resolveInboundSessionID(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform string) string {
	if sid := strings.TrimSpace(ev.OutboundMeta["session_id"]); sid != "" {
		return sid
	}
	routing, err := biz.ParseChannelRouting(chRow.ConfigJSON)
	if err != nil {
		return ""
	}
	peerKey := ev.PeerKey
	if peerKey == "" {
		peerKey = biz.PeerKeyForSession(routing.DMScope, ev.PeerID)
	}
	req, err := h.prepareChannelChatRequest(ctx, chRow, platform, peerKey, ev.PeerID, ev.Text)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(req.GetSessionId())
}
