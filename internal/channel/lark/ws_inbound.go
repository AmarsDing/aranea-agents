package lark

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/runtime"
	"aranea-agents/internal/event"
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
	handler runtime.InboundHandler,
	ev port.InboundEvent,
) {
	defer func() {
		if r := recover(); r != nil {
			event.SysLogError(flowStepFeishuWSPanic, "飞书 WebSocket 入站 panic",
				event.P("channel_id", ch.ID),
				event.P("channel_key", ch.Key),
				event.P("recover", r),
			)
		}
	}()
	procCtx := context.WithoutCancel(parentCtx)
	if err := handler.ProcessInbound(procCtx, ch, ev); err != nil {
		event.SysLogWarn(flowStepFeishuWSInboundErr, "飞书 WebSocket 入站失败",
			event.P("channel_id", ch.ID),
			event.P("channel_key", ch.Key),
			event.P("peer_id", ev.PeerID),
			event.P("error", err.Error()),
		)
		notifyFeishuInboundError(procCtx, ch, creds, lookup, ev, err)
	}
}

func notifyFeishuInboundError(
	ctx context.Context,
	ch biz.Channel,
	creds []biz.ChannelCredential,
	lookup runtime.CredentialLookup,
	ev port.InboundEvent,
	err error,
) {
	if err == nil {
		return
	}
	recipient := strings.TrimSpace(ev.OutboundMeta["recipient"])
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
	msg := "处理消息失败：" + strings.TrimSpace(err.Error())
	_ = (&FeishuTextSender{
		Region:        region,
		AppID:         appID,
		AppSecret:     strings.TrimSpace(appSecret),
		ReceiveIDType: ReceiveIDTypeFromMeta(ev.OutboundMeta),
	}).SendText(ctx, recipient, msg)
}
