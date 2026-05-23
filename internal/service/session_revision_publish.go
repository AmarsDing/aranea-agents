package service

import (
	"context"
)

// bumpSessionRevisionAndPublish increments the session revision and publishes a WS notification.
func (s *ChatService) bumpSessionRevisionAndPublish(ctx context.Context, sessionID, runID, turnID string) {
	s.orch.bumpSessionRevisionAndPublish(ctx, sessionID, runID, turnID)
}
