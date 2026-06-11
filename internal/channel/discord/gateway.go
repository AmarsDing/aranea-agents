package discord

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/runtime"
	"aranea-agents/pkg/loggateway"

	"github.com/bwmarrin/discordgo"
)

func init() {
	runtime.RegisterStarterWithLogger("discord", "gateway", RunGateway)
}

func RunGateway(
	ctx context.Context,
	ch biz.Channel,
	creds []biz.ChannelCredential,
	lookup runtime.CredentialLookup,
	handler port.InboundHandler,
	lg loggateway.Logger,
) error {
	lg.Info("Discord Gateway 连接器启动",
		loggateway.StepID("channel.discord.gateway.start"),
		loggateway.Str("channel_id", ch.ID),
	)
	token, err := lookup(ctx, creds, "bot_token")
	if err != nil {
		lg.Error("Discord 凭据获取失败",
			loggateway.StepID("channel.discord.gateway.creds_fail"),
			loggateway.Str("channel_id", ch.ID),
			loggateway.Err(err),
		)
		return err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return errBotTokenRequired
	}

	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return discordGatewayError("discord gateway: new session", err)
	}

	chRow := ch
	session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m == nil || m.Author == nil || m.Author.Bot {
			return
		}
		text := strings.TrimSpace(m.Content)
		if text == "" {
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
		channelID := m.ChannelID
		userID := m.Author.ID
		ev := port.InboundEvent{
			PlatformType:   "discord",
			PeerID:         port.FirstNonEmpty(userID, channelID),
			Text:           text,
			IdempotencyKey: "discord:" + m.ID,
			OutboundMeta: map[string]string{
				port.MetaRecipient: channelID,
				port.MetaChannel:   channelID,
			},
		}
		if err := handler.ProcessInbound(ctx, chRow, ev); err != nil {
			lg.Warn("Discord 入站处理失败",
				loggateway.StepID("channel.discord.inbound_failed"),
				loggateway.Err(err),
			)
		}
	})

	if err := session.Open(); err != nil {
		return discordGatewayError("discord gateway: open", err)
	}
	defer session.Close()

	<-ctx.Done()
	return ctx.Err()
}
