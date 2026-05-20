package biz

import (
	"context"

	"aranea-agents/internal/event"
)

// stateDeltaHandler applies session state deltas from domain events.
type stateDeltaHandler struct {
	sessions *SessionUsecase
}

func newStateDeltaHandler(sessions *SessionUsecase) *stateDeltaHandler {
	return &stateDeltaHandler{sessions: sessions}
}

func (h *stateDeltaHandler) Handle(ctx context.Context, de DomainEvent) {
	if h == nil || de.StateDelta == nil || h.sessions == nil {
		return
	}
	if de.StateDelta.Path == "__state__" {
		err := h.sessions.UpdateRunnerSnapshotJSON(ctx, de.SessionID, de.StateDelta.ValueJSON)
		if err != nil {
			event.SessionSysLogWarn(context.Background(), de.SessionID, "event_bus.state.persist", "会话状态增量持久化失败", event.P("error", err))
		}
		return
	}
	err := h.sessions.ApplyStateDelta(ctx, de.SessionID, DomainStateDelta{
		Operation: de.StateDelta.Operation,
		Path:      de.StateDelta.Path,
		ValueJSON: de.StateDelta.ValueJSON,
	})
	if err != nil {
		event.SessionSysLogWarn(context.Background(), de.SessionID, "event_bus.state.apply", "会话状态增量应用失败", event.P("error", err), event.P("path", de.StateDelta.Path))
	}
}
