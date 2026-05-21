package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
)

var ErrActivityToolCallIDRequired = errors.New("tool_call id required")

const ChatActivitySchemaV1 = "chat.activity/v1"

func ActivityMessageID(toolCallID string) (string, error) {
	id := strings.TrimSpace(toolCallID)
	if id == "" {
		return "", ErrActivityToolCallIDRequired
	}
	return "act-" + id, nil
}

func ActivityMessageStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "calling", "running", "in_progress":
		return "tool_running"
	case "blocked":
		return "tool_blocked"
	case "cancelled":
		return "tool_cancelled"
	case "failed", "error":
		return "tool_failed"
	default:
		return "tool_success"
	}
}

func FormatActivityMarkdown(displayLabel, agentName, summary, status string, durationMS int64, errMsg string) string {
	label := strings.TrimSpace(displayLabel)
	if label == "" {
		label = "tool"
	}
	agent := strings.TrimSpace(agentName)
	if agent == "" {
		agent = "Agent"
	}
	summary = strings.TrimSpace(summary)
	summaryPart := ""
	if summary != "" {
		summaryPart = " · " + summary
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "calling", "running", "in_progress":
		return fmt.Sprintf("工具调用：%s 正在使用 **%s**%s", agent, label, summaryPart)
	case "blocked":
		return fmt.Sprintf("工具调用待确认：%s 使用 **%s**%s", agent, label, summaryPart)
	case "cancelled":
		return fmt.Sprintf("工具调用已取消：%s 使用 **%s**%s", agent, label, summaryPart)
	case "failed", "error":
		errText := strings.TrimSpace(errMsg)
		if errText == "" {
			errText = "未知错误"
		}
		return fmt.Sprintf("工具调用失败：%s 使用 **%s**%s\n\n%s", agent, label, summaryPart, errText)
	default:
		duration := ""
		if durationMS > 0 {
			duration = fmt.Sprintf("，耗时 %dms", durationMS)
		}
		return fmt.Sprintf("工具调用完成：%s 已使用 **%s**%s%s", agent, label, summaryPart, duration)
	}
}

func toolEventFromEnvelope(tc event.EnvelopeToolCall, meta ProjectMeta) map[string]any {
	phase := "before"
	if strings.TrimSpace(tc.ResultJSON) != "" || strings.EqualFold(tc.Status, "success") || strings.EqualFold(tc.Status, "failed") {
		phase = "after"
	}
	eventStatus := strings.ToLower(strings.TrimSpace(tc.Status))
	switch eventStatus {
	case "calling":
		eventStatus = "running"
	}
	agentKey := strings.TrimSpace(tc.AgentKey)
	if agentKey == "" {
		agentKey = strings.TrimSpace(tc.AgentName)
	}
	ev := map[string]any{
		"id":              tc.ID,
		"phase":           phase,
		"status":          eventStatus,
		"agent_id":        firstNonEmptyStr(tc.AgentID, meta.AgentID),
		"agent_key":       agentKey,
		"agent_name":      firstNonEmptyStr(tc.AgentName, tc.AgentKey, meta.AgentDisplayName),
		"tool_name":       tc.Name,
		"tool_label":      coalesceStr(tc.DisplayLabel, tc.Name),
		"display_label":   tc.DisplayLabel,
		"activity_kind":   tc.ActivityKind,
		"icon_key":        tc.IconKey,
		"summary":         tc.Summary,
		"started_at":      tc.StartedAt,
		"finished_at":     tc.FinishedAt,
		"error_code":      tc.ErrorCode,
		"run_id":          coalesceStr(tc.RunID, meta.RunID),
		"trace_id":        coalesceStr(tc.TraceID, meta.TraceID),
		"duration_ms":     tc.DurationMS,
		"is_long_running": tc.IsLongRunning,
		"occurred_at":     firstNonEmptyStr(tc.FinishedAt, tc.StartedAt, RFC3339Now()),
	}
	if args := parseJSONObject([]byte(tc.ArgumentsJSON)); len(args) > 0 {
		ev["arguments"] = args
	}
	if res := parseJSONObject([]byte(tc.ResultJSON)); len(res) > 0 {
		ev["result"] = res
		if errVal, ok := res["error"].(string); ok && strings.TrimSpace(errVal) != "" {
			ev["error"] = strings.TrimSpace(errVal)
		}
	}
	return ev
}

