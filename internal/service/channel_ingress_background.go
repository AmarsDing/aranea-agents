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
	flowStepRunEscalate               = "run.escalate.durable"
	channelBackgroundReplyOK          = "已转入后台继续执行。"
	channelBackgroundReplyAlready     = "任务已在后台执行中。"
	channelBackgroundReplyNoActiveRun = "当前没有可转入后台的任务。"
	channelBackgroundReplyDenied      = "无权操作该任务。"
)

func firstNonEmptyString(parts ...string) string {
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			return v
		}
	}
	return ""
}

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
	peerKey, err := h.inboundPeerKey(chRow, ev)
	if err != nil {
		return true, "", err
	}
	input, err := h.prepareChannelChatRequest(ctx, chRow, platform, peerKey, ev.PeerID, ev.Text, false)
	if err != nil {
		return true, "", err
	}
	sessionID := strings.TrimSpace(input.SessionID)
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
	h.recordDelivery(ctx, chRow.ID, "background", map[string]any{
		"peer_id":    ev.PeerID,
		"session_id": sessionID,
		"escalated":  escalated,
	}, "")
	return true, reply, nil
}

// EscalateActiveSessionRun moves the active session run to durable phase (/background).
func (s *ChatService) EscalateActiveSessionRun(ctx context.Context, sessionID string) (escalated bool, reply string, err error) {
	if s == nil || s.orch == nil || s.orch.chJobs().SessionRuns == nil {
		return false, channelBackgroundReplyNoActiveRun, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false, channelBackgroundReplyNoActiveRun, nil
	}
	run, err := s.orch.chJobs().SessionRuns.GetActiveForSession(ctx, sessionID)
	if err != nil || run.ID == "" {
		return false, channelBackgroundReplyNoActiveRun, nil
	}
	if run.Phase == biz.SessionRunPhaseDurable {
		return true, channelBackgroundReplyAlready, nil
	}
	s.orch.sessionRunLC().EscalateToDurableByUser(ctx, sessionID, run.ID)
	return true, channelBackgroundReplyOK, nil
}

// EscalateSessionRun moves a specific session run to durable phase (Feishu card callback).
func (s *ChatService) EscalateSessionRun(ctx context.Context, sessionRunID, expectedSessionID string) (reply string, err error) {
	if s == nil || s.orch == nil || s.orch.chJobs().SessionRuns == nil {
		return channelBackgroundReplyNoActiveRun, nil
	}
	sessionRunID = strings.TrimSpace(sessionRunID)
	expectedSessionID = strings.TrimSpace(expectedSessionID)
	if sessionRunID == "" {
		return channelBackgroundReplyNoActiveRun, nil
	}
	run, err := s.orch.chJobs().SessionRuns.Get(ctx, sessionRunID)
	if err != nil || run.ID == "" {
		return channelBackgroundReplyNoActiveRun, nil
	}
	if expectedSessionID != "" && run.SessionID != expectedSessionID {
		return channelBackgroundReplyDenied, nil
	}
	if run.Phase == biz.SessionRunPhaseDurable {
		return channelBackgroundReplyAlready, nil
	}
	if run.Phase == biz.SessionRunPhaseCompleted || run.Phase == biz.SessionRunPhaseFailed {
		return channelBackgroundReplyNoActiveRun, nil
	}
	s.orch.sessionRunLC().EscalateToDurableByUser(ctx, run.SessionID, run.ID)
	return channelBackgroundReplyOK, nil
}
