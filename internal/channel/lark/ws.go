package lark

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/runtime"
	"aranea-agents/pkg/safego"

	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkcallback "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

func init() {
	runtime.RegisterStarter("feishu", "websocket", RunWebSocket)
}

// RunWebSocket uses Lark long connection (MuseBot StartLarkRobot).
func RunWebSocket(
	ctx context.Context,
	ch biz.Channel,
	creds []biz.ChannelCredential,
	lookup runtime.CredentialLookup,
	handler runtime.InboundHandler,
) error {
	appID, appSecret, err := WSAppCredentials(ctx, ch, creds, lookup)
	if err != nil {
		return err
	}
	chRow := ch
	onMessage := func(_ context.Context, message *larkim.P2MessageReceiveV1) error {
		ev, ok := InboundEventFromWSMessage(message)
		if !ok {
			return nil // filtered by AcceptFeishuInbound (non-user, no message_id, group w/o @, etc.)
		}
		ev.PlatformType = "feishu"
		// Feishu event ctx may cancel when the handler returns; process async like MuseBot.
		safego.Go(ctx, "channel.feishu.ws.inbound", func() {
			HandleWSInbound(ctx, chRow, creds, lookup, handler, ev)
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
