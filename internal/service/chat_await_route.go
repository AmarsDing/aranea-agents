package service

import (
	"context"
)

func (s *ChatService) canResumeAwait(ctx context.Context, sessionID string) (runID string, ok bool) {
	return s.orch.canResumeAwait(ctx, sessionID)
}

func (s *ChatService) hasPendingAwaitUserReplyRoute(ctx context.Context, sessionID string) bool {
	return s.orch.hasPendingAwaitUserReplyRoute(ctx, sessionID)
}

func (s *ChatService) resolveUserID(ctx context.Context, sessionID string) string {
	return s.orch.resolveUserID(ctx, sessionID)
}
