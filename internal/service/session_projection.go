package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
)

// SessionProjectionAdapter implements biz.SessionProjection for read-only session views (DECO-13).
type SessionProjectionAdapter struct {
	sessions *biz.SessionUsecase
	orch     *ChatOrchestrator
}

// NewSessionProjectionAdapter wires session messages + run activity into the projection port.
var _ biz.SessionProjection = (*SessionProjectionAdapter)(nil)

func NewSessionProjectionAdapter(sessions *biz.SessionUsecase, orch *ChatOrchestrator) *SessionProjectionAdapter {
	return &SessionProjectionAdapter{sessions: sessions, orch: orch}
}

func (p *SessionProjectionAdapter) GetMessagesAfterRevision(ctx context.Context, sessionID string, afterRevision int) ([]biz.ChatMessage, error) {
	if p == nil || p.sessions == nil {
		return nil, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil
	}
	if afterRevision < 0 {
		afterRevision = 0
	}
	return p.sessions.ListMessagesAfterRevision(ctx, sessionID, int64(afterRevision))
}

func (p *SessionProjectionAdapter) GetLatestRevision(ctx context.Context, sessionID string) (int, error) {
	if p == nil || p.sessions == nil {
		return 0, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, nil
	}
	rev, err := p.sessions.GetSessionRevision(ctx, sessionID)
	if err != nil {
		return 0, err
	}
	return int(rev), nil
}

func (p *SessionProjectionAdapter) GetSessionActivity(ctx context.Context, sessionID string) (*biz.SessionActivity, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil
	}
	out := &biz.SessionActivity{SessionID: sessionID}
	if p == nil || p.orch == nil {
		return out, nil
	}
	out.HasActive = p.orch.HasActiveRun(sessionID)
	if snap, ok := p.orch.hydrateRunStatusFromSession(ctx, sessionID); ok {
		out.RunStatus = snap.Status
		out.RunID = snap.RunID
		out.AwaitKind = snap.AwaitKind
		out.AwaitToolKey = snap.AwaitToolKey
	}
	for _, entry := range p.orch.GetPendingMessages(sessionID) {
		if id := strings.TrimSpace(entry.ID); id != "" {
			out.PendingIDs = append(out.PendingIDs, id)
		}
	}
	return out, nil
}
