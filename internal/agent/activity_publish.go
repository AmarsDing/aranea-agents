package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

const stuckToolResultI18nKey = "chat.tool.stuckTimeout"

// stuckToolResultFallback is the human-readable fallback message for the error field.
// The i18n_key field takes priority on the frontend; this fallback is shown when i18n is unavailable.
const stuckToolResultFallback = "工具执行未返回结果，已自动标记为失败。如需重试请重新发送指令"

func toolCallIDValid(tc *event.EnvelopeToolCall) bool {
	return tc != nil && strings.TrimSpace(tc.ID) != ""
}

// PublishActivityEnvelopes persists chat.activity rows after WS publish (orchestration lives outside EventProjector).
func PublishActivityEnvelopes(ctx context.Context, meta ProjectMeta, persister ActivityPersister, envelopes []event.Envelope, lg loggateway.Logger) {
	if persister == nil || len(envelopes) == 0 {
		return
	}
	for _, env := range envelopes {
		if env.ToolCall == nil {
			continue
		}
		if !toolCallIDValid(env.ToolCall) {
			lg.Warn("跳过无ID的ToolCall卡片",
				loggateway.StepID("agent.activity.persist"),
				loggateway.Str("session_id", meta.SessionID),
				loggateway.Str("tool_name", env.ToolCall.Name),
			)
			continue
		}
		if env.Type != event.EnvelopeTypeToolCall && env.Type != event.EnvelopeTypeToolResult {
			continue
		}
		if err := persister.UpsertActivity(ctx, meta, *env.ToolCall); err != nil {
			lg.Warn("执行卡片落库失败",
				loggateway.StepID("agent.activity.persist"),
				loggateway.Str("session_id", meta.SessionID),
				loggateway.Str("tool_call_id", env.ToolCall.ID),
				loggateway.Str("tool_name", env.ToolCall.Name),
				loggateway.Err(err),
			)
		}
	}
}

func stuckToolCallPatch(tc event.EnvelopeToolCall) event.EnvelopeToolCall {
	errPayload, _ := json.Marshal(map[string]string{"error": stuckToolResultFallback, "i18n_key": stuckToolResultI18nKey})
	stuck := tc
	stuck.Status = "failed"
	stuck.ErrorCode = event.ErrorCodeToolTimeout
	stuck.ResultJSON = string(errPayload)
	if strings.TrimSpace(stuck.FinishedAt) == "" {
		stuck.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return stuck
}

// PublishStuckToolResultEnvelopes emits failed tool_result envelopes for orphan in-flight tools (CC-FIX-TOOL-01).
//
// AS-EVT-01: ToolResult is a Critical event — must go through Infra.Publish (WBPF)
// to ensure durability. Direct bus.Publish would lose the event on crash, leaving
// the frontend tool card stuck in "running" forever.
func PublishStuckToolResultEnvelopes(ctx context.Context, meta ProjectMeta, infra *event.Infra, pending map[string]event.EnvelopeToolCall) {
	if infra == nil || len(pending) == 0 {
		return
	}
	for _, tc := range pending {
		stuck := stuckToolCallPatch(tc)
		author := strings.TrimSpace(stuck.AgentName)
		if author == "" {
			author = strings.TrimSpace(stuck.AgentKey)
		}
		if author == "" {
			author = "Agent"
		}
		env := event.NewEnvelope(event.EnvelopeTypeToolResult, author, meta.SessionID)
		env.RequestID = meta.RequestID
		env.InvocationID = meta.InvocationID
		env.ParentInvocationID = meta.ParentInvocationID
		env.ToolCall = &stuck
		infra.Publish(ctx, env)
	}
}

// FinalizeStuckToolActivities marks in-flight tool cards as failed when a turn ends without tool_result.
func FinalizeStuckToolActivities(ctx context.Context, meta ProjectMeta, persister ActivityPersister, pending map[string]event.EnvelopeToolCall, lg loggateway.Logger) {
	if persister == nil || len(pending) == 0 {
		return
	}
	for _, tc := range pending {
		if !toolCallIDValid(&tc) {
			lg.Warn("跳过无ID的未完成工具卡片",
				loggateway.StepID("agent.activity.finalize_stuck"),
				loggateway.Str("session_id", meta.SessionID),
				loggateway.Str("tool_name", tc.Name),
			)
			continue
		}
		stuck := stuckToolCallPatch(tc)
		if err := persister.UpsertActivity(ctx, meta, stuck); err != nil {
			lg.Warn("未完成工具卡片落库失败",
				loggateway.StepID("agent.activity.finalize_stuck"),
				loggateway.Str("session_id", meta.SessionID),
				loggateway.Str("tool_call_id", stuck.ID),
				loggateway.Str("tool_name", stuck.Name),
				loggateway.Err(err),
			)
		}
	}
}

// publishStuckToolNotification pushes a user-facing alert via WS when tools get stuck.
// This complements the tool_result failure envelope (which is for programmatic cleanup)
// with a human-readable notification the user can see and act on.
//
// AlertNotify is an Informational event — direct bus.Publish is acceptable (no WBPF).
func publishStuckToolNotification(ctx context.Context, meta ProjectMeta, infra *event.Infra, pending map[string]event.EnvelopeToolCall) {
	if infra == nil || len(pending) == 0 {
		return
	}
	bus := infra.SessionBus
	if bus == nil {
		return
	}
	toolNames := make([]string, 0, len(pending))
	for _, tc := range pending {
		if name := strings.TrimSpace(tc.Name); name != "" {
			toolNames = append(toolNames, name)
		}
	}
	env := event.NewEnvelope(event.EnvelopeTypeAlertNotify, "chat-service", meta.SessionID)
	env.RequestID = meta.RequestID
	env.InvocationID = meta.InvocationID
	env.Metadata = map[string]any{
		"alert_kind": "stuck_tool",
		"tool_names": toolNames,
		"count":      len(pending),
		"message":    stuckToolResultFallback,
		"i18n_key":   stuckToolResultI18nKey,
	}
	bus.Publish(ctx, env)
}
