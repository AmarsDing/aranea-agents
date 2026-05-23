package service

import (
	"context"
	"errors"
)

var errResumeInFlight = errors.New("resume already in flight for this session")

func (s *ChatService) tryBeginResume(sessionID string) bool {
	return s.orch.tryBeginResume(sessionID)
}

func (s *ChatService) endResume(sessionID string) {
	s.orch.endResume(sessionID)
}

func (s *ChatService) publishAwaitResumed(sessionID, runID string) {
	s.orch.publishAwaitResumed(sessionID, runID)
}

func (s *ChatService) resumeAwaitAfterRestart(ctx context.Context, sessionID, reply, runID string) error {
	return s.orch.resumeAwaitAfterRestart(ctx, sessionID, reply, runID)
}

func (s *ChatService) sessionAwaitingUser(ctx context.Context, sessionID string) (persistedRunStatus, bool) {
	return s.orch.sessionAwaitingUser(ctx, sessionID)
}
