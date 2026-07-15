package server

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// sessionAuthorizer is the production SessionAuthorizer. It verifies session
// ownership by querying the session repo so that a user cannot subscribe to
// another user's WebSocket events (IDOR protection for B-02).
type sessionAuthorizer struct {
	reader biz.SessionReader
	lg     loggateway.Logger
}

// NewSessionAuthorizer creates a production SessionAuthorizer backed by the
// session repo. Registered in the server Wire ProviderSet.
func NewSessionAuthorizer(reader biz.SessionReader, lg loggateway.Logger) SessionAuthorizer {
	return &sessionAuthorizer{reader: reader, lg: lg}
}

// CheckOwnership returns nil if sessionID belongs to userID. It preserves
// NotFound errors from the repo so callers can distinguish missing sessions
// from ownership denials. Other repo errors are propagated as-is.
func (a *sessionAuthorizer) CheckOwnership(ctx context.Context, sessionID, userID string) error {
	sess, err := a.reader.GetSessionByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if sess.UserID != userID {
		return apierror.Forbidden("SESSION", "session does not belong to user")
	}
	return nil
}
