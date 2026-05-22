package lark

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/runtime"
	"aranea-agents/pkg/safego"

	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
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
			return nil
		}
		ev.PlatformType = "feishu"
		// Feishu event ctx may cancel when the handler returns; process async like MuseBot.
		safego.Go(ctx, "channel.feishu.ws.inbound", func() {
			HandleWSInbound(ctx, chRow, creds, lookup, handler, ev)
		})
		return nil
	}
	eventHandler := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(onMessage)
	cli := larkws.NewClient(appID, appSecret, larkws.WithEventHandler(eventHandler))
	return cli.Start(ctx)
}
