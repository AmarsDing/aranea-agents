package event

import (
	"context"
	"strings"
)

type sessionRunIDKey struct{}
type turnIDKey struct{}
type sessionDefaultContextWindowKey struct{}

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

// WithTurnID tags ctx with the chat turn / user message id for L0 snapshot correlation.
func WithTurnID(ctx context.Context, turnID string) context.Context {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" || ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, turnIDKey{}, turnID)
}

// TurnIDFromContext reads the active turn id from ctx.
func TurnIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(turnIDKey{}).(string)
	return strings.TrimSpace(v)
}

// WithSessionDefaultContextWindow tags ctx with sessions.default_context_window_tokens.
func WithSessionDefaultContextWindow(ctx context.Context, tokens int) context.Context {
	if tokens <= 0 || ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, sessionDefaultContextWindowKey{}, tokens)
}

// SessionDefaultContextWindowFromContext reads session default context window from ctx.
func SessionDefaultContextWindowFromContext(ctx context.Context) int {
	if ctx == nil {
		return 0
	}
	v, _ := ctx.Value(sessionDefaultContextWindowKey{}).(int)
	return v
}
