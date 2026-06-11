package mattermost

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type TextSender struct {
	ServerURL string
	BotToken  string
	HTTP      *http.Client
}

func (s *TextSender) ID() string { return "mattermost" }

func (s *TextSender) SendText(ctx context.Context, channelID, text string) error {
	channelID = strings.TrimSpace(channelID)
	text = strings.TrimSpace(text)
	if channelID == "" {
		return errChannelIDRequired
	}
	if text == "" {
		return nil
	}
	token := strings.TrimSpace(s.BotToken)
	if token == "" {
		return errBotTokenRequired
	}
	base := strings.TrimRight(strings.TrimSpace(s.ServerURL), "/")
	if base == "" {
		return errServerURLRequired
	}
	body, _ := json.Marshal(map[string]any{
		"channel_id": channelID,
		"message":    text,
	})
	_, err := doPost(ctx, s.HTTP, token, base+"/api/v4/posts", body)
	return err
}
