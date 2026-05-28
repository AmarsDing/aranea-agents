package discord

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"aranea-agents/pkg/safego"

	"github.com/bwmarrin/discordgo"
)

type TextSender struct {
	BotToken string

	mu      sync.Mutex
	session *discordgo.Session
}

func (s *TextSender) ID() string { return "discord" }

func (s *TextSender) SendText(ctx context.Context, channelID, text string) error {
	channelID = strings.TrimSpace(channelID)
	text = strings.TrimSpace(text)
	if channelID == "" || text == "" {
		return nil
	}
	token := strings.TrimSpace(s.BotToken)
	if token == "" {
		return fmt.Errorf("discord outbound: bot_token required")
	}
	sess, err := s.sessionOnce(token)
	if err != nil {
		return err
	}
	result := make(chan error, 1)
	safego.Go(ctx, "channel.discord.outbound.send", func() {
		_, sendErr := sess.ChannelMessageSend(channelID, text)
		result <- sendErr
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case sendErr := <-result:
		return sendErr
	}
}

func (s *TextSender) sessionOnce(token string) (*discordgo.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session != nil {
		return s.session, nil
	}
	sess, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}
	if err := sess.Open(); err != nil {
		return nil, fmt.Errorf("discord outbound: open session: %w", err)
	}
	s.session = sess
	return sess, nil
}

func (s *TextSender) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil {
		return nil
	}
	err := s.session.Close()
	s.session = nil
	return err
}
