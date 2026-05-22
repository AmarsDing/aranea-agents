package discord

import (
	"context"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// TextSender delivers plain text via discordgo.
type TextSender struct {
	BotToken string
}

// ID implements channel.Identified.
func (s *TextSender) ID() string { return "discord" }

// SendText posts a message to a Discord channel ID.
func (s *TextSender) SendText(ctx context.Context, channelID, text string) error {
	_ = ctx
	channelID = strings.TrimSpace(channelID)
	text = strings.TrimSpace(text)
	if channelID == "" || text == "" {
		return nil
	}
	token := strings.TrimSpace(s.BotToken)
	if token == "" {
		return fmt.Errorf("discord outbound: bot_token required")
	}
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return err
	}
	defer session.Close()
	_, err = session.ChannelMessageSend(channelID, text)
	return err
}
