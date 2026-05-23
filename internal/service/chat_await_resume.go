package service

import (
	"context"
	"errors"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"
)

var errResumeInFlight = errors.New("await resume already in progress for session")

func (s *ChatService) tryBeginResume(sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	_, loaded := s.resumeInFlight.LoadOrStore(sessionID, struct{}{})
	return !loaded
}

func (s *ChatService) endResume(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	s.resumeInFlight.Delete(sessionID)
}

func (s *ChatService) publishAwaitResumed(sessionID, runID string) {
	bus := s.td.Pipeline.Bus
	if bus == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	env := event.NewEnvelope(event.EnvelopeTypeRunStatus, "chat-service", sessionID)
	env.Channel = event.RouteChannel(env)
	env.Metadata = map[string]any{
		"run_id":        runID,
		"status":        "running",
		"await_resumed": true,
	}
	bus.Publish(context.Background(), env)
}

func (s *ChatService) resumeAwaitAfterRestart(ctx context.Context, sessionID, reply, runID string) error {
	if !s.tryBeginResume(sessionID) {
		return errResumeInFlight
	}
	if err := s.clearAwaitingRunStateSync(ctx, sessionID); err != nil {
		s.endResume(sessionID)
		return err
	}
	s.publishAwaitResumed(sessionID, runID)
	req := &chatv1.SendChatMessageRequest{
		SessionId: sessionID,
		Content:   reply,
	}
	safego.Go(ctx, "chat.resume_await_turn", func() {
		defer s.endResume(sessionID)
		bgCtx, cancel := context.WithTimeout(context.Background(), defaultTurnTimeout)
		defer cancel()
		_, _, turnErr := s.runNativeAgentTurn(bgCtx, req)
		if turnErr != nil && !IsTurnMessageQueued(turnErr) {
			s.setRunStatus(sessionID, runID, "failed", turnErr.Error())
			if s.td.Pipeline.Bus != nil {
				env := event.NewEnvelope(event.EnvelopeTypeError, "chat-service", sessionID)
				env.Error = &event.EnvelopeError{
					Type:    "await_resume_failed",
					Message: turnErr.Error(),
				}
				s.td.Pipeline.Bus.Publish(context.Background(), env)
			}
		}
	})
	return nil
}

func (s *ChatService) sessionAwaitingUser(ctx context.Context, sessionID string) (persistedRunStatus, bool) {
	snap, ok := s.hydrateRunStatusFromSession(ctx, sessionID)
	if !ok {
		return persistedRunStatus{}, false
	}
	if strings.TrimSpace(strings.ToLower(snap.Status)) != "awaiting_user" {
		return persistedRunStatus{}, false
	}
	return snap, true
}
