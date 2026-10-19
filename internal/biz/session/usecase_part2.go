package session

import (
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"context"
	"strings"
)

func (uc *SessionUsecase) GetRootSession(ctx context.Context, sessionID string) (Session, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return Session{}, validationErr("session_id is required")
	}
	sess, err := uc.sessionReader.GetSessionByID(ctx, sessionID)
	if err != nil {
		return Session{}, err
	}
	const maxDepth = 10
	for i := 0; i < maxDepth && sess.RootSessionID != ""; i++ {
		root, err := uc.sessionReader.GetSessionByID(ctx, sess.RootSessionID)
		if err != nil {
			return Session{}, err
		}
		if root.RootSessionID == "" || root.RootSessionID == root.ID {
			return root, nil
		}
		sess = root
	}
	if sess.RootSessionID != "" {
		uc.lg.Warn("GetRootSession 达到最大遍历深度，可能存在循环引用",
			loggateway.StepID("session.root_max_depth"),
			loggateway.Str("session_id", sessionID),
			loggateway.Int("max_depth", maxDepth),
		)
	}
	return sess, nil
}

func normalizeSessionSearch(q *SessionSearchQuery) {
	if q.PageSize > 0 {
		q.Limit = q.PageSize
		q.Offset = (q.Page - 1) * q.PageSize
		if q.Page <= 0 {
			q.Offset = 0
		}
	}
	if q.Limit <= 0 {
		q.Limit = 20
	}
	if q.Limit > MaxSessionSearchLimit {
		q.Limit = MaxSessionSearchLimit
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
}

func validationErr(msg string) error {
	return apierror.BadRequest("SESSION", msg)
}
