package lark

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/runtime"
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
			batcher.Submit(ctx, wsInboundBridge{ctx: ctx, ch: chRow, creds: creds, lookup: lookup, handler: handler, lg: lg}, chRow, ev)
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
	cli := larkws.NewClient(appID, appSecret, larkws.WithEventHandler(eventHandler))
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
