package onebot

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
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
	MessageType   string        `json:"message_type"`
	MessageID     string        `json:"message_id"`
	GroupID       string        `json:"group_id"`
	UserID        string        `json:"user_id"`
	Message       []messageItem `json:"message"`
	RawMessage    string        `json:"raw_message"`
	Sender        senderInfo    `json:"sender"`
}

type messageItem struct {
	Type string `json:"type"`
	Data struct {
		Text string `json:"text"`
	} `json:"data"`
}

type senderInfo struct {
	UserID   string `json:"user_id"`
	Nickname string `json:"nickname"`
}

// VerifySignature checks X-Signature HMAC-SHA1 (MuseBot OneBot handler).
func VerifySignature(receiveToken string, body []byte, signature string) error {
	receiveToken = strings.TrimSpace(receiveToken)
	if receiveToken == "" {
		return nil
	}
	mac := hmac.New(sha1.New, []byte(receiveToken))
	_, _ = mac.Write(body)
	expected := "sha1=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(strings.TrimSpace(signature))) {
		return fmt.Errorf("onebot: bad signature")
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
		return nil, fmt.Errorf("onebot: empty message")
	}
	userID := firstNonEmpty(msg.UserID, msg.Sender.UserID)
	peerID := userID
	if strings.TrimSpace(msg.GroupID) != "" {
		peerID = msg.GroupID
	}
	return &InboundMessage{
		PeerID:    peerID,
		Text:      text,
		MessageID: strings.TrimSpace(msg.MessageID),
		GroupID:   strings.TrimSpace(msg.GroupID),
		UserID:    userID,
	}, nil
}

func firstNonEmpty(parts ...string) string {
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			return strings.TrimSpace(p)
		}
	}
	return ""
}
