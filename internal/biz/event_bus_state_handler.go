package biz

import (
	"context"
)

// stateDeltaHandler applies session state deltas from domain events.
type stateDeltaHandler struct {
	sessions *SessionUsecase
	logger   SessionLogWriter
}

func newStateDeltaHandler(sessions *SessionUsecase) *stateDeltaHandler {
	return &stateDeltaHandler{sessions: sessions}
}

func (h *stateDeltaHandler) SetLogger(logger SessionLogWriter) {
	h.logger = logger
}

func (h *stateDeltaHandler) Handle(ctx context.Context, de DomainEvent) {
	if h == nil || de.StateDelta == nil || h.sessions == nil {
		return
	}
	if de.StateDelta.Path == "__state__" {
		err := h.sessions.UpdateRunnerSnapshotJSON(ctx, de.SessionID, de.StateDelta.ValueJSON)
		if err != nil {
			h.logError(context.Background(), de.SessionID, "event_bus.state.persist", "会话状态增量持久化失败", LogPair{Key: "error", Value: err})
		}
		return
	}
	err := h.sessions.ApplyStateDelta(ctx, de.SessionID, DomainStateDelta{
		Operation: de.StateDelta.Operation,
		Path:      de.StateDelta.Path,
		ValueJSON: de.StateDelta.ValueJSON,
	})
	if err != nil {
		h.logError(context.Background(), de.SessionID, "event_bus.state.apply", "会话状态增量应用失败",
			LogPair{Key: "error", Value: err}, LogPair{Key: "path", Value: de.StateDelta.Path})
	}
}

func (h *stateDeltaHandler) logError(ctx context.Context, sessionID, stepID, message string, pairs ...LogPair) {
	if h.logger != nil {
		h.logger.SessionSysLogError(ctx, sessionID, stepID, message, pairs...)
	}
}
