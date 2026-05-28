package line

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

type TextSender struct {
	ChannelSecret string
	ChannelToken  string
	HTTP          *http.Client
}

func (s *TextSender) ID() string { return "line" }

func (s *TextSender) SendText(ctx context.Context, recipient, text string) error {
	recipient = strings.TrimSpace(recipient)
	text = strings.TrimSpace(text)
	if recipient == "" || text == "" {
		return nil
	}
	token := strings.TrimSpace(s.ChannelToken)
	if token == "" {
		return fmt.Errorf("line outbound: channel_token required")
	}
	body, _ := marshalMessages(recipient, []map[string]any{textMessage(text)})
	_, err := doPost(ctx, s.HTTP, token, "https://api.line.me/v2/bot/message/push", body)
	return err
}

func (s *TextSender) ReplyText(ctx context.Context, replyToken, text string) error {
	replyToken = strings.TrimSpace(replyToken)
	text = strings.TrimSpace(text)
	if replyToken == "" || text == "" {
		return nil
	}
	token := strings.TrimSpace(s.ChannelToken)
	if token == "" {
		return fmt.Errorf("line reply: channel_token required")
	}
	payload := map[string]any{
		"replyToken": replyToken,
		"messages":   []map[string]any{textMessage(text)},
	}
	body, _ := marshalJSON(payload)
	_, err := doPost(ctx, s.HTTP, token, "https://api.line.me/v2/bot/message/reply", body)
	return err
}
