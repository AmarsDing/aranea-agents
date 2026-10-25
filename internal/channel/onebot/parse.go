package onebot

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"strings"

	"aranea-agents/internal/channel/port"
)

// InboundMessage is a normalized OneBot HTTP push payload (NapCat/LLOneBot).
type InboundMessage struct {
	PeerID    string
	Text      string
	MessageID string
	GroupID   string
	UserID    string
}

type qqMessage struct {
	MessageType string        `json:"message_type"`
	MessageID   json.Number   `json:"message_id"`
	GroupID     json.Number   `json:"group_id"`
	UserID      json.Number   `json:"user_id"`
	Message     []messageItem `json:"message"`
	RawMessage  string        `json:"raw_message"`
	Sender      senderInfo    `json:"sender"`
}

type messageItem struct {
	Type string `json:"type"`
	Data struct {
		Text string `json:"text"`
	} `json:"data"`
}

type senderInfo struct {
	UserID   json.Number `json:"user_id"`
	Nickname string      `json:"nickname"`
}

// VerifySignature checks X-Signature HMAC-SHA1 (MuseBot OneBot handler).
func VerifySignature(receiveToken string, body []byte, signature string) error {
	receiveToken = strings.TrimSpace(receiveToken)
	if receiveToken == "" {
		return port.ErrCredentialsNotConfigured
	}
	mac := hmac.New(sha1.New, []byte(receiveToken))
	_, _ = mac.Write(body)
	expected := "sha1=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(strings.TrimSpace(signature))) {
		return errBadSignature
	}
	return nil
}

// ParseInbound extracts text from OneBot JSON push.
func ParseInbound(raw []byte) (*InboundMessage, error) {
	var msg qqMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, err
	}
	text := strings.TrimSpace(msg.RawMessage)
	if text == "" {
		for _, item := range msg.Message {
			if item.Type == "text" {
				text = strings.TrimSpace(item.Data.Text)
				break
			}
		}
	}
	if text == "" {
		return nil, errEmptyMessage
	}
	userID := port.FirstNonEmpty(msg.UserID.String(), msg.Sender.UserID.String())
	peerID := userID
	if strings.TrimSpace(msg.GroupID.String()) != "" {
		peerID = msg.GroupID.String()
	}
	return &InboundMessage{
		PeerID:    peerID,
		Text:      text,
		MessageID: strings.TrimSpace(msg.MessageID.String()),
		GroupID:   strings.TrimSpace(msg.GroupID.String()),
		UserID:    userID,
	}, nil
}
