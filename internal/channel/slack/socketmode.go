package slack

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/runtime"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

func init() {
	runtime.RegisterStarter("slack", "socket_mode", RunSocketMode)
}

// RunSocketMode listens via Slack Socket Mode (MuseBot StartSlackRobot).
func RunSocketMode(
	ctx context.Context,
	ch biz.Channel,
	creds []biz.ChannelCredential,
	lookup runtime.CredentialLookup,
	handler port.InboundHandler,
) error {
	botToken, err := lookup(ctx, creds, "bot_token")
	if err != nil {
		return err
	}
	appToken, err := lookup(ctx, creds, "app_token")
	if err != nil {
		return err
	}
	botToken = strings.TrimSpace(botToken)
	appToken = strings.TrimSpace(appToken)
	if botToken == "" || appToken == "" {
		return fmt.Errorf("slack socket_mode: bot_token and app_token required")
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
						event.SysLogWarn("channel.slack.inbound_failed", "Slack 入站处理失败",
							event.P("error", err.Error()),
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
			"recipient": channelID,
			"channel":   channelID,
		},
	}, true
}
