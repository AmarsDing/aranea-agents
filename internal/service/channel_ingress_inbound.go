package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	arametrics "aranea-agents/internal/metrics"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const defaultChannelPassiveQueuedReply = "当前有任务进行中，请稍后再试。"

// processInboundCore runs a synchronous turn and returns reply text for platforms that
// must embed the assistant reply in the webhook HTTP body (e.g. WeChat passive mode).
// Includes TurnJob governance (creation, status tracking, metrics) and context pressure
// check to align with the async executeInboundTurn path (P1 #1).
func (h *ChannelIngress) processInboundCore(ctx context.Context, chRow biz.Channel, ev port.InboundEvent) (reply string, err error) {
	if h == nil || h.chat == nil || h.channels == nil || h.sessions == nil {
		return "", nil
	}
	platform := inboundPlatform(chRow, ev, h.lg)

	// Resolve TurnInput once (incl. session) and reuse it for both job creation
	// and downstream execution, avoiding redundant prepareChannelChatRequest /
	// ensureChannelSession DB calls (P1 #3). TurnJob 用于治理追踪（P1 #1）。
	turnInput, turnInputErr := h.resolveTurnInput(ctx, chRow, platform, ev)
	sessionID := ""
	if turnInputErr == nil {
		sessionID = strings.TrimSpace(turnInput.SessionID)
	}

	jobID, ctx, err := h.createTurnJob(ctx, chRow, ev, platform, sessionID)
	if err != nil {
		return "", err
	}
	h.markTurnJobByEvent(ctx, biz.JobEventStart, "", "", "")
	if jobID != "" {
		arametrics.ChannelTurnJobTotal.WithLabelValues(chRow.ID, biz.ChannelTurnJobStatusRunning).Inc()
	}

	var execErr error
	terminalEvent := biz.JobEventComplete
	defer func() {
		if execErr == nil {
			h.markTurnJobByEvent(ctx, terminalEvent, "", "", "")
			terminalStatus, _ := biz.ChannelTurnJobStatusFromEvent(terminalEvent)
			if jobID != "" && terminalStatus != "" {
				arametrics.ChannelTurnJobTotal.WithLabelValues(chRow.ID, terminalStatus).Inc()
			}
			return
		}
		failEvent := biz.JobEventFail
		h.markTurnJobByEvent(ctx, failEvent, execErr.Error(), "", "")
		failStatus, _ := biz.ChannelTurnJobStatusFromEvent(failEvent)
		if jobID != "" && failStatus != "" {
			arametrics.ChannelTurnJobTotal.WithLabelValues(chRow.ID, failStatus).Inc()
		}
	}()

	// Context pressure check: return error message synchronously (passive mode can't enqueue).
	if h.sessionContextPressure(ctx, sessionID) {
		recordIngressIntentMetric("context_pressure")
		execErr = errContextPressureRejected
		return biz.ChannelTurnErrorContextOverflowMsg, nil
	}

	if turnInputErr != nil {
		execErr = turnInputErr
		return "", turnInputErr
	}

	result, runErr := h.runChatTurnWithInput(ctx, chRow, platform, turnInput)
	if runErr != nil {
		if isTurnMessageQueued(runErr) || result.Outcome == biz.TurnOutcomeQueued {
			pendingID := strings.TrimSpace(result.PendingID)
			if pendingID == "" && h.chat != nil {
				pendingID = h.chat.LastPendingMessageID(sessionID)
			}
			terminalEvent = biz.JobEventQueue
			ltCfg := biz.ParseChannelLongTaskConfig(chRow.ConfigJSON)
			text := strings.TrimSpace(ltCfg.AckOnQueued)
			if text == "" {
				text = defaultChannelPassiveQueuedReply
			}
			return biz.RenderChannelTemplate(text, map[string]string{"pending_id": pendingID}), nil
		}
		execErr = runErr
		return "", runErr
	}

	return result.Reply, nil
}

// processWeChatPassiveInbound gates idempotency/access then runs a sync turn for XML reply.
func (h *ChannelIngress) processWeChatPassiveInbound(ctx context.Context, chRow biz.Channel, ev port.InboundEvent) (reply string, err error) {
	platform := "wechat"
	proceed, denyReply, err := h.gateInboundBeforeTurn(ctx, chRow, ev, true)
	if err != nil {
		return "", err
	}
	if !proceed {
		return denyReply, nil
	}
	defer h.releaseInboundInflight(ev, platform)

	if handled, cancelReply, cerr := h.resolveCancelInboundTurn(ctx, chRow, ev, platform); handled {
		return cancelReply, cerr
	}
	return h.processInboundCore(ctx, chRow, ev)
}

// ProcessInbound runs accept + synchronous execute (runtime WS path).
func (h *ChannelIngress) ProcessInbound(ctx context.Context, chRow biz.Channel, ev port.InboundEvent) error {
	platform := inboundPlatform(chRow, ev, h.lg)
	if biz.IngressDebounceEnabled(platform) && h.peerDebouncer != nil {
		peerKey := strings.TrimSpace(ev.PeerKey)
		if peerKey == "" {
			peerKey = strings.TrimSpace(ev.PeerID)
		}
		run := func(ctx context.Context) error {
			return h.processInboundNow(ctx, chRow, ev)
		}
		h.peerDebouncer.Submit(ctx, chRow.ID, ev.PeerID, peerKey, ev.Text, ev.IdempotencyKey, run)
		return nil
	}
	return h.processInboundNow(ctx, chRow, ev)
}

func (h *ChannelIngress) processInboundNow(ctx context.Context, chRow biz.Channel, ev port.InboundEvent) error {
	outcome, err := h.acceptInbound(ctx, chRow, ev, false)
	if err != nil {
		return err
	}
	platform := inboundPlatform(chRow, ev, h.lg)
	if outcome.DispatchAsync {
		ltCfg := biz.ParseChannelLongTaskConfig(chRow.ConfigJSON)
		release := outcome.releaseConcurrent
		safego.Go(context.WithoutCancel(ctx), "channel.inbound.async", func() {
			procCtx := context.WithoutCancel(ctx)
			defer h.releaseInboundInflight(ev, platform)
			if release != nil {
				defer release()
			}
			if err := h.dispatchAsyncInbound(procCtx, chRow, ev, platform, ltCfg); err != nil {
				if replyErr := h.deliverTurnErrorReply(procCtx, chRow, ev, platform, err); replyErr != nil {
					h.lg.Warn("异步回复投递失败",
						loggateway.StepID("channel.async.reply_failed"),
						loggateway.Err(replyErr),
					)
				}
			}
		})
		return nil
	}
	if !outcome.ExecuteSync {
		return nil
	}
	release := outcome.releaseConcurrent
	if release != nil {
		defer release()
	}
	if err := h.executeInboundTurn(ctx, chRow, ev); err != nil {
		// executeInboundTurn already enqueues a user-visible IM error (LT-06).
		return nil
	}
	return nil
}
