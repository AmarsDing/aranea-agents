package biz

import (
	"context"

	"aranea-agents/internal/biz/session"
)

// sessionActivityLister adapts biz.ActivityReader to session.ActivityLister.
// This bridges the package boundary: biz/session cannot import biz (circular),
// so the adapter is provided from the biz package side via Wire.
type sessionActivityLister struct {
	reader ActivityReader
}

// NewSessionActivityLister creates a session.ActivityLister from biz.ActivityReader.
// Returns nil when reader is nil so NewSessionUsecase falls back to the legacy
// sessions-repo code path (used by tests/CLI without ActivityReader wired).
func NewSessionActivityLister(reader ActivityReader) session.ActivityLister {
	if reader == nil {
		return nil
	}
	return &sessionActivityLister{reader: reader}
}

func (a *sessionActivityLister) ListBySessionTurn(ctx context.Context, sessionID, turnID string) ([]session.ActivityEntry, error) {
	acts, err := a.reader.ListBySessionTurn(ctx, sessionID, turnID)
	if err != nil {
		return nil, err
	}
	return activitiesToSessionEntries(acts), nil
}

func (a *sessionActivityLister) ListBySession(ctx context.Context, sessionID string) ([]session.ActivityEntry, error) {
	acts, err := a.reader.ListBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return activitiesToSessionEntries(acts), nil
}

// activitiesToSessionEntries converts []biz.Activity to []session.ActivityEntry.
// Only the fields needed for ChatMessage conversion are copied.
func activitiesToSessionEntries(acts []Activity) []session.ActivityEntry {
	out := make([]session.ActivityEntry, 0, len(acts))
	for _, a := range acts {
		out = append(out, session.ActivityEntry{
			ID:         a.ID,
			Kind:       string(a.Kind),
			Status:     string(a.Status),
			SessionID:  a.SessionID,
			TurnID:     a.TurnID,
			Timestamp:  a.Timestamp,
			Content:    a.Content,
			Reasoning:  a.Reasoning,
			ToolName:   a.ToolName,
			ToolResult: a.ToolResult,
			AgentKey:   a.AgentKey,
		})
	}
	return out
}
