package lark

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/runtime"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkcallback "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

func init() {
	runtime.RegisterStarterWithLogger("feishu", "websocket", RunWebSocket)
}

func RunWebSocket(
	ctx context.Context,
	ch biz.Channel,
	creds []biz.ChannelCredential,
	lookup runtime.CredentialLookup,
	handler port.InboundHandler,
	lg loggateway.Logger,
) error {
	lg.Info("飞书 WebSocket 连接器启动",
		loggateway.StepID("channel.feishu.ws.start"),
		loggateway.Str("channel_id", ch.ID),
	)
	appID, appSecret, err := WSAppCredentials(ctx, ch, creds, lookup)
	if err != nil {
		lg.Error("飞书 WebSocket 凭据获取失败",
			loggateway.StepID("channel.feishu.ws.creds_fail"),
			loggateway.Str("channel_id", ch.ID),
			loggateway.Err(err),
		)
		return err
	}
	chRow := ch
	batcher := NewTextInboundBatcher(lg)
	onMessage := func(_ context.Context, message *larkim.P2MessageReceiveV1) error {
		ev, ok := InboundEventFromWSMessage(message)
		if !ok {
			return nil
		}
		ev.PlatformType = "feishu"
		safego.Go(ctx, "channel.feishu.ws.inbound", func() {
			batcher.Submit(ctx, wsInboundBridge{ctx: ctx, ch: chRow, creds: creds, lookup: lookup, handler: handler, lg: lg}, chRow, ev, lg)
		})
		return nil
	}
	onCardAction := func(_ context.Context, event *larkcallback.CardActionTriggerEvent) (*larkcallback.CardActionTriggerResponse, error) {
		action, ok := CardActionPayloadFromSDK(event)
		if !ok {
			return &larkcallback.CardActionTriggerResponse{}, nil
		}
		ingress, ok := handler.(CardActionHandler)
		if !ok {
			lg.Warn("飞书卡片动作处理器未就绪",
				loggateway.StepID("channel.feishu.ws.card_handler_unavailable"),
				loggateway.Str("channel_id", chRow.ID),
			)
			return cardActionSDKResponse(NewCardActionToast("服务未就绪")), nil
		}
		resp := ingress.HandleFeishuCardAction(context.WithoutCancel(ctx), chRow, action)
		return cardActionSDKResponse(resp), nil
	}
	eventHandler := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(onMessage).
		OnP2CardActionTrigger(onCardAction)
	// flowGuard 收敛 SDK 内部无限重连期间的重复发射：每个故障期一条
	// connect.error，每次（重新）建立一条 connect.open，正常停止一条 connect.close。
	flowGuard := &runtime.ConnectFlowGuard{}
	cli := larkws.NewClient(appID, appSecret,
		larkws.WithEventHandler(eventHandler),
		larkws.WithOnReady(func() {
			flowGuard.EmitOpen(func() {
				lg.Info("飞书 WebSocket 已连接",
					loggateway.StepID("channel.feishu.ws.connected"),
					loggateway.Str("channel_id", ch.ID),
				)
				runtime.EmitConnectOpen(ctx, "feishu", ch.ID, appID, "飞书 WebSocket 已连接")
			})
		}),
		larkws.WithOnReconnected(func() {
			flowGuard.EmitOpen(func() {
				lg.Info("飞书 WebSocket 重连成功",
					loggateway.StepID("channel.feishu.ws.reconnected"),
					loggateway.Str("channel_id", ch.ID),
				)
				runtime.EmitConnectOpen(ctx, "feishu", ch.ID, appID, "飞书 WebSocket 重连成功", event.P("reconnect", true))
			})
		}),
		larkws.WithOnError(func(err error) {
			flowGuard.EmitError(func() {
				runtime.EmitConnectError(ctx, "feishu", ch.ID, "飞书 WebSocket 连接异常", err)
			})
		}),
	)
	// lark SDK 的 Start 在 ctx 取消后仍永久阻塞（select{} 不退出的 SDK 限制），
	// 连接断开流程日志无法由 supervisor 收口，这里监听 ctx 补发 close。
	safego.Go(ctx, "channel.feishu.ws.close_watch", func() {
		<-ctx.Done()
		flowGuard.EmitClose(func() {
			lg.Info("飞书 WebSocket 连接已断开",
				loggateway.StepID("channel.feishu.ws.close"),
				loggateway.Str("channel_id", ch.ID),
			)
			runtime.EmitConnectClose(ctx, "feishu", ch.ID, "飞书 WebSocket 连接已断开")
		})
	})
	return cli.Start(ctx)
}

func cardActionSDKResponse(httpResp *CardActionHTTPResponse) *larkcallback.CardActionTriggerResponse {
	if httpResp == nil || httpResp.Toast == nil {
		return &larkcallback.CardActionTriggerResponse{}
	}
	return &larkcallback.CardActionTriggerResponse{
		Toast: &larkcallback.Toast{
			Type:    httpResp.Toast.Type,
			Content: httpResp.Toast.Content,
		},
	}
}
