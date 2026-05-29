package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"
)

const defaultChannelPassiveQueuedReply = "当前有任务进行中，请稍后再试。"

// processInboundCore runs a synchronous turn and returns reply text for platforms that
// must embed the assistant reply in the webhook HTTP body (e.g. WeChat passive mode).
func (h *ChannelIngress) processInboundCore(ctx context.Context, chRow biz.Channel, ev port.InboundEvent) (reply string, err error) {
	if h == nil || h.chat == nil || h.channels == nil || h.sessions == nil {
		return "", nil
	}
	platform := inboundPlatform(chRow, ev)
	result, err := h.runChatTurnWithOutcome(ctx, chRow, platform, ev)
	if err != nil {
		return "", err
	}
	switch result.Outcome {
	case biz.TurnOutcomeQueued:
		ltCfg := biz.ParseChannelLongTaskConfig(chRow.ConfigJSON)
		text := strings.TrimSpace(ltCfg.AckOnQueued)
		if text == "" {
			text = defaultChannelPassiveQueuedReply
		}
		return biz.RenderChannelTemplate(text, map[string]string{"pending_id": result.PendingID}), nil
	default:
		return result.Reply, nil
	}
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
	platform := inboundPlatform(chRow, ev)
	if ingressDebounceEnabled(platform) && h.peerDebouncer != nil {
		h.peerDebouncer.submit(ctx, chRow, ev, h.processInboundNow)
		return nil
	}
	return h.processInboundNow(ctx, chRow, ev)
}

func (h *ChannelIngress) processInboundNow(ctx context.Context, chRow biz.Channel, ev port.InboundEvent) error {
	outcome, err := h.acceptInbound(ctx, chRow, ev, false)
	if err != nil {
		return err
	}
	platform := inboundPlatform(chRow, ev)
	if outcome.DispatchAsync {
		ltCfg := biz.ParseChannelLongTaskConfig(chRow.ConfigJSON)
		release := outcome.releaseConcurrent
		safego.Go(context.Background(), "channel.inbound.async", func() {
			procCtx := context.WithoutCancel(ctx)
			defer h.releaseInboundInflight(ev, platform)
			if release != nil {
				defer release()
			}
			if err := h.dispatchAsyncInbound(procCtx, chRow, ev, platform, ltCfg); err != nil {
			if replyErr := h.deliverTurnErrorReply(procCtx, chRow, ev, platform, err); replyErr != nil {
				event.SysLogWarn("channel.async.reply_failed", "异步回复投递失败", event.P("error", replyErr.Error()))
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
