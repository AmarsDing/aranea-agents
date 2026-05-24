package biz

import (
	"context"

	"aranea-agents/pkg/ctxuser"
)

// stateDeltaHandler applies session state deltas from domain events.
type stateDeltaHandler struct {
	sessions   *SessionUsecase
	runnerSync RunnerSnapshotSync
	logger     SessionLogWriter
}

func newStateDeltaHandler(sessions *SessionUsecase, runnerSync RunnerSnapshotSync) *stateDeltaHandler {
	return &stateDeltaHandler{sessions: sessions, runnerSync: runnerSync}
}

func (h *stateDeltaHandler) SetLogger(logger SessionLogWriter) {
	h.logger = logger
}

func (h *stateDeltaHandler) Handle(ctx context.Context, de DomainEvent) {
	if h == nil || de.StateDelta == nil || h.sessions == nil {
		return
	}
	if de.StateDelta.Path == "__state__" {
		snapshotJSON := de.StateDelta.ValueJSON
		err := h.sessions.UpdateRunnerSnapshotJSON(ctx, de.SessionID, snapshotJSON)
		if err != nil {
			h.logError(context.Background(), de.SessionID, "event_bus.state.persist", "会话状态增量持久化失败", LogPair{Key: "error", Value: err})
			return
		}
		h.syncRunnerSnapshot(ctx, de.SessionID, snapshotJSON, "")
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
		return
	}
	h.syncStateDelta(ctx, de.SessionID, de.StateDelta.Operation, de.StateDelta.Path, de.StateDelta.ValueJSON)
}

func (h *stateDeltaHandler) syncStateDelta(ctx context.Context, sessionID, operation, path, valueJSON string) {
	if h == nil || h.runnerSync == nil {
		return
	}
	userID := ctxuser.TRPCUserKey(ctx)
	if err := h.runnerSync.SyncStateDelta(ctx, userID, sessionID, operation, path, valueJSON); err != nil {
		h.logError(context.Background(), sessionID, "event_bus.state.trpc_kv_sync", "trpc 会话 KV 同步失败",
			LogPair{Key: "error", Value: err}, LogPair{Key: "path", Value: path}, LogPair{Key: "user_id", Value: userID})
	}
}

func (h *stateDeltaHandler) syncRunnerSnapshot(ctx context.Context, sessionID, snapshotJSON, summaryMarkdown string) {
	if h == nil || h.runnerSync == nil {
		return
	}
	userID := ctxuser.TRPCUserKey(ctx)
	if err := h.runnerSync.SyncRunnerSnapshot(ctx, userID, sessionID, snapshotJSON, summaryMarkdown); err != nil {
		h.logError(context.Background(), sessionID, "event_bus.state.trpc_sync", "trpc 会话快照同步失败",
			LogPair{Key: "error", Value: err}, LogPair{Key: "user_id", Value: userID})
	}
}

func (h *stateDeltaHandler) logError(ctx context.Context, sessionID, stepID, message string, pairs ...LogPair) {
	if h.logger != nil {
		h.logger.SessionSysLogError(ctx, sessionID, stepID, message, pairs...)
	}
}
