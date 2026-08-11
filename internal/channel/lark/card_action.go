package lark

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"aranea-agents/internal/biz"

	larkcallback "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
)

const CardActionBackground = "background"
const CardActionCancel = "cancel"

// Gate card actions (CC: 渠道交互门): 确认卡片与澄清卡片的按钮回调。
const CardActionGateConfirm = "gate_confirm"
const CardActionGateClarify = "gate_clarify"

// CardActionPayload is a normalized Feishu card.action.trigger callback.
type CardActionPayload struct {
	Action         string
	SessionRunID   string
	SessionID      string
	OperatorOpenID string
	OpenChatID     string
	OpenMessageID  string
	// StepID 是 gate 卡片回调的目标 step（confirm/clarify）。
	StepID string
	// Reply 是确认卡片的短回复键（approve/deny/approve_session/approve_always），
	// 由 service 层映射为 serviceawaitreply 结构化 token。
	Reply string
	// QuestionIndex 是澄清卡片按钮对应的问题下标（-1 表示非逐题选择）。
	QuestionIndex int
	// Option 是澄清卡片按钮对应的选项文本（按原文回传，提交时原样进 Selected）。
	Option string
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
	stepID := strings.TrimSpace(stringFromAny(res.CardActionValue["step_id"]))
	if action == "" && sessionRunID == "" && stepID == "" {
		return CardActionPayload{}, false
	}
	return CardActionPayload{
		Action:         action,
		SessionRunID:   sessionRunID,
		SessionID:      sessionID,
		OperatorOpenID: strings.TrimSpace(res.CardActionOperatorOpenID),
		OpenChatID:     strings.TrimSpace(res.CardOpenChatID),
		OpenMessageID:  strings.TrimSpace(res.CardOpenMessageID),
		StepID:         stepID,
		Reply:          strings.TrimSpace(stringFromAny(res.CardActionValue["reply"])),
		QuestionIndex:  intFromAny(res.CardActionValue["q"], -1),
		Option:         stringFromAny(res.CardActionValue["opt"]),
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
	stepID := strings.TrimSpace(stringFromAny(val["step_id"]))
	if action == "" && sessionRunID == "" && stepID == "" {
		return CardActionPayload{}, false
	}
	out := CardActionPayload{
		Action:        action,
		SessionRunID:  sessionRunID,
		SessionID:     sessionID,
		StepID:        stepID,
		Reply:         strings.TrimSpace(stringFromAny(val["reply"])),
		QuestionIndex: intFromAny(val["q"], -1),
		Option:        stringFromAny(val["opt"]),
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

// intFromAny 从卡片 value 中解析整型字段（JSON 数字经 decoder 可能是 float64/json.Number）。
func intFromAny(v any, fallback int) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return int(n)
		}
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(t)); err == nil {
			return n
		}
	}
	return fallback
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
