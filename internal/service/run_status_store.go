package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
)

type persistedRunStatus struct {
	RunID           string
	Status          string
	ErrorMessage    string
	UpdatedAt       string
	AwaitKind       string
	AwaitToolKey    string
	AwaitToolCallID string
}

const (
	stateKeyRunID            = "runtime.run_id"
	stateKeyRunStatus        = "runtime.status"
	stateKeyRunError         = "runtime.error_message"
	stateKeyRunUpdatedAt     = "runtime.updated_at"
	stateKeyAwaitRunID       = "runtime.await_run_id"
	stateKeyAwaitSince       = "runtime.await_since"
	stateKeyAwaitKind        = "runtime.await_kind"
	stateKeyAwaitToolKey     = "runtime.await_tool_key"
	stateKeyAwaitToolCallID  = "runtime.await_tool_call_id"
)

func terminalRunStatus(status string) bool {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "completed", "failed", "cancelled", "idle":
		return true
	default:
		return false
	}
}

func (s *ChatService) persistRunStatus(ctx context.Context, sessionID, runID, status, errMsg string) {
	persistRunStatusToSession(s.td.Sessions, ctx, sessionID, runID, status, errMsg)
}

func (s *ChatService) hydrateRunStatusFromSession(ctx context.Context, sessionID string) (persistedRunStatus, bool) {
	if s == nil || s.td.Sessions == nil {
		return persistedRunStatus{}, false
	}
	state, err := s.td.Sessions.GetSessionState(ctx, sessionID)
	if err != nil || len(state) == 0 {
		return persistedRunStatus{}, false
	}
	status := strings.TrimSpace(state[stateKeyRunStatus])
	if status == "" {
		return persistedRunStatus{}, false
	}
	return persistedRunStatus{
		RunID:           strings.TrimSpace(state[stateKeyRunID]),
		Status:          status,
		ErrorMessage:    strings.TrimSpace(state[stateKeyRunError]),
		UpdatedAt:       strings.TrimSpace(state[stateKeyRunUpdatedAt]),
		AwaitKind:       strings.TrimSpace(state[stateKeyAwaitKind]),
		AwaitToolKey:    strings.TrimSpace(state[stateKeyAwaitToolKey]),
		AwaitToolCallID: strings.TrimSpace(state[stateKeyAwaitToolCallID]),
	}, true
}

func (s *ChatService) persistAwaitMarkers(ctx context.Context, sessionID, runID string, await AwaitStatusMeta, syncWrite bool) {
	s.setAwaitMetaCache(sessionID, await)
	persistAwaitMarkersToSession(s.td.Sessions, ctx, sessionID, runID, await, syncWrite)
}

func (s *ChatService) setAwaitMetaCache(sessionID string, meta biz.ChatAwaitMeta) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	s.awaitMetaCache.Store(sessionID, meta)
}

func (s *ChatService) getAwaitMetaCache(sessionID string) (biz.ChatAwaitMeta, bool) {
	v, ok := s.awaitMetaCache.Load(strings.TrimSpace(sessionID))
	if !ok {
		return biz.ChatAwaitMeta{}, false
	}
	meta, ok := v.(biz.ChatAwaitMeta)
	return meta, ok
}

func (s *ChatService) clearAwaitMetaCache(sessionID string) {
	s.awaitMetaCache.Delete(strings.TrimSpace(sessionID))
}

func (s *ChatService) resolveAwaitMeta(ctx context.Context, sessionID, status string) biz.ChatAwaitMeta {
	if strings.TrimSpace(status) != "awaiting_user" {
		return biz.ChatAwaitMeta{}
	}
	if meta, ok := s.getAwaitMetaCache(sessionID); ok {
		return meta
	}
	if snap, ok := s.hydrateRunStatusFromSession(ctx, sessionID); ok {
		return biz.ChatAwaitMeta{
			Kind:       snap.AwaitKind,
			ToolKey:    snap.AwaitToolKey,
			ToolCallID: snap.AwaitToolCallID,
		}
	}
	return biz.ChatAwaitMeta{}
}

func (s *ChatService) clearAwaitingRunStateSync(ctx context.Context, sessionID string) error {
	if s == nil || s.td.Sessions == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	state, err := s.td.Sessions.GetSessionState(ctx, sessionID)
	if err != nil {
		return err
	}
	if len(state) == 0 {
		return nil
	}
	s.clearAwaitMetaCache(sessionID)
	delete(state, stateKeyRunID)
	delete(state, stateKeyRunStatus)
	delete(state, stateKeyRunError)
	delete(state, stateKeyRunUpdatedAt)
	delete(state, stateKeyAwaitRunID)
	delete(state, stateKeyAwaitSince)
	delete(state, stateKeyAwaitKind)
	delete(state, stateKeyAwaitToolKey)
	delete(state, stateKeyAwaitToolCallID)
	return s.td.Sessions.SaveSessionState(ctx, sessionID, state)
}

func (s *ChatService) clearAwaitingRunState(ctx context.Context, sessionID string) {
	s.clearAwaitMetaCache(sessionID)
	clearAwaitingRunStateFromSession(s.td.Sessions, ctx, sessionID)
}
