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
	PublishRunStatusMeta(bus, sessionID, runID, status, errMsg, nil)
}

// AwaitStatusMeta is optional metadata for awaiting_user runs.
type AwaitStatusMeta = biz.ChatAwaitMeta

// PublishRunStatusMeta emits run_status with optional await metadata.
func PublishRunStatusMeta(bus event.Bus, sessionID, runID, status, errMsg string, await *AwaitStatusMeta) {
	PublishRunStatusFull(bus, sessionID, runID, status, errMsg, await, "", "")
}

// PublishRunStatusFull emits run_status with optional session_run_id and turn_id (CC-R-04).
func PublishRunStatusFull(bus event.Bus, sessionID, runID, status, errMsg string, await *AwaitStatusMeta, sessionRunID, turnID string) {
	if bus == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	env := event.NewEnvelope(event.EnvelopeTypeRunStatus, "run-service", sessionID)
	env.Channel = event.RouteChannel(env)
	meta := map[string]any{
		"run_id":        runID,
		"status":        status,
		"error_message": errMsg,
	}
	if sr := strings.TrimSpace(sessionRunID); sr != "" {
		meta["session_run_id"] = sr
	}
	if tid := strings.TrimSpace(turnID); tid != "" {
		meta["turn_id"] = tid
	}
	if await != nil {
		if k := strings.TrimSpace(await.Kind); k != "" {
			meta["await_kind"] = k
		}
		if k := strings.TrimSpace(await.ToolKey); k != "" {
			meta["await_tool_key"] = k
		}
		if k := strings.TrimSpace(await.ToolCallID); k != "" {
			meta["await_tool_call_id"] = k
		}
	}
	env.Metadata = meta
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
