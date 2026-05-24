package ctxuser

import (
	"context"
	"strconv"
	"strings"

	"aranea-agents/pkg/auth"
)

type key struct{}

// DefaultUserID matches trpc chat runs when no explicit user is injected.
const DefaultUserID = "default_user"

// WithUserID stores the trpc session user scope on ctx.
func WithUserID(ctx context.Context, userID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ctx
	}
	return context.WithValue(ctx, key{}, userID)
}

// TRPCUserKey returns the user scope for trpc session.Service keys.
// Matches legacy agent.UserIDFromCtx: explicit WithUserID or DefaultUserID (ignores auth).
func TRPCUserKey(ctx context.Context) string {
	if ctx == nil {
		return DefaultUserID
	}
	if v := ctx.Value(key{}); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return DefaultUserID
}

// FromContext returns explicit ctx user, auth user, or DefaultUserID.
func FromContext(ctx context.Context) string {
	if uid := TRPCUserKey(ctx); uid != DefaultUserID {
		return uid
	}
	if a, ok := auth.FromContext(ctx); ok && a != nil && a.UserID > 0 {
		return strconv.FormatInt(a.UserID, 10)
	}
	return DefaultUserID
}
