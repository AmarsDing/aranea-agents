package telegram

import (
	"crypto/hmac"
	"encoding/json"
	"strings"

	"aranea-agents/internal/channel/port"
)

// InboundMessage is a normalized Telegram Bot API update.
type InboundMessage struct {
	UpdateID  int64
	MessageID int64
	ChatID    int64
	Text      string
	Username  string
}

type updateEnvelope struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		MessageID int64 `json:"message_id"`
		Text      string `json:"text"`
		From      *struct {
			IsBot    bool   `json:"is_bot"`
			Username string `json:"username"`
		} `json:"from"`
		Chat *struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	} `json:"message"`
}

// ParseInbound decodes a Telegram webhook update with a text message.
func ParseInbound(raw []byte) (InboundMessage, error) {
	var top updateEnvelope
	if err := json.Unmarshal(raw, &top); err != nil {
		return InboundMessage{}, err
	}
	if top.Message == nil {
		return InboundMessage{}, errNoMessage
	}
	if top.Message.From != nil && top.Message.From.IsBot {
		return InboundMessage{}, errBotMessageIgnored
	}
	text := strings.TrimSpace(top.Message.Text)
	if text == "" {
		return InboundMessage{}, errEmptyText
	}
	if top.Message.Chat == nil || top.Message.Chat.ID == 0 {
		return InboundMessage{}, errMissingChatID
	}
	username := ""
	if top.Message.From != nil {
		username = strings.TrimSpace(top.Message.From.Username)
	}
	return InboundMessage{
		UpdateID:  top.UpdateID,
		MessageID: top.Message.MessageID,
		ChatID:    top.Message.Chat.ID,
		Text:      text,
		Username:  username,
	}, nil
}

// VerifySecretToken compares optional Telegram secret header with configured value.
func VerifySecretToken(headerValue, configured string) error {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return port.ErrCredentialsNotConfigured
	}
	if !hmac.Equal([]byte(strings.TrimSpace(headerValue)), []byte(configured)) {
		return errSecretTokenMismatch
	}
	return nil
}
