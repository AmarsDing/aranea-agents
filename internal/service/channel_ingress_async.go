package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const (
	defaultAsyncAckTemplate      = "后台任务已创建（{{target_type}}: {{target_id}}）。完成后将通知你。"
	defaultAsyncCronDoneTemplate = "Cron 任务已完成（{{target_id}}）。"
	// asyncWatchTimeout is the in-process watch ceiling until CC-F-01 durable worker ships.
	asyncWatchTimeout     = biz.ChannelAsyncJobWatchMax
	asyncCronPollInterval = 5 * time.Second
)

func asyncWatchPersistCtx(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}

func (h *ChannelIngress) dispatchAsyncInbound(
	ctx context.Context,
	chRow biz.Channel,
	ev port.InboundEvent,
	platform string,
	ltCfg biz.ChannelLongTaskConfig,
) error {
	jobID, ctx, err := h.createTurnJob(ctx, chRow, ev, platform)
	if err != nil {
		return err
	}
	ctx = withChannelTurnJobID(ctx, jobID)

	peerKey, err := h.inboundPeerKey(chRow, ev)
	if err != nil {
		h.markTurnJobByEvent(ctx, biz.JobEventFail, err.Error(), "", "")
		return err
	}
	turnInput, err := h.prepareChannelChatRequest(ctx, chRow, platform, peerKey, ev.PeerID, ev.Text, false)
	if err != nil {
		h.markTurnJobByEvent(ctx, biz.JobEventFail, err.Error(), "", "")
		return err
	}
	sessionID := strings.TrimSpace(turnInput.SessionID)
	input := strings.TrimSpace(ev.Text)
	if strings.HasPrefix(strings.ToLower(input), "/async") {
		input = strings.TrimSpace(input[len("/async"):])
	}

	var targetType, targetID, asyncID string
	asyncTarget, resolveErr := biz.ResolveChannelAsyncGraphTarget(ltCfg)
	switch {
	case resolveErr == nil:
		targetType, targetID, asyncID, err = h.executeAsyncGraphTarget(ctx, asyncTarget, sessionID, map[string]any{
			"input":    input,
			"channel":  chRow.Key,
			"peer_id":  ev.PeerID,
			"platform": platform,
		})
		if err != nil {
			h.markTurnJobByEvent(ctx, biz.JobEventFail, err.Error(), "", "")
			return err
		}
	case errors.Is(resolveErr, biz.ErrAsyncTargetNotConfigured) && ltCfg.AsyncCronTaskID != "" && h.cron != nil:
		// No graph target configured; fall back to cron if available.
		targetType = "cron"
		targetID = ltCfg.AsyncCronTaskID
		run, cerr := h.cron.TriggerCronTask(ctx, targetID)
		if cerr != nil {
			h.markTurnJobByEvent(ctx, biz.JobEventFail, cerr.Error(), "", "")
			return cerr
		}
		asyncID = strings.TrimSpace(run.ID)
	default:
		h.markTurnJobByEvent(ctx, biz.JobEventFail, resolveErr.Error(), "", "")
		return resolveErr
	}

	h.markTurnJobByEvent(ctx, biz.JobEventAsyncQueue, "", "", "")
	h.markTurnJobAsyncTarget(ctx, targetType, asyncID)
	msg := biz.RenderChannelTemplate(defaultAsyncAckTemplate, map[string]string{
		"target_type": targetType,
		"target_id":   asyncID,
	})
	if err := h.enqueueOutboundReply(ctx, chRow, platform, outboundRecipient(ev), msg, ev.OutboundMeta, ackIdempotencyKey(platform, ev, "async")); err != nil {
		return err
	}
	h.recordDelivery(ctx, chRow.ID, "async_queued", map[string]any{
		"target_type": targetType,
		"target_id":   asyncID,
		"session_id":  sessionID,
	}, "")

	switch targetType {
	case "graph", "team_graph":
		if asyncID != "" {
			h.watchAsyncGraphCompletion(ctx, chRow, ev, platform, sessionID, asyncID)
		}
	case "cron":
		if asyncID != "" {
			h.watchAsyncCronCompletion(ctx, chRow, ev, platform, asyncID)
		}
	}
	h.logTurnFlow(ctx, sessionID, flowStepChannelTurnDone, "Channel 异步任务已派发",
		nil, event.P("target_type", targetType), event.P("target_id", asyncID))
	return nil
}

