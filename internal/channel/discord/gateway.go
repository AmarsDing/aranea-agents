package discord

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/runtime"

	"github.com/bwmarrin/discordgo"
)

func init() {
	runtime.RegisterStarter("discord", "gateway", RunGateway)
}

// RunGateway connects via discordgo Gateway (MuseBot StartDiscordRobot).
func RunGateway(
	ctx context.Context,
	ch biz.Channel,
	creds []biz.ChannelCredential,
	lookup runtime.CredentialLookup,
	handler runtime.InboundHandler,
) error {
	token, err := lookup(ctx, creds, "bot_token")
	if err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("discord gateway: bot_token required")
	}

	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return fmt.Errorf("discord gateway: new session: %w", err)
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
			PeerID:         firstNonEmpty(userID, channelID),
			Text:           text,
			IdempotencyKey: "discord:" + m.ID,
			OutboundMeta: map[string]string{
				"recipient": channelID,
				"channel":   channelID,
			},
		}
		_ = handler.ProcessInbound(ctx, chRow, ev)
	})

	if err := session.Open(); err != nil {
		return fmt.Errorf("discord gateway: open: %w", err)
	}
	defer session.Close()

	<-ctx.Done()
	return ctx.Err()
}

func firstNonEmpty(parts ...string) string {
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			return strings.TrimSpace(p)
		}
	}
	return ""
}
