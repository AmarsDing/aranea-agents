package line

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"

	"aranea-agents/internal/channel/port"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type InboundMessage struct {
	Text       string
	UserID     string
	GroupID    string
	RoomID     string
	ReplyToken string
	MessageID  string
}

type webhookPayload struct {
	Events []webhookEvent `json:"events"`
}

type webhookEvent struct {
	Type       string          `json:"type"`
	ReplyToken string          `json:"replyToken"`
	Source     eventSource     `json:"source"`
	Message    json.RawMessage `json:"message"`
	Timestamp  int64           `json:"timestamp"`
}

type eventSource struct {
	Type    string `json:"type"`
	UserID  string `json:"userId"`
	GroupID string `json:"groupId"`
	RoomID  string `json:"roomId"`
}

type textMessageContent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Text string `json:"text"`
}

func ParseInbound(raw []byte) ([]InboundMessage, error) {
	var payload webhookPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	var results []InboundMessage
	for _, evt := range payload.Events {
		if strings.TrimSpace(strings.ToLower(evt.Type)) != "message" {
			continue
		}
		var content textMessageContent
		if err := json.Unmarshal(evt.Message, &content); err != nil {
			continue
		}
		if strings.TrimSpace(strings.ToLower(content.Type)) != "text" {
			continue
		}
		text := strings.TrimSpace(content.Text)
		if text == "" {
			continue
		}
		results = append(results, InboundMessage{
			Text:       text,
			UserID:     strings.TrimSpace(evt.Source.UserID),
			GroupID:    strings.TrimSpace(evt.Source.GroupID),
			RoomID:     strings.TrimSpace(evt.Source.RoomID),
			ReplyToken: strings.TrimSpace(evt.ReplyToken),
			MessageID:  strings.TrimSpace(content.ID),
		})
	}
	if len(results) == 0 {
		return nil, kerrors.BadRequest("LINE_PROTOCOL", "line: no text messages in payload")
	}
	return results, nil
}

func VerifySignature(channelSecret string, requestBody []byte, signature string) error {
	channelSecret = strings.TrimSpace(channelSecret)
	if channelSecret == "" {
		return port.ErrCredentialsNotConfigured
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(signature))
	if err != nil {
		return kerrors.BadRequest("LINE_PROTOCOL", "line: bad signature encoding")
	}
	mac := hmac.New(sha256.New, []byte(channelSecret))
	_, _ = mac.Write(requestBody)
	if !hmac.Equal(mac.Sum(nil), decoded) {
		return kerrors.BadRequest("LINE_PROTOCOL", "line: bad signature")
	}
	return nil
}
