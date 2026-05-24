package session

import (
	"context"
	"strings"

	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/trpcscope"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// DefaultAppName is the trpc-agent-go session scope for Aranea chat runs.
const DefaultAppName = trpcscope.DefaultAppName

// Key builds a trpc session key for the default app scope.
func Key(userID, sessionID string) trpcsession.Key {
	return trpcsession.Key{
		AppName:   DefaultAppName,
		UserID:    strings.TrimSpace(userID),
		SessionID: strings.TrimSpace(sessionID),
	}
}

// TRPCUserKey resolves the user scope for trpc session.Service keys.
func TRPCUserKey(ctx context.Context) string {
	return ctxuser.TRPCUserKey(ctx)
}

// ResolveUserID is deprecated; prefer TRPCUserKey(ctx).
func ResolveUserID(bizUserID string) string {
	if uid := strings.TrimSpace(bizUserID); uid != "" {
		return uid
	}
	return ctxuser.DefaultUserID
}
