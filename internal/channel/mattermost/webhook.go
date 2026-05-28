package mattermost

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

type InboundMessage struct {
	Text        string
	UserID      string
	ChannelID   string
	TeamID      string
	PostID      string
	TriggerWord string
}

type webhookPayload struct {
	Token       string `json:"token"`
	TeamID      string `json:"team_id"`
	TeamDomain  string `json:"team_domain"`
	ChannelID   string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	UserID      string `json:"user_id"`
	UserName    string `json:"user_name"`
	PostID      string `json:"post_id"`
	Text        string `json:"text"`
	TriggerWord string `json:"trigger_word"`
	FileIDs     string `json:"file_ids"`
	Timestamp   int64  `json:"timestamp"`
}

func ParseInbound(raw []byte) (InboundMessage, error) {
	var payload webhookPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return InboundMessage{}, err
	}
	text := strings.TrimSpace(payload.Text)
	if text == "" {
		return InboundMessage{}, fmt.Errorf("mattermost: empty text")
	}
	return InboundMessage{
		Text:        text,
		UserID:      strings.TrimSpace(payload.UserID),
		ChannelID:   strings.TrimSpace(payload.ChannelID),
		TeamID:      strings.TrimSpace(payload.TeamID),
		PostID:      strings.TrimSpace(payload.PostID),
		TriggerWord: strings.TrimSpace(payload.TriggerWord),
	}, nil
}

func VerifyToken(receiveToken, payloadToken string) error {
	receiveToken = strings.TrimSpace(receiveToken)
	if receiveToken == "" {
		return nil
	}
	if strings.TrimSpace(payloadToken) != receiveToken {
		return fmt.Errorf("mattermost: bad token")
	}
	return nil
}

func VerifySignature(signingSecret string, body []byte, signature string) error {
	signingSecret = strings.TrimSpace(signingSecret)
	if signingSecret == "" {
		return nil
	}
	sig := strings.TrimSpace(signature)
	if sig == "" {
		return fmt.Errorf("mattermost: missing signature")
	}
	mac := hmac.New(sha256.New, []byte(signingSecret))
	_, _ = mac.Write(body)
	want := "v1=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(sig)) {
		return fmt.Errorf("mattermost: bad signature")
	}
	return nil
}
