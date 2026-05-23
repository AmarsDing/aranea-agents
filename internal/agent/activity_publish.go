package agent

import (
	"context"
	"encoding/json"

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

// FinalizeStuckToolActivities marks in-flight tool cards as failed when a turn ends without tool_result.
func FinalizeStuckToolActivities(ctx context.Context, meta ProjectMeta, persister ActivityPersister, pending map[string]event.EnvelopeToolCall) {
	if persister == nil || len(pending) == 0 {
		return
	}
	errPayload, _ := json.Marshal(map[string]string{"error": stuckToolResultReason})
	for _, tc := range pending {
		stuck := tc
		stuck.Status = "failed"
		stuck.ErrorCode = "tool_timeout"
		stuck.ResultJSON = string(errPayload)
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
