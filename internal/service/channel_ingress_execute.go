package service

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/event"
	arametrics "aranea-agents/internal/metrics"
	"aranea-agents/pkg/loggateway"
)

const flowStepChannelTurnExecute = "channel.turn.execute"

// executeInboundTurn runs the agent turn and outbound delivery after acceptInbound.
func (h *ChannelIngress) executeInboundTurn(ctx context.Context, chRow biz.Channel, ev port.InboundEvent) error {
	platform := inboundPlatform(chRow, ev, h.lg)
	defer h.releaseInboundInflight(ev, platform)

	ltCfg := biz.ParseChannelLongTaskConfig(chRow.ConfigJSON)
	var cancel context.CancelFunc
	ctx, cancel = h.attachChannelTurnContext(ctx, ltCfg)
	if cancel != nil {
		defer cancel()
	}

	jobID, ctx, err := h.createTurnJob(ctx, chRow, ev, platform)
	if err != nil {
		if replyErr := h.deliverTurnErrorReply(ctx, chRow, ev, platform, err); replyErr != nil {
			h.lg.Warn("异步回复投递失败",
				loggateway.StepID("channel.async.reply_failed"),
				loggateway.Err(replyErr),
			)
		}
		return err
	}
	h.markTurnJobByEvent(ctx, biz.JobEventStart, "", "", "")
	if jobID != "" {
		arametrics.ChannelTurnJobTotal.WithLabelValues(chRow.ID, biz.ChannelTurnJobStatusRunning).Inc()
	}

	// Resolve TurnInput once and pass it downstream to avoid redundant
	// prepareChannelChatRequest / ensureChannelSession DB calls (P1 #3).
	turnInput, turnInputErr := h.resolveTurnInput(ctx, chRow, platform, ev)
	sessionID := ""
	if turnInputErr == nil {
		sessionID = strings.TrimSpace(turnInput.SessionID)
	}
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
	// terminalEvent is the single source of truth for the job's terminal state.
	// Success paths set it to Complete (default) or Queue; the defer derives
	// Fail/Timeout from execErr for error paths.
	terminalEvent := biz.JobEventComplete
	defer func() {
		arametrics.ChannelTurnDuration.WithLabelValues(platform).Observe(time.Since(start).Seconds())

		// Derive terminalEvent from execErr for error paths.
		errMsg := ""
		jobPreviewMsgID := ""
		jobContentPreview := ""
		if execErr != nil {
			terminalEvent = biz.JobEventFail
			if turnErrorIsTimeout(execErr) {
				terminalEvent = biz.JobEventTimeout
			}
			errMsg = execErr.Error()
		} else {
			jobPreviewMsgID = previewMsgID
			jobContentPreview = contentPreview
		}

		h.markTurnJobByEvent(ctx, terminalEvent, errMsg, jobPreviewMsgID, jobContentPreview)
		terminalStatus, _ := biz.ChannelTurnJobStatusFromEvent(terminalEvent)
		if jobID != "" && terminalStatus != "" {
			arametrics.ChannelTurnJobTotal.WithLabelValues(chRow.ID, terminalStatus).Inc()
		}

		if execErr == nil {
			step := flowStepChannelTurnDone
			msg := "Channel Turn 执行完成"
			if terminalStatus == biz.ChannelTurnJobStatusQueued {
				msg = "Channel Turn 已排队"
			}
			h.logTurnFlow(ctx, sessionID, step, msg, nil,
				event.P("channel_id", chRow.ID), event.P("job_id", jobID), event.P("status", terminalStatus))
			return
		}
		step := flowStepChannelTurnDone
		if terminalStatus == biz.ChannelTurnJobStatusTimeout {
			step = flowStepChannelTurnTimeout
		}
		h.logTurnFlow(ctx, sessionID, step, "Channel Turn 执行失败", execErr,
			event.P("channel_id", chRow.ID), event.P("job_id", jobID))
		if replyErr := h.deliverTurnErrorReply(ctx, chRow, ev, platform, execErr); replyErr != nil {
			h.lg.Warn("异步回复投递失败",
				loggateway.StepID("channel.async.reply_failed"),
				loggateway.Err(replyErr),
			)
		}
		h.publishChannelTurnRunStatus(ctx, sessionID, jobID, "failed", formatChannelTurnErrorMessage(execErr))
	}()

	if handled, perr := h.rejectIfContextPressure(ctx, chRow, ev, platform, sessionID); handled {
		if perr == nil {
			// Context pressure rejected the turn and the error reply was already
			// sent to the user. Mark as failed (not completed) since the turn
			// did not execute. The sentinel error skips duplicate reply in defer.
			execErr = errContextPressureRejected
		} else {
			execErr = perr
		}
		return perr
	}

	if handled, perr := h.applyPreTurnIngressPolicy(ctx, chRow, ev, platform, sessionID, ltCfg); handled {
		if perr == nil {
			terminalEvent = biz.JobEventQueue
		} else {
			execErr = perr
		}
		return perr
	}

	var turnQueued bool
	stopReaction := h.startFeishuProcessingReaction(ctx, chRow, ev)
	defer stopReaction()
	if turnInputErr != nil {
		// Session resolution failed; let the streaming/unary path surface the error.
		execErr = turnInputErr
		return execErr
	}
	if biz.ChannelStreamingEnabled(chRow.ConfigJSON) {
		execErr = h.processInboundStreaming(ctx, chRow, ev, platform, ltCfg, sessionID, turnInput, &contentPreview, &previewMsgID, &turnQueued)
	} else {
		contentPreview, previewMsgID, turnQueued, execErr = h.processInboundUnaryWithOutcome(ctx, chRow, ev, platform, ltCfg, sessionID, turnInput)
	}
	if execErr == nil && turnQueued {
		terminalEvent = biz.JobEventQueue
	}
	return execErr
}

