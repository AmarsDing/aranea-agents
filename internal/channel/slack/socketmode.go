package slack

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/runtime"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

func init() {
	runtime.RegisterStarterWithLogger("slack", "socket_mode", RunSocketMode)
}

func RunSocketMode(
	ctx context.Context,
	ch biz.Channel,
	creds []biz.ChannelCredential,
	lookup runtime.CredentialLookup,
	handler port.InboundHandler,
	lg loggateway.Logger,
) error {
	lg.Info("Slack Socket Mode 连接器启动",
		loggateway.StepID("channel.slack.socketmode.start"),
		loggateway.Str("channel_id", ch.ID),
	)
	botToken, err := lookup(ctx, creds, "bot_token")
	if err != nil {
		lg.Error("Slack 凭据获取失败",
			loggateway.StepID("channel.slack.socketmode.creds_fail"),
			loggateway.Str("channel_id", ch.ID),
			loggateway.Err(err),
		)
		return err
	}
	appToken, err := lookup(ctx, creds, "app_token")
	if err != nil {
		lg.Error("Slack 凭据获取失败",
			loggateway.StepID("channel.slack.socketmode.creds_fail"),
			loggateway.Str("channel_id", ch.ID),
			loggateway.Err(err),
		)
		return err
	}
	botToken = strings.TrimSpace(botToken)
	appToken = strings.TrimSpace(appToken)
	if botToken == "" || appToken == "" {
		return errBotTokenAndAppRequired
	}

	client := slack.New(
		botToken,
		slack.OptionAppLevelToken(appToken),
	)
	socketClient := socketmode.New(client)
	chRow := ch

	safego.Go(ctx, "channel.slack.socketmode", func() {
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-socketClient.Events:
				if !ok {
					return
				}
				switch evt.Type {
				case socketmode.EventTypeEventsAPI:
					socketClient.Ack(*evt.Request)
					inner, ok := evt.Data.(slackevents.EventsAPIEvent)
					if !ok || inner.Type != slackevents.CallbackEvent {
						continue
					}
					msgEv, ok := inner.InnerEvent.Data.(*slackevents.MessageEvent)
					if !ok || strings.TrimSpace(msgEv.BotID) != "" {
						continue
					}
					ev, ok := messageEventToInbound(msgEv)
					if !ok {
						continue
					}
					ev.PlatformType = "slack"
					if err := handler.ProcessInbound(ctx, chRow, ev); err != nil {
						lg.Warn("Slack 入站处理失败",
							loggateway.StepID("channel.slack.inbound_failed"),
							loggateway.Err(err),
						)
					}
				}
			}
		}
	})

	return socketClient.RunContext(ctx)
}

func messageEventToInbound(ev *slackevents.MessageEvent) (port.InboundEvent, bool) {
	if ev == nil {
		return port.InboundEvent{}, false
	}
	text := strings.TrimSpace(ev.Text)
	if text == "" {
		return port.InboundEvent{}, false
	}
	channelID := strings.TrimSpace(ev.Channel)
	userID := strings.TrimSpace(ev.User)
	return port.InboundEvent{
		PeerID:         port.FirstNonEmpty(userID, channelID),
		Text:           text,
		IdempotencyKey: "slack:" + strings.TrimSpace(ev.TimeStamp),
		OutboundMeta: map[string]string{
			port.MetaRecipient: channelID,
			port.MetaChannel:   channelID,
		},
	}, true
}
