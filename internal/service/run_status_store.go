package service

import (
	"context"
	"strings"
	"time"

	"aranea-agents/pkg/safego"
)

type persistedRunStatus struct {
	RunID        string
	Status       string
	ErrorMessage string
	UpdatedAt    string
}

const (
	stateKeyRunID        = "runtime.run_id"
	stateKeyRunStatus    = "runtime.status"
	stateKeyRunError     = "runtime.error_message"
	stateKeyRunUpdatedAt = "runtime.updated_at"
	stateKeyAwaitRunID   = "runtime.await_run_id"
	stateKeyAwaitSince   = "runtime.await_since"
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
	if s == nil || s.td.Sessions == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	safego.Go(ctx, "chat.persist_run_status", func() {
		bg := context.Background()
		state, err := s.td.Sessions.GetSessionState(bg, sessionID)
		if err != nil {
			return
		}
		if state == nil {
			state = map[string]string{}
		}
		if terminalRunStatus(status) {
			delete(state, stateKeyRunID)
			delete(state, stateKeyRunStatus)
			delete(state, stateKeyRunError)
			delete(state, stateKeyRunUpdatedAt)
			delete(state, stateKeyAwaitRunID)
			delete(state, stateKeyAwaitSince)
		} else {
			state[stateKeyRunID] = strings.TrimSpace(runID)
			state[stateKeyRunStatus] = strings.TrimSpace(status)
			state[stateKeyRunError] = strings.TrimSpace(errMsg)
			state[stateKeyRunUpdatedAt] = now
		}
		_ = s.td.Sessions.SaveSessionState(bg, sessionID, state)
	})
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
		RunID:        strings.TrimSpace(state[stateKeyRunID]),
		Status:       status,
		ErrorMessage: strings.TrimSpace(state[stateKeyRunError]),
		UpdatedAt:    strings.TrimSpace(state[stateKeyRunUpdatedAt]),
	}, true
}

func (s *ChatService) persistAwaitMarkers(ctx context.Context, sessionID, runID string) {
	if s == nil || s.td.Sessions == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	safego.Go(ctx, "chat.persist_await_markers", func() {
		bg := context.Background()
		state, err := s.td.Sessions.GetSessionState(bg, sessionID)
		if err != nil {
			return
		}
		if state == nil {
			state = map[string]string{}
		}
		state[stateKeyAwaitRunID] = strings.TrimSpace(runID)
		state[stateKeyAwaitSince] = now
		_ = s.td.Sessions.SaveSessionState(bg, sessionID, state)
	})
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
	delete(state, stateKeyRunID)
	delete(state, stateKeyRunStatus)
	delete(state, stateKeyRunError)
	delete(state, stateKeyRunUpdatedAt)
	delete(state, stateKeyAwaitRunID)
	delete(state, stateKeyAwaitSince)
	return s.td.Sessions.SaveSessionState(ctx, sessionID, state)
}

func (s *ChatService) clearAwaitingRunState(ctx context.Context, sessionID string) {
	if s == nil || s.td.Sessions == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	safego.Go(ctx, "chat.clear_await_state", func() {
		bg := context.Background()
		state, err := s.td.Sessions.GetSessionState(bg, sessionID)
		if err != nil || len(state) == 0 {
			return
		}
		delete(state, stateKeyRunID)
		delete(state, stateKeyRunStatus)
		delete(state, stateKeyRunError)
		delete(state, stateKeyRunUpdatedAt)
		delete(state, stateKeyAwaitRunID)
		delete(state, stateKeyAwaitSince)
		_ = s.td.Sessions.SaveSessionState(bg, sessionID, state)
	})
}
