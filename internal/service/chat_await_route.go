package service

import (
	"context"
	"strings"

	chatagent "aranea-agents/internal/agent"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// canResumeAwait reports whether a cross-process await resume is allowed for the session.
func (s *ChatService) canResumeAwait(ctx context.Context, sessionID string) (runID string, ok bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", false
	}
	if snap, awaiting := s.sessionAwaitingUser(ctx, sessionID); awaiting {
		return strings.TrimSpace(snap.RunID), true
	}
	if s.hasPendingAwaitUserReplyRoute(ctx, sessionID) {
		if snap, ok := s.hydrateRunStatusFromSession(ctx, sessionID); ok {
			return strings.TrimSpace(snap.RunID), true
		}
		return "", true
	}
	return "", false
}

func (s *ChatService) hasPendingAwaitUserReplyRoute(ctx context.Context, sessionID string) bool {
	if s == nil || s.td.Persist.Session == nil {
		return false
	}
	userID := s.resolveUserID(ctx, sessionID)
	if userID == "" {
		return false
	}
	sess, err := s.td.Persist.Session.GetSession(ctx, trpcsession.Key{
		AppName:   chatagent.TRPCDefaultAppName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil || sess == nil {
		return false
	}
	_, pending, err := trpcagent.PendingAwaitUserReplyRoute(sess)
	return err == nil && pending
}

func (s *ChatService) resolveUserID(ctx context.Context, sessionID string) string {
	if uid := strings.TrimSpace(chatagent.UserIDFromCtx(ctx)); uid != "" {
		return uid
	}
	if s == nil || s.td.Sessions == nil {
		return ""
	}
	sess, err := s.td.Sessions.Get(ctx, sessionID)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(sess.UserID)
}
