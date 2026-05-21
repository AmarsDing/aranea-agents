package service

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"

	"github.com/google/uuid"
)

func (s *ChatService) recordTurnUsage(
	ctx context.Context,
	emitter *event.TraceEmitter,
	sessionID, runID, agentKey, agentID, prov, mod, status string,
	promptTok, completionTok int,
	latency time.Duration,
	errMsg string,
) {
	if s == nil || s.usage == nil {
		return
	}
	now := time.Now().UTC()
	meta := "{}"
	if emitter != nil {
		meta = emitter.MetadataJSON()
	}
	usageID := uuid.NewString()
	traceID := ""
	if emitter != nil {
		traceID = emitter.TraceID()
	}
	ev := biz.TokenUsageEvent{
		ID:               usageID,
		SessionID:        sessionID,
		AgentKey:         agentKey,
		AgentID:          agentID,
		ModelAPIID:       mod,
		ModelDisplayName: mod,
		ProviderCode:     prov,
		InputTokens:      promptTok,
		OutputTokens:     completionTok,
		TotalTokens:      promptTok + completionTok,
		LatencyMS:        int(latency.Milliseconds()),
		Status:           status,
		UsageKind:        biz.UsageKindChatTurn,
		MetadataJSON:     meta,
		OccurredAt:       now.Format(time.RFC3339),
		DateKey:          now.Format("2006-01-02"),
		HourKey:          now.Format("2006-01-02T15"),
		ErrorMessage:     errMsg,
	}
	if runID != "" {
		ev.MessageID = runID
	}
	recCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
	defer cancel()
	if _, err := s.usage.RecordTokenUsageEvent(recCtx, ev); err != nil && emitter != nil {
		emitter.LogError("chat.usage_record", "用量落库失败",
			event.P("error", err.Error()),
			event.P("run_id", runID),
			event.P("usage_kind", ev.UsageKind),
			event.P("status", status),
		)
		return
	}
	if s.monitor != nil && sessionID != "" && runID != "" {
		linkCtx, linkCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer linkCancel()
		if err := s.monitor.LinkRunnerCompletionUsage(linkCtx, sessionID, runID, usageID, traceID); err != nil && emitter != nil {
			emitter.LogWarn("chat.completion_link", "关联 runner.completion 失败", err.Error(),
				event.P("run_id", runID),
				event.P("usage_event_id", usageID),
			)
		}
	}
}
