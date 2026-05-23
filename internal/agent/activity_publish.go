package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/event"
)

const stuckToolResultReason = "turn completed without tool result"

// PublishActivityEnvelopes persists chat.activity rows after WS publish (orchestration lives outside EventProjector).
func PublishActivityEnvelopes(ctx context.Context, meta ProjectMeta, persister ActivityPersister, envelopes []event.Envelope) {
	if persister == nil || len(envelopes) == 0 {
		return
	}
	for _, env := range envelopes {
		if env.ToolCall == nil {
			continue
		}
		if env.Type != event.EnvelopeTypeToolCall && env.Type != event.EnvelopeTypeToolResult {
			continue
		}
		if err := persister.UpsertActivity(ctx, meta, *env.ToolCall); err != nil {
			event.CtxFlowLogWarn(ctx, "chat.activity.persist", "执行卡片落库失败",
				event.P("session_id", meta.SessionID),
				event.P("tool_call_id", env.ToolCall.ID),
				event.P("tool_name", env.ToolCall.Name),
				event.P("error", err.Error()),
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
func FinalizeStuckToolActivities(ctx context.Context, meta ProjectMeta, persister ActivityPersister, pending map[string]event.EnvelopeToolCall) {
	if persister == nil || len(pending) == 0 {
		return
	}
	for _, tc := range pending {
		stuck := stuckToolCallPatch(tc)
		if err := persister.UpsertActivity(ctx, meta, stuck); err != nil {
			event.CtxFlowLogWarn(ctx, "chat.activity.finalize_stuck", "未完成工具卡片落库失败",
				event.P("session_id", meta.SessionID),
				event.P("tool_call_id", stuck.ID),
				event.P("tool_name", stuck.Name),
				event.P("error", err.Error()),
			)
		}
	}
}
