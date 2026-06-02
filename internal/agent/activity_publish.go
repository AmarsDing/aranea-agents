package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

const stuckToolResultReason = "turn completed without tool result"

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
	errPayload, _ := json.Marshal(map[string]string{"error": stuckToolResultReason})
	stuck := tc
	stuck.Status = "failed"
	stuck.ErrorCode = "tool_timeout"
	stuck.ResultJSON = string(errPayload)
	if strings.TrimSpace(stuck.FinishedAt) == "" {
		stuck.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return stuck
}

// PublishStuckToolResultEnvelopes emits failed tool_result envelopes for orphan in-flight tools (CC-FIX-TOOL-01).
func PublishStuckToolResultEnvelopes(ctx context.Context, meta ProjectMeta, bus event.Bus, pending map[string]event.EnvelopeToolCall) {
	if bus == nil || len(pending) == 0 {
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
		bus.Publish(ctx, env)
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
