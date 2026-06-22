package lark

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/runtime"
	"aranea-agents/pkg/loggateway"
)

const (
	flowStepFeishuWSPanic      = "channel.feishu.ws.panic"
	flowStepFeishuWSInboundErr = "channel.feishu.ws.inbound_fail"
)

// HandleWSInbound runs ProcessInbound on a detached context and notifies the peer on failure.
func HandleWSInbound(
	parentCtx context.Context,
	ch biz.Channel,
	creds []biz.ChannelCredential,
	lookup runtime.CredentialLookup,
	handler port.InboundHandler,
	ev port.InboundEvent,
	lg loggateway.Logger,
) {
	defer func() {
		if r := recover(); r != nil {
			lg.Error("飞书 WebSocket 入站 panic",
				loggateway.StepID(flowStepFeishuWSPanic),
				loggateway.Str("channel_id", ch.ID),
				loggateway.Str("channel_key", ch.Key),
				loggateway.Any("recover", r),
			)
		}
	}()
	procCtx := context.WithoutCancel(parentCtx)
	if err := handler.ProcessInbound(procCtx, ch, ev); err != nil {
		lg.Warn("飞书 WebSocket 入站失败",
			loggateway.StepID(flowStepFeishuWSInboundErr),
			loggateway.Str("channel_id", ch.ID),
			loggateway.Str("channel_key", ch.Key),
			loggateway.Str("peer_id", ev.PeerID),
			loggateway.Err(err),
		)
		notifyFeishuInboundError(procCtx, ch, creds, lookup, ev, err, lg)
	}
}

func notifyFeishuInboundError(
	ctx context.Context,
	ch biz.Channel,
	creds []biz.ChannelCredential,
	lookup runtime.CredentialLookup,
	ev port.InboundEvent,
	err error,
	lg loggateway.Logger,
) {
	if err == nil {
		return
	}
	recipient := strings.TrimSpace(ev.OutboundMeta[port.MetaRecipient])
	if recipient == "" {
		return
	}
	region, appID, rerr := AppAndRegionFromConfig(ch.ConfigJSON)
	if rerr != nil {
		return
	}
	appSecret, serr := lookup(ctx, creds, "app_secret")
	if serr != nil || strings.TrimSpace(appSecret) == "" {
		return
	}
	// SEC-02: do not expose internal error details to end users.
	msg := "消息处理失败，请稍后重试"
	_ = err
	if sendErr := (&FeishuTextSender{
		Region:        region,
		AppID:         appID,
		AppSecret:     strings.TrimSpace(appSecret),
		ReceiveIDType: ReceiveIDTypeFromMeta(ev.OutboundMeta),
	}).SendText(ctx, recipient, msg); sendErr != nil {
		lg.Warn("飞书错误通知发送失败",
			loggateway.StepID("channel.feishu.ws.notify_fail"),
			loggateway.Err(sendErr),
		)
	}
}