func (h *ChannelIngress) watchAsyncGraphCompletion(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform, sessionID, execID string) {
	if h == nil || h.eventBus == nil {
		return
	}
	jobID := channelTurnJobIDFromContext(ctx)
	chCopy := chRow
	evCopy := ev
	// Use context.WithoutCancel(ctx) so the watch outlives the HTTP request
	// but is still cancelled when the parent process shuts down (unlike appctx.Ctx()
	// which is a background context that never cancels).
	watchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), asyncWatchTimeout)
	safego.Go(context.WithoutCancel(ctx), "channel.async.graph.watch", func() {
		defer cancel()
		ch, unsub := h.eventBus.Subscribe(biz.EventSubscribeOptions{
			SpiritSessionID: sessionID,
		})
		defer unsub()
		for {
			select {
			case <-watchCtx.Done():
				h.finishAsyncTargetWatch(asyncWatchPersistCtx(ctx), chCopy, evCopy, platform, jobID, execID, "graph", watchCtx.Err())
				return
			case e, ok := <-ch:
				if !ok {
					h.failAsyncTargetWatch(asyncWatchPersistCtx(ctx), chCopy, evCopy, platform, jobID, execID, "graph", errors.New("event bus subscription closed"))
					return
				}
				notice, ok := e.(*biz.SystemNoticeEvent)
				if !ok {
					continue
				}
				aev := biz.ActivityEventFromSystemNotice(notice)
				if aev.Activity.Kind != biz.ActivityKindGraphStage {
					continue
				}
				metaID := metaStringFromActivity(aev, "execution_id")
				if metaID != "" && metaID != execID {
					continue
				}
				switch aev.Activity.Stage {
				case "node_error":
					if aev.Event == biz.ActivityEventFailed {
						h.failAsyncTargetWatch(asyncWatchPersistCtx(ctx), chCopy, evCopy, platform, jobID, execID, "graph", graphNodeErrorMessageFromActivity(aev))
						return
					}
				case "execution_done":
					if aev.Event != biz.ActivityEventCompleted {
						continue
					}
					if failed, errMsg := graphExecutionSummaryFailedFromActivity(aev, h.lg); failed {
						h.failAsyncTargetWatch(asyncWatchPersistCtx(ctx), chCopy, evCopy, platform, jobID, execID, "graph", errors.New(errMsg))
						return
					}
					summary := channelAsyncGraphDoneSummary
					if content := strings.TrimSpace(aev.Activity.Content); content != "" {
						summary = content
					}
					h.completeAsyncTargetWatch(asyncWatchPersistCtx(ctx), chCopy, evCopy, platform, jobID, summary)
					return
				}
			}
		}
	})
}

func (h *ChannelIngress) watchAsyncCronCompletion(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform, runID string) {
	if h == nil || h.cron == nil {
		return
	}
	jobID := channelTurnJobIDFromContext(ctx)
	chCopy := chRow
	evCopy := ev
	watchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), asyncWatchTimeout)
	safego.Go(context.WithoutCancel(ctx), "channel.async.cron.watch", func() {
		defer cancel()
		ticker := time.NewTicker(asyncCronPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-watchCtx.Done():
				h.finishAsyncTargetWatch(asyncWatchPersistCtx(ctx), chCopy, evCopy, platform, jobID, runID, "cron", watchCtx.Err())
				return
			case <-ticker.C:
				run, err := h.cron.GetTaskRun(asyncWatchPersistCtx(ctx), runID)
				if err != nil {
					h.lg.Warn("Cron 任务状态查询失败",
						loggateway.StepID("channel.async.cron_get_failed"),
						loggateway.Str("run_id", runID),
						loggateway.Err(err),
					)
					continue
				}
				switch strings.ToLower(strings.TrimSpace(run.Status)) {
				case "success":
					summary := biz.RenderChannelTemplate(defaultAsyncCronDoneTemplate, map[string]string{"target_id": runID})
					h.completeAsyncTargetWatch(asyncWatchPersistCtx(ctx), chCopy, evCopy, platform, jobID, summary)
					return
				case "failure", "failed":
					watchErr := errors.New(strings.TrimSpace(run.ErrorMessage))
					if watchErr.Error() == "" {
						watchErr = errors.New("cron task failed")
					}
					h.failAsyncTargetWatch(asyncWatchPersistCtx(ctx), chCopy, evCopy, platform, jobID, runID, "cron", watchErr)
					return
				case "skipped":
					h.completeAsyncTargetWatch(asyncWatchPersistCtx(ctx), chCopy, evCopy, platform, jobID, channelAsyncCronSkippedSummary)
					return
				}
			}
		}
	})
}

