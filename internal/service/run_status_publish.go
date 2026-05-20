package service

import (
	"context"
	"strings"

	"aranea-agents/internal/chatactivity"
	"aranea-agents/internal/event"
	"aranea-agents/internal/biz"
)

// PublishRunStatus emits a run_status envelope for WS subscribers.
func PublishRunStatus(bus event.Bus, sessionID, runID, status, errMsg string) {
	if bus == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	env := event.NewEnvelope(event.EnvelopeTypeRunStatus, "run-service", sessionID)
	env.Channel = event.RouteChannel(env)
	env.Metadata = map[string]any{
		"run_id":        runID,
		"status":        status,
		"error_message": errMsg,
	}
	bus.Publish(context.Background(), env)
}

// CancelSessionRunSideEffects publishes cancelled run_status and marks running activity cards cancelled.
func CancelSessionRunSideEffects(ctx context.Context, bus event.Bus, sessions *biz.SessionUsecase, sessionID, runID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	PublishRunStatus(bus, sessionID, runID, "cancelled", "")
	if _, err := chatactivity.CancelRunningActivityMessages(ctx, sessions, sessionID); err != nil {
		event.CtxFlowLogWarn(ctx, "chat.activity.cancel", "取消执行卡片查询失败",
			event.P("session_id", sessionID),
			event.P("error", err.Error()),
		)
	}
}
