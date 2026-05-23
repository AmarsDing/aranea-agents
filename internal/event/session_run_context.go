package event

import (
	"context"
	"strings"
)

type sessionRunIDKey struct{}

// WithSessionRunID tags ctx with M55 session_runs.id for envelope projection (CC-R-04).
func WithSessionRunID(ctx context.Context, sessionRunID string) context.Context {
	sessionRunID = strings.TrimSpace(sessionRunID)
	if sessionRunID == "" || ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, sessionRunIDKey{}, sessionRunID)
}

// SessionRunIDFromContext reads session_runs.id from ctx.
func SessionRunIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(sessionRunIDKey{}).(string)
	return strings.TrimSpace(v)
}