func ChatMessageFromToolActivity(meta ProjectMeta, tc event.EnvelopeToolCall) (biz.ChatMessage, error) {
	msgID, err := ActivityMessageID(tc.ID)
	if err != nil {
		return biz.ChatMessage{}, err
	}
	toolEvent := toolEventFromEnvelope(tc, meta)
	errMsg := ""
	if v, ok := toolEvent["error"].(string); ok {
		errMsg = v
	}
	agentName, _ := toolEvent["agent_name"].(string)
	displayLabel, _ := toolEvent["display_label"].(string)
	if displayLabel == "" {
		displayLabel = tc.Name
	}
	optionsRaw, _ := json.Marshal(map[string]any{
		"schema":     ChatActivitySchemaV1,
		"agent":      activityAgentOptions(meta, tc),
		"tool_event": toolEvent,
	})
	return biz.ChatMessage{
		ID:              msgID,
		SessionID:       meta.SessionID,
		Role:            "assistant",
		ContentMarkdown: FormatActivityMarkdown(displayLabel, agentName, tc.Summary, tc.Status, tc.DurationMS, errMsg),
		LatencyMS:       int(tc.DurationMS),
		Status:          ActivityMessageStatus(tc.Status),
		OptionsJSON:     string(optionsRaw),
		ErrorMessage:    errMsg,
		CreatedAt:       coalesceStr(tc.StartedAt, RFC3339Now()),
	}, nil
}

func activityAgentOptions(meta ProjectMeta, tc event.EnvelopeToolCall) map[string]string {
	key := strings.TrimSpace(tc.AgentKey)
	if key == "" {
		key = strings.TrimSpace(tc.AgentName)
	}
	name := firstNonEmptyStr(tc.AgentName, tc.AgentKey, meta.AgentDisplayName)
	return map[string]string{
		"agent_id":  firstNonEmptyStr(tc.AgentID, meta.AgentID),
		"agent_key": key,
		"name":      name,
	}
}

// CancelledActivityMessage transitions a persisted running card to cancelled (StopGeneration).
func CancelledActivityMessage(msg biz.ChatMessage) (biz.ChatMessage, bool) {
	if strings.TrimSpace(msg.Status) != "tool_running" {
		return msg, false
	}
	var opts map[string]any
	if err := json.Unmarshal([]byte(msg.OptionsJSON), &opts); err != nil || opts == nil {
		return msg, false
	}
	toolEventRaw, ok := opts["tool_event"].(map[string]any)
	if !ok || toolEventRaw == nil {
		return msg, false
	}
	toolEventRaw["status"] = "cancelled"
	toolEventRaw["phase"] = "after"
	if _, hasErr := toolEventRaw["error"]; !hasErr {
		toolEventRaw["error"] = "用户已停止生成"
	}
	displayLabel := stringField(toolEventRaw, "display_label", "tool_label", "tool_name")
	agentName := stringField(toolEventRaw, "agent_name", "agent_key")
	if agentName == "" {
		if agent, ok := opts["agent"].(map[string]any); ok {
			agentName = stringField(agent, "name", "agent_key")
		}
	}
	summary := stringField(toolEventRaw, "summary")
	cancelReason := stringField(toolEventRaw, "error")
	msg.Status = ActivityMessageStatus("cancelled")
	msg.ContentMarkdown = FormatActivityMarkdown(displayLabel, agentName, summary, "cancelled", int64(msg.LatencyMS), cancelReason)
	msg.ErrorMessage = cancelReason
	out, err := json.Marshal(opts)
	if err != nil {
		return msg, false
	}
	msg.OptionsJSON = string(out)
	return msg, true
}

func stringField(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func firstNonEmptyStr(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
