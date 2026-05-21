package agent

import (
	"context"

	"aranea-agents/internal/event"
)

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
