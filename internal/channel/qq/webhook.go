package qq

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/tencent-connect/botgo/dto"
	"github.com/tencent-connect/botgo/interaction/signature"
	qqwebhook "github.com/tencent-connect/botgo/interaction/webhook"
)

// InboundMessage is a normalized QQ official bot text event.
type InboundMessage struct {
	Text      string
	UserID    string
	GroupID   string
	PeerID    string
	MessageID string
	EventID   string
}

// WebhookResult is the outcome of parsing a QQ callback payload.
type WebhookResult struct {
	ValidationBody []byte
	HeartbeatBody  string
	DispatchACK    string
	Message        *InboundMessage
}

// VerifyRequest validates Ed25519 webhook signature (QQ Bot API v2).
func VerifyRequest(appSecret string, header http.Header, body []byte) error {
	appSecret = strings.TrimSpace(appSecret)
	if appSecret == "" {
		return nil
	}
	ok, err := signature.Verify(appSecret, header, body)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("qq: bad signature")
	}
	return nil
}

// ParseWebhook decodes QQ HTTP callback payloads.
func ParseWebhook(body []byte, header http.Header, appSecret string) (*WebhookResult, error) {
	var payload dto.WSPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	res := &WebhookResult{}
	switch payload.OPCode {
	case dto.HTTPCallbackValidation:
		data, ok := payload.Data.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("qq: invalid validation payload")
		}
		plainToken, _ := data["plain_token"].(string)
		eventTs, _ := data["event_ts"].(string)
		if plainToken == "" || eventTs == "" {
			return nil, fmt.Errorf("qq: missing validation fields")
		}
		req := &dto.WHValidationReq{PlainToken: plainToken, EventTs: eventTs}
		res.ValidationBody = qqwebhook.GenValidationACK(req, header, appSecret)
		return res, nil
	case dto.WSHeartbeat:
		seq, _ := payload.Data.(float64)
		res.HeartbeatBody = qqwebhook.GenHeartbeatACK(uint32(seq))
		return res, nil
	case dto.WSDispatchEvent:
		msg, err := parseDispatch(&payload)
		if err != nil {
			res.DispatchACK = qqwebhook.GenDispatchACK(false)
			return res, err
		}
		res.DispatchACK = qqwebhook.GenDispatchACK(true)
		res.Message = msg
		return res, nil
	default:
		return res, nil
	}
}

func parseDispatch(payload *dto.WSPayload) (*InboundMessage, error) {
	raw, err := json.Marshal(payload.Data)
	if err != nil {
		return nil, err
	}
	switch payload.Type {
	case dto.EventC2CMessageCreate:
		var msg dto.WSC2CMessageData
		if err := json.Unmarshal(raw, &msg); err != nil {
			return nil, err
		}
		return messageFromData(dto.Message(msg), payload.EventID)
	case dto.EventGroupAtMessageCreate:
		var msg dto.WSGroupATMessageData
		if err := json.Unmarshal(raw, &msg); err != nil {
			return nil, err
		}
		return messageFromData(dto.Message(msg), payload.EventID)
	default:
		return nil, fmt.Errorf("qq: unsupported event %s", payload.Type)
	}
}

func messageFromData(msg dto.Message, eventID string) (*InboundMessage, error) {
	text := strings.TrimSpace(msg.Content)
	if text == "" {
		return nil, fmt.Errorf("qq: empty content")
	}
	userID := ""
	if msg.Author != nil {
		userID = strings.TrimSpace(msg.Author.ID)
	}
	groupID := strings.TrimSpace(msg.GroupID)
	peerID := userID
	if groupID != "" {
		peerID = groupID
	}
	return &InboundMessage{
		Text:      text,
		UserID:    userID,
		GroupID:   groupID,
		PeerID:    peerID,
		MessageID: strings.TrimSpace(msg.ID),
		EventID:   strings.TrimSpace(eventID),
	}, nil
}
