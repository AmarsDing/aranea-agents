package teams

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type InboundMessage struct {
	Text           string
	FromID         string
	FromName       string
	ChannelID      string
	ConversationID string
	ServiceURL     string
	ActivityID     string
	RecipientID    string
}

type activity struct {
	Type         string          `json:"type"`
	ID           string          `json:"id"`
	Timestamp    string          `json:"timestamp"`
	ServiceURL   string          `json:"serviceUrl"`
	ChannelID    string          `json:"channelId"`
	From         channelAccount  `json:"from"`
	Conversation conversation    `json:"conversation"`
	Recipient    channelAccount  `json:"recipient"`
	Text         string          `json:"text"`
	TextFormat   string          `json:"textFormat"`
	ChannelData  json.RawMessage `json:"channelData"`
}

type channelAccount struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type conversation struct {
	ID               string `json:"id"`
	ConversationType string `json:"conversationType"`
}

func ParseInbound(raw []byte) (InboundMessage, error) {
	var act activity
	if err := json.Unmarshal(raw, &act); err != nil {
		return InboundMessage{}, err
	}
	if strings.TrimSpace(strings.ToLower(act.Type)) != "message" {
		return InboundMessage{}, fmt.Errorf("teams: unsupported activity type %q", act.Type)
	}
	text := strings.TrimSpace(act.Text)
	if text == "" {
		return InboundMessage{}, fmt.Errorf("teams: empty text")
	}
	return InboundMessage{
		Text:           text,
		FromID:         strings.TrimSpace(act.From.ID),
		FromName:       strings.TrimSpace(act.From.Name),
		ChannelID:      strings.TrimSpace(act.ChannelID),
		ConversationID: strings.TrimSpace(act.Conversation.ID),
		ServiceURL:     strings.TrimSpace(act.ServiceURL),
		ActivityID:     strings.TrimSpace(act.ID),
		RecipientID:    strings.TrimSpace(act.Recipient.ID),
	}, nil
}

func VerifyRequest(appID, appSecret string, header http.Header, body []byte) error {
	authHeader := strings.TrimSpace(header.Get("Authorization"))
	if authHeader == "" {
		return fmt.Errorf("teams: missing authorization header")
	}
	if appID == "" || appSecret == "" {
		return nil
	}
	return verifyBotFrameworkToken(authHeader, appID, appSecret)
}

func verifyBotFrameworkToken(authHeader, appID, appSecret string) error {
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return fmt.Errorf("teams: invalid authorization scheme")
	}
	return nil
}
