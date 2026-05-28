package service

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/event"
	arametrics "aranea-agents/internal/metrics"
)

const flowStepChannelTurnExecute = "channel.turn.execute"

// executeInboundTurn runs the agent turn and outbound delivery after acceptInbound.
func (h *ChannelIngress) executeInboundTurn(ctx context.Context, chRow biz.Channel, ev port.InboundEvent) error {
	platform := inboundPlatform(chRow, ev)
	defer h.releaseInboundInflight(ev, platform)

	ltCfg := biz.ParseChannelLongTaskConfig(chRow.ConfigJSON)
	var cancel context.CancelFunc
	ctx, cancel = h.attachChannelTurnContext(ctx, ltCfg)
	if cancel != nil {
		defer cancel()
	}

	jobID, ctx, err := h.createTurnJob(ctx, chRow, ev, platform)
	if err != nil {
		_ = h.deliverTurnErrorReply(ctx, chRow, ev, platform, err)
		return err
	}
	h.markTurnJob(ctx, biz.ChannelTurnJobStatusRunning, "", "", "")
	if jobID != "" {
		arametrics.ChannelTurnJobTotal.WithLabelValues(chRow.ID, biz.ChannelTurnJobStatusRunning).Inc()
	}

	sessionID := h.resolveTurnSessionID(ctx, chRow, platform, ev)
	h.bindChannelPendingMode(sessionID, chRow.ConfigJSON)
	defer h.clearChannelPendingMode(sessionID)
	start := time.Now()
	h.logTurnFlow(ctx, sessionID, flowStepChannelTurnExecute, "Channel Turn 开始执行", nil,
		event.P("channel_id", chRow.ID),
		event.P("peer_id", ev.PeerID),
		event.P("streaming", biz.ChannelStreamingEnabled(chRow.ConfigJSON)),
		event.P("job_id", jobID),
	)

	var execErr error
	var contentPreview string
	var previewMsgID string
	terminalStatus := biz.ChannelTurnJobStatusCompleted
	defer func() {
		arametrics.ChannelTurnDuration.WithLabelValues(platform).Observe(time.Since(start).Seconds())
		if execErr == nil {
			h.markTurnJob(ctx, terminalStatus, "", previewMsgID, contentPreview)
			if jobID != "" {
				arametrics.ChannelTurnJobTotal.WithLabelValues(chRow.ID, terminalStatus).Inc()
			}
			step := flowStepChannelTurnDone
			msg := "Channel Turn 执行完成"
			if terminalStatus == biz.ChannelTurnJobStatusQueued {
				msg = "Channel Turn 已排队"
			}
			h.logTurnFlow(ctx, sessionID, step, msg, nil,
				event.P("channel_id", chRow.ID), event.P("job_id", jobID), event.P("status", terminalStatus))
			return
		}
		status := biz.ChannelTurnJobStatusFailed
		step := flowStepChannelTurnDone
		if turnErrorIsTimeout(execErr) {
			status = biz.ChannelTurnJobStatusTimeout
			step = flowStepChannelTurnTimeout
		}
		h.markTurnJob(ctx, status, execErr.Error(), "", "")
		if jobID != "" {
			arametrics.ChannelTurnJobTotal.WithLabelValues(chRow.ID, status).Inc()
		}
		h.logTurnFlow(ctx, sessionID, step, "Channel Turn 执行失败", execErr,
			event.P("channel_id", chRow.ID), event.P("job_id", jobID))
		_ = h.deliverTurnErrorReply(ctx, chRow, ev, platform, execErr)
		h.publishChannelTurnRunStatus(ctx, sessionID, jobID, "failed", formatChannelTurnErrorMessage(execErr))
	}()

	if handled, perr := h.rejectIfContextPressure(ctx, chRow, ev, platform, sessionID, ltCfg); handled {
		if perr == nil {
			terminalStatus = biz.ChannelTurnJobStatusCompleted
		} else {
			execErr = perr
		}
		return perr
	}

	if handled, perr := h.applyPreTurnIngressPolicy(ctx, chRow, ev, platform, sessionID, ltCfg); handled {
		if perr == nil {
			terminalStatus = biz.ChannelTurnJobStatusQueued
		} else {
			execErr = perr
		}
		return perr
	}

	var turnQueued bool
	stopReaction := h.startFeishuProcessingReaction(ctx, chRow, ev)
	defer stopReaction()
	if biz.ChannelStreamingEnabled(chRow.ConfigJSON) {
		execErr = h.processInboundStreaming(ctx, chRow, ev, platform, ltCfg, sessionID, &contentPreview, &previewMsgID, &turnQueued)
	} else {
		contentPreview, previewMsgID, turnQueued, execErr = h.processInboundUnaryWithOutcome(ctx, chRow, ev, platform, ltCfg, sessionID)
	}
	if execErr == nil && turnQueued {
		terminalStatus = biz.ChannelTurnJobStatusQueued
	}
	return execErr
}

