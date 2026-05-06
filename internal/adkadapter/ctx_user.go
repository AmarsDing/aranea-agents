package adkadapter

import "context"

type userIDKey struct{}

// WithUserID stores a logical user id for ADK Runner (defaults to [DefaultUserID]).
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey{}, userID)
}

// UserIDFromCtx returns the user id for Runner.Run, or [DefaultUserID].
func UserIDFromCtx(ctx context.Context) string {
	if ctx == nil {
		return DefaultUserID
	}
	if v := ctx.Value(userIDKey{}); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return DefaultUserID
}
