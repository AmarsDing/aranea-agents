package slack

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/channel/port"
)

// InboundMessage is a normalized Slack event callback message.
type InboundMessage struct {
	Text      string
	UserID    string
	ChannelID string
	EventTS   string
	TeamID    string
}

type envelope struct {
	Type      string          `json:"type"`
	Challenge string          `json:"challenge"`
	Event     json.RawMessage `json:"event"`
}

type messageEvent struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	Text    string `json:"text"`
	User    string `json:"user"`
	Channel string `json:"channel"`
	BotID   string `json:"bot_id"`
	TS      string `json:"ts"`
	Team    string `json:"team"`
}

// ParseInbound decodes Slack url_verification or event_callback payloads.
func ParseInbound(raw []byte) (challenge string, msg *InboundMessage, err error) {
	var top envelope
	if err := json.Unmarshal(raw, &top); err != nil {
		return "", nil, err
	}
	switch strings.TrimSpace(top.Type) {
	case "url_verification":
		ch := strings.TrimSpace(top.Challenge)
		if ch == "" {
			return "", nil, errEmptyChallenge
		}
		return ch, nil, nil
	case "event_callback":
		var ev messageEvent
		if err := json.Unmarshal(top.Event, &ev); err != nil {
			return "", nil, err
		}
		if strings.TrimSpace(ev.Type) != "message" {
			return "", nil, errUnsupportedEventType
		}
		if strings.TrimSpace(ev.Subtype) != "" || strings.TrimSpace(ev.BotID) != "" {
			return "", nil, errIgnoredMessageSubtype
		}
		text := strings.TrimSpace(ev.Text)
		if text == "" {
			return "", nil, errEmptyMessage
		}
		return "", &InboundMessage{
			Text:      text,
			UserID:    strings.TrimSpace(ev.User),
			ChannelID: strings.TrimSpace(ev.Channel),
			EventTS:   strings.TrimSpace(ev.TS),
			TeamID:    strings.TrimSpace(ev.Team),
		}, nil
	default:
		return "", nil, errUnsupportedPayloadType
	}
}

// VerifyRequest validates Slack signing headers when signingSecret is configured.
func VerifyRequest(timestamp, signature, signingSecret string, rawBody []byte) error {
	signingSecret = strings.TrimSpace(signingSecret)
	if signingSecret == "" {
		return port.ErrCredentialsNotConfigured
	}
	ts := strings.TrimSpace(timestamp)
	sig := strings.TrimSpace(signature)
	if ts == "" || sig == "" {
		return errMissingSignature
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return errBadTimestamp
	}
	now := time.Now().Unix()
	if sec < now-port.WebhookTimestampToleranceSec || sec > now+port.WebhookTimestampToleranceSec {
		return errTimestampOutOfRange
	}
	base := "v0:" + ts + ":" + string(rawBody)
	mac := hmac.New(sha256.New, []byte(signingSecret))
	_, _ = mac.Write([]byte(base))
	want := "v0=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(sig)) {
		return errBadSignature
	}
	return nil
}
