package service

import (
	"context"
	"net/http"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// inboundHTTPResult is returned by processInboundHTTP so platform handlers can shape HTTP responses.
type inboundHTTPResult struct {
	Outcome inboundAcceptOutcome
	Err     error
}

func writeInboundHTTPResponse(w http.ResponseWriter, result inboundHTTPResult) {
	if result.Err != nil {
		http.Error(w, "agent error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// processInboundHTTP accepts the inbound event and schedules background work when needed.
// The caller must write the HTTP response (see writeInboundHTTPResponse or platform-specific ACK).
func (h *ChannelIngress) processInboundHTTP(r *http.Request, chRow biz.Channel, ev port.InboundEvent) inboundHTTPResult {
	outcome, err := h.acceptInbound(r.Context(), chRow, ev, true)
	if err != nil {
		return inboundHTTPResult{Err: err}
	}
	h.scheduleInboundBackground(r, chRow, ev, outcome)
	return inboundHTTPResult{Outcome: outcome}
}

func (h *ChannelIngress) scheduleInboundBackground(r *http.Request, chRow biz.Channel, ev port.InboundEvent, outcome inboundAcceptOutcome) {
	if h == nil || !outcome.needsBackgroundWork() {
		return
	}
	chCopy := chRow
	evCopy := ev
	platform := inboundPlatform(chCopy, evCopy, h.lg)
	ltCfg := biz.ParseChannelLongTaskConfig(chCopy.ConfigJSON)
	release := outcome.releaseConcurrent
	safego.Go(appctx.Ctx(), "channel.inbound.background", func() {
		if release != nil {
			defer release()
		}
		procCtx := context.WithoutCancel(r.Context())
		if outcome.DispatchAsync {
			defer h.releaseInboundInflight(evCopy, platform)
			if err := h.dispatchAsyncInbound(procCtx, chCopy, evCopy, platform, ltCfg); err != nil {
				if replyErr := h.deliverTurnErrorReply(procCtx, chCopy, evCopy, platform, err); replyErr != nil {
					h.lg.Warn("异步回复投递失败",
						loggateway.StepID("channel.async.reply_failed"),
						loggateway.Err(replyErr),
					)
				}
				h.recordDelivery(procCtx, chCopy.ID, "error", map[string]any{"phase": "async_dispatch", "error": err.Error()}, err.Error())
			}
			return
		}
		if err := h.executeInboundTurn(procCtx, chCopy, evCopy); err != nil {
			h.recordDelivery(procCtx, chCopy.ID, "error", map[string]any{"phase": "async_execute", "error": err.Error()}, err.Error())
		}
	})
}
