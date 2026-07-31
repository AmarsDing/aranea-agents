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
		newErr := discordGatewayError("discord gateway: new session", err)
		runtime.EmitConnectError(ctx, "discord", ch.ID, "Discord Gateway 会话创建失败", newErr)
		return newErr
	}

	chRow := ch
	// discordgo 在内部自动重连，每次 WS（重新）建立都会派发 Connect 事件；
	// guard 保证每次建立一条 open、每个故障期一条 error。
	flowGuard := &runtime.ConnectFlowGuard{}
	session.AddHandler(func(_ *discordgo.Session, _ *discordgo.Connect) {
		flowGuard.EmitOpen(func() {
			lg.Info("Discord Gateway 已连接",
				loggateway.StepID("channel.discord.gateway.connected"),
				loggateway.Str("channel_id", ch.ID),
			)
			runtime.EmitConnectOpen(ctx, "discord", ch.ID, "", "Discord Gateway 已连接")
		})
	})
	session.AddHandler(func(_ *discordgo.Session, _ *discordgo.Disconnect) {
		flowGuard.EmitError(func() {
			runtime.EmitConnectError(ctx, "discord", ch.ID, "Discord Gateway 连接异常断开", nil)
		})
	})
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
		openErr := discordGatewayError("discord gateway: open", err)
		flowGuard.EmitError(func() {
			runtime.EmitConnectError(ctx, "discord", ch.ID, "Discord Gateway 连接失败", openErr)
		})
		return openErr
	}
	defer session.Close()

	<-ctx.Done()
	return ctx.Err()
}