// resolveTurnInput resolves the TurnInput (including session) once for executeInboundTurn,
// so it can be reused by streaming/unary paths without redundant DB calls.
func (h *ChannelIngress) resolveTurnInput(ctx context.Context, chRow biz.Channel, platform string, ev port.InboundEvent) (biz.TurnInput, error) {
	peerKey, err := h.inboundPeerKey(chRow, ev)
	if err != nil {
		h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "routing", "error": err.Error()}, err.Error())
		return biz.TurnInput{}, err
	}
	return h.prepareChannelChatRequest(ctx, chRow, platform, peerKey, ev.PeerID, ev.Text, channelAllowQueueFromConfig(chRow.ConfigJSON))
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

func (h *ChannelIngress) processInboundUnaryWithOutcome(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform string, ltCfg biz.ChannelLongTaskConfig, sessionID string, turnInput biz.TurnInput) (string, string, bool, error) {
	policy := biz.ParseChannelIMRenderPolicy(chRow.ConfigJSON, ltCfg)
	var previewCoord *TurnPreviewCoordinator
	var stopPreview context.CancelFunc
	if policy.Mode != biz.ChannelIMRenderModeReplyOnly && strings.TrimSpace(sessionID) != "" {
		previewCoord, stopPreview = h.startTurnPreviewAccumulate(ctx, sessionID, platform, chRow.ConfigJSON, ltCfg)
		defer stopPreview()
	}

	result, err := h.runChatTurnWithInput(ctx, chRow, platform, turnInput)
	if err != nil {
		return "", "", false, err
	}
	switch result.Outcome {
	case biz.TurnOutcomeQueued:
		if err := h.sendInboundQueuedAck(ctx, chRow, ev, platform, ltCfg, result.PendingID); err != nil {
			h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "queued_ack", "error": err.Error()}, err.Error())
			return "", "", false, err
		}
		h.recordDelivery(ctx, chRow.ID, "queued", map[string]any{"peer_id": ev.PeerID, "pending_id": result.PendingID}, "")
		return "", "", true, nil
	case biz.TurnOutcomeCompleted:
		reply := strings.TrimSpace(result.Reply)
		if reply == "" {
			h.lg.Warn("Channel Turn 返回空回复，使用 fallback 消息",
				loggateway.StepID("channel.turn.empty_reply"),
				loggateway.Str("channel_id", chRow.ID),
				loggateway.Str("session_id", sessionID),
			)
			reply = biz.ChannelTurnEmptyReplyMsg
		}
		if previewCoord != nil {
			// transcript no longer subscribes to EventBus (Blocker F Stage 1);
			// it only holds the initial ACK, so don't let it override the LLM reply.
			if previewID := strings.TrimSpace(previewCoord.PreviewMessageID()); previewID != "" {
				if err := previewCoord.FlushFinalText(ctx, reply); err != nil {
					h.lg.Warn("preview flush final text failed", loggateway.StepID("channel.turn.preview_flush"), loggateway.Err(err))
				}
				preview := truncateForLog(reply, 200)
				h.recordDelivery(ctx, chRow.ID, "streamed", map[string]any{
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
		h.recordDelivery(ctx, chRow.ID, "error", map[string]any{"phase": "enqueue", "error": err.Error()}, err.Error())
		return err
	}
	h.recordDelivery(ctx, chRow.ID, "queued", map[string]any{"peer_id": ev.PeerID}, "")
	return nil
}

// deliverStreamFlushFallback enqueues the final reply when in-place preview edit fails (turn already succeeded).
func (h *ChannelIngress) deliverStreamFlushFallback(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform, reply string) error {
	idempotency := ackIdempotencyKey(platform, ev, "stream_final")
	return h.enqueueOutboundReply(ctx, chRow, platform, outboundRecipient(ev), reply, ev.OutboundMeta, idempotency)
}
