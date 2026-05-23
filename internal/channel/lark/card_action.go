package lark

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"

	larkcallback "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
)

const CardActionBackground = "background"
const CardActionCancel = "cancel"

// CardActionPayload is a normalized Feishu card.action.trigger callback.
type CardActionPayload struct {
	Action         string
	SessionRunID   string
	SessionID      string
	OperatorOpenID string
	OpenChatID     string
	OpenMessageID  string
}

// CardActionHTTPResponse is returned to Feishu for card.action.trigger over HTTP.
type CardActionHTTPResponse struct {
	Toast *CardActionToast `json:"toast,omitempty"`
}

type CardActionToast struct {
	Type    string `json:"type,omitempty"`
	Content string `json:"content,omitempty"`
}

// CardActionPayloadFromWebhook extracts card action fields from ParseWebhookPost result.
func CardActionPayloadFromWebhook(res *WebhookParseResult) (CardActionPayload, bool) {
	if res == nil {
		return CardActionPayload{}, false
	}
	switch res.EventType {
	case "card.action.trigger", "card.action.trigger_v1":
	default:
		return CardActionPayload{}, false
	}
	action := strings.TrimSpace(stringFromAny(res.CardActionValue["action"]))
	sessionRunID := strings.TrimSpace(stringFromAny(res.CardActionValue["session_run_id"]))
	sessionID := strings.TrimSpace(stringFromAny(res.CardActionValue["session_id"]))
	if action == "" && sessionRunID == "" {
		return CardActionPayload{}, false
	}
	return CardActionPayload{
		Action:         action,
		SessionRunID:   sessionRunID,
		SessionID:      sessionID,
		OperatorOpenID: strings.TrimSpace(res.CardActionOperatorOpenID),
		OpenChatID:     strings.TrimSpace(res.CardOpenChatID),
		OpenMessageID:  strings.TrimSpace(res.CardOpenMessageID),
	}, true
}

// CardActionPayloadFromSDK converts SDK card.action.trigger event.
func CardActionPayloadFromSDK(ev *larkcallback.CardActionTriggerEvent) (CardActionPayload, bool) {
	if ev == nil || ev.Event == nil || ev.Event.Action == nil {
		return CardActionPayload{}, false
	}
	val := ev.Event.Action.Value
	if val == nil {
		return CardActionPayload{}, false
	}
	action := strings.TrimSpace(stringFromAny(val["action"]))
	sessionRunID := strings.TrimSpace(stringFromAny(val["session_run_id"]))
	sessionID := strings.TrimSpace(stringFromAny(val["session_id"]))
	if action == "" && sessionRunID == "" {
		return CardActionPayload{}, false
	}
	out := CardActionPayload{
		Action:       action,
		SessionRunID: sessionRunID,
		SessionID:    sessionID,
	}
	if ev.Event.Operator != nil {
		out.OperatorOpenID = strings.TrimSpace(ev.Event.Operator.OpenID)
	}
	if ev.Event.Context != nil {
		out.OpenChatID = strings.TrimSpace(ev.Event.Context.OpenChatID)
		out.OpenMessageID = strings.TrimSpace(ev.Event.Context.OpenMessageID)
	}
	return out, true
}

func stringFromAny(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	default:
		return ""
	}
}

func NewCardActionToast(content string) *CardActionHTTPResponse {
	content = strings.TrimSpace(content)
	if content == "" {
		return &CardActionHTTPResponse{}
	}
	return &CardActionHTTPResponse{
		Toast: &CardActionToast{Type: "info", Content: content},
	}
}

// CardActionHandler handles card.action.trigger for session run escalation.
type CardActionHandler interface {
	HandleFeishuCardAction(ctx context.Context, ch biz.Channel, action CardActionPayload) *CardActionHTTPResponse
}
