package service

import (
	"context"

	"aranea-agents/internal/event"
)

func (s *ChatService) bumpSessionRevisionAndPublish(ctx context.Context, sessionID, runID, turnID string) {
	if s == nil || s.td.Sessions == nil || s.td.Pipeline.Bus == nil {
		return
	}
	event.BumpAndPublishSessionRevision(
		ctx,
		s.td.Sessions,
		s.td.Pipeline.Bus,
		sessionID,
		runID,
		turnID,
		event.EnvelopeSourceFromContext(ctx),
	)
}
