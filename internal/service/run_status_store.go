package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
)

// persistedRunStatus is the on-disk shape stored in session metadata for run lifecycle.
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
	stateKeyRunID           = "runtime.run_id"
	stateKeyRunStatus       = "runtime.status"
	stateKeyRunError        = "runtime.error_message"
	stateKeyRunUpdatedAt    = "runtime.updated_at"
	stateKeyAwaitRunID      = "runtime.await_run_id"
	stateKeyAwaitSince      = "runtime.await_since"
	stateKeyAwaitKind       = "runtime.await_kind"
	stateKeyAwaitToolKey    = "runtime.await_tool_key"
	stateKeyAwaitToolCallID = "runtime.await_tool_call_id"
)

func terminalRunStatus(status string) bool {
	return biz.IsSessionRunPhaseTerminal(biz.ParseSessionRunPhase(status)) || strings.TrimSpace(strings.ToLower(status)) == "idle"
}

func (s *ChatService) persistRunStatus(ctx context.Context, sessionID, runID, status, errMsg string) {
	s.orch.persistRunStatus(ctx, sessionID, runID, status, errMsg)
}

func (s *ChatService) hydrateRunStatusFromSession(ctx context.Context, sessionID string) (persistedRunStatus, bool) {
	return s.orch.hydrateRunStatusFromSession(ctx, sessionID)
}

func (s *ChatService) persistAwaitMarkers(ctx context.Context, sessionID, runID string, await AwaitStatusMeta, syncWrite bool) {
	s.orch.persistAwaitMarkers(ctx, sessionID, runID, await, syncWrite)
}

func (s *ChatService) setAwaitMetaCache(sessionID string, meta biz.ChatAwaitMeta) {
	s.orch.setAwaitMetaCache(sessionID, meta)
}

func (s *ChatService) getAwaitMetaCache(sessionID string) (biz.ChatAwaitMeta, bool) {
	return s.orch.getAwaitMetaCache(sessionID)
}

func (s *ChatService) clearAwaitMetaCache(sessionID string) {
	s.orch.clearAwaitMetaCache(sessionID)
}

func (s *ChatService) resolveAwaitMeta(ctx context.Context, sessionID, status string) biz.ChatAwaitMeta {
	return s.orch.resolveAwaitMeta(ctx, sessionID, status)
}

func (s *ChatService) clearAwaitingRunStateSync(ctx context.Context, sessionID string) error {
	return s.orch.clearAwaitingRunStateSync(ctx, sessionID)
}

func (s *ChatService) clearAwaitingRunState(ctx context.Context, sessionID string) {
	s.orch.clearAwaitingRunState(ctx, sessionID)
}