func (h *ChannelIngress) completeAsyncTargetWatch(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform, jobID, summary string) {
	if h == nil {
		return
	}
	if jobID != "" && h.turnJobs != nil {
		if err := h.turnJobs.TransitionByEvent(ctx, jobID, biz.JobEventComplete, "", "", truncateForLog(summary, 200)); err != nil {
			h.lg.Warn("异步任务状态更新失败",
				loggateway.StepID("channel.async.job_status_update_failed"),
				loggateway.Str("job_id", jobID),
				loggateway.Err(err),
			)
		}
	}
	if err := h.enqueueOutboundReply(ctx, chRow, platform, outboundRecipient(ev), summary, ev.OutboundMeta, ackIdempotencyKey(platform, ev, "async_done")); err != nil {
		h.lg.Warn("enqueueOutboundReply failed",
			loggateway.StepID("channel.async.reply"),
			loggateway.Err(err),
		)
	}
}

func (h *ChannelIngress) failAsyncTargetWatch(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform, jobID, targetID, targetType string, cause error) {
	if h == nil || cause == nil {
		return
	}
	if jobID != "" && h.turnJobs != nil {
		if err := h.turnJobs.TransitionByEvent(ctx, jobID, biz.JobEventFail, truncateForLog(cause.Error(), 200), "", ""); err != nil {
			h.lg.Warn("异步任务状态更新失败",
				loggateway.StepID("channel.async.job_status_update_failed"),
				loggateway.Str("job_id", jobID),
				loggateway.Err(err),
			)
		}
	}
	if err := h.deliverTurnErrorReply(ctx, chRow, ev, platform, cause); err != nil {
		h.lg.Warn("deliverTurnErrorReply failed",
			loggateway.StepID("channel.async.error_reply"),
			loggateway.Err(err),
		)
	}
	h.lg.Warn("Channel 异步任务失败",
		loggateway.StepID(flowStepChannelTurnDone),
		loggateway.Str("target_type", targetType),
		loggateway.Str("target_id", targetID),
		loggateway.Str("job_id", jobID),
		loggateway.Err(cause),
	)
}

func (h *ChannelIngress) finishAsyncTargetWatch(ctx context.Context, chRow biz.Channel, ev port.InboundEvent, platform, jobID, targetID, targetType string, cause error) {
	if h == nil || cause == nil {
		return
	}
	if errors.Is(cause, context.Canceled) {
		return
	}
	watchEvent := biz.JobEventFail
	watchErr := cause
	if errors.Is(cause, context.DeadlineExceeded) {
		watchEvent = biz.JobEventTimeout
		watchErr = context.DeadlineExceeded
	}
	if jobID != "" && h.turnJobs != nil {
		if err := h.turnJobs.TransitionByEvent(ctx, jobID, watchEvent, truncateForLog(cause.Error(), 200), "", ""); err != nil {
			h.lg.Warn("异步任务状态更新失败",
				loggateway.StepID("channel.async.job_status_update_failed"),
				loggateway.Str("job_id", jobID),
				loggateway.Err(err),
			)
		}
	}
	if err := h.deliverTurnErrorReply(ctx, chRow, ev, platform, watchErr); err != nil {
		h.lg.Warn("deliverTurnErrorReply failed",
			loggateway.StepID("channel.async.error_reply"),
			loggateway.Err(err),
		)
	}
	h.lg.Warn("Channel 异步任务监听结束",
		loggateway.StepID(flowStepChannelTurnDone),
		loggateway.Str("target_type", targetType),
		loggateway.Str("target_id", targetID),
		loggateway.Str("job_id", jobID),
		loggateway.Err(cause),
	)
}

func metaStringFromActivity(aev biz.ActivityEvent, key string) string {
	if aev.Activity.Meta == nil {
		return ""
	}
	v, ok := aev.Activity.Meta[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}

func graphNodeErrorMessageFromActivity(aev biz.ActivityEvent) error {
	if content := strings.TrimSpace(aev.Activity.Content); content != "" {
		return errors.New(content)
	}
	if msg := metaStringFromActivity(aev, "error"); msg != "" {
		return errors.New(msg)
	}
	return errors.New("graph node error")
}

func graphExecutionSummaryFailedFromActivity(aev biz.ActivityEvent, lg loggateway.Logger) (bool, string) {
	raw, ok := aev.Activity.Meta["execution_summary"]
	if !ok || raw == nil {
		return false, ""
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return false, ""
	}
	var summary struct {
		Nodes []struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(data, &summary); err != nil {
		lg.Warn("graph execution summary unmarshal failed", loggateway.StepID("channel.ingress_async.graph_summary"), loggateway.Err(err))
		return false, ""
	}
	for _, node := range summary.Nodes {
		if strings.EqualFold(strings.TrimSpace(node.Status), "error") {
			msg := strings.TrimSpace(node.Error)
			if msg == "" {
				msg = "graph execution failed"
			}
			return true, msg
		}
	}
	return false, ""
}
