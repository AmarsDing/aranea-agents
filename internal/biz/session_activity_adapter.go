package biz

import (
	"context"

	"aranea-agents/internal/biz/session"
)

// sessionActivityLister adapts biz.StepV2Reader to session.ActivityLister.
// This bridges the package boundary: biz/session cannot import biz (circular),
// so the adapter is provided from the biz package side via Wire.
//
// Adapts StepV2Reader → session.ActivityLister (DTO shape for compress/search).
// Steps are converted to the v1 Activity shape via StepToActivity for
// backward compat with the session.ActivityEntry conversion.
type sessionActivityLister struct {
	stepReader StepV2Reader
}

// NewSessionActivityLister creates a session.ActivityLister from biz.StepV2Reader.
// Returns nil when stepReader is nil so NewSessionUsecase falls back to the legacy
// sessions-repo code path (used by tests/CLI without StepV2Reader wired).
func NewSessionActivityLister(stepReader StepV2Reader) session.ActivityLister {
	if stepReader == nil {
		return nil
	}
	return &sessionActivityLister{stepReader: stepReader}
}

func (a *sessionActivityLister) ListBySessionTurn(ctx context.Context, sessionID, turnID string) ([]session.ActivityEntry, error) {
	steps, err := a.stepReader.ListStepsByTurn(ctx, turnID)
	if err != nil {
		return nil, err
	}
	acts := make([]Activity, 0, len(steps))
	for _, s := range steps {
		acts = append(acts, StepToActivity(s))
	}
	return activitiesToSessionEntries(acts), nil
}

func (a *sessionActivityLister) ListBySession(ctx context.Context, sessionID string) ([]session.ActivityEntry, error) {
	steps, err := a.stepReader.ListStepsBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	acts := make([]Activity, 0, len(steps))
	for _, s := range steps {
		acts = append(acts, StepToActivity(s))
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