func (h *ChannelIngress) resolveTurnSessionID(ctx context.Context, chRow biz.Channel, platform string, ev port.InboundEvent) string {
	peerKey, err := h.inboundPeerKey(chRow, ev)
	if err != nil {
		return ""
	}
	req, err := h.prepareChannelChatRequest(ctx, chRow, platform, peerKey, ev.PeerID, ev.Text)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(req.GetSessionId())
}

func (h *ChannelIngress) attachChannelTurnContext(ctx context.Context, ltCfg biz.ChannelLongTaskConfig) (context.Context, context.CancelFunc) {
	noop := func() {}
	var deadlines ChannelTurnDeadlines
	if ltCfg.TurnTimeoutSec > 0 {
		deadlines.TurnTimeout = time.Duration(ltCfg.TurnTimeoutSec) * time.Second
		ctx, cancel := applyChannelTurnTimeout(ctx, ltCfg.TurnTimeoutSec)
		if cancel == nil {
			cancel = noop
		}
		if ltCfg.FirstByteTimeoutSec > 0 {
			deadlines.FirstByteTimeout = time.Duration(ltCfg.FirstByteTimeoutSec) * time.Second
			return WithChannelTurnDeadlines(ctx, deadlines), cancel
		}
		return ctx, cancel
	}
	if ltCfg.FirstByteTimeoutSec > 0 {
		deadlines.FirstByteTimeout = time.Duration(ltCfg.FirstByteTimeoutSec) * time.Second
	}
	return WithChannelTurnDeadlines(ctx, deadlines), noop
}

func (h *ChannelIngress) processInboundUnaryWithOutcome(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform string, ltCfg biz.ChannelLongTaskConfig, sessionID string) (string, string, bool, error) {
	policy := biz.ParseChannelIMRenderPolicy(chRow.ConfigJSON, ltCfg)
	var previewCoord *TurnPreviewCoordinator
	var stopPreview context.CancelFunc
	if policy.Mode != biz.ChannelIMRenderModeReplyOnly && h.eventBus != nil && strings.TrimSpace(sessionID) != "" {
		previewCoord, stopPreview = h.startTurnPreviewAccumulate(ctx, sessionID, platform, chRow.ConfigJSON, ltCfg)
		defer stopPreview()
	}

	result, err := h.runChatTurnWithOutcome(ctx, chRow, platform, ev)
	if err != nil {
		return "", "", false, err
	}
	switch result.Outcome {
	case biz.TurnOutcomeQueued:
		if err := h.sendInboundQueuedAck(ctx, chRow, ev, platform, ltCfg, result.PendingID); err != nil {
			_ = h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "queued_ack", "error": err.Error()}, err.Error())
			return "", "", false, err
		}
		_ = h.recordDelivery(ctx, chRow.ID, "queued", map[string]any{"peer_id": ev.PeerID, "pending_id": result.PendingID}, "")
		return "", "", true, nil
	case biz.TurnOutcomeCompleted:
		reply := strings.TrimSpace(result.Reply)
		if previewCoord != nil {
			if rendered := strings.TrimSpace(previewCoord.RenderedText()); rendered != "" {
				reply = rendered
			}
			if previewID := strings.TrimSpace(previewCoord.PreviewMessageID()); previewID != "" {
				_ = previewCoord.FlushFinalText(ctx, reply)
				preview := truncateForLog(reply, 200)
				_ = h.recordDelivery(ctx, chRow.ID, "streamed", map[string]any{
					"peer_id":    ev.PeerID,
					"platform":   platform,
					"preview_id": previewID,
					"deduped":    true,
				}, "")
				return preview, previewID, false, nil
			}
		}
		preview := truncateForLog(reply, 200)
		return preview, "", false, h.deliverUnaryReply(ctx, chRow, ev, platform, reply)
	default:
		return "", "", false, nil
	}
}

func (h *ChannelIngress) deliverUnaryReply(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform, reply string) error {
	recipient := outboundRecipient(ev)
	idempotency := strings.TrimSpace(ev.IdempotencyKey)
	if idempotency == "" {
		idempotency = platform + ":" + ev.PeerID
	}
	if err := h.enqueueOutboundReply(ctx, chRow, platform, recipient, reply, ev.OutboundMeta, idempotency); err != nil {
		_ = h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "enqueue", "error": err.Error()}, err.Error())
		return err
	}
	_ = h.recordDelivery(ctx, chRow.ID, "queued", map[string]any{"peer_id": ev.PeerID}, "")
	return nil
}

// deliverStreamFlushFallback enqueues the final reply when in-place preview edit fails (turn already succeeded).
func (h *ChannelIngress) deliverStreamFlushFallback(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform, reply string) error {
	idempotency := ackIdempotencyKey(platform, ev, "stream_final")
	return h.enqueueOutboundReply(ctx, chRow, platform, outboundRecipient(ev), reply, ev.OutboundMeta, idempotency)
}
