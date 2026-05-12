package agent

import "context"

type userIDKey struct{}

func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey{}, userID)
}

func UserIDFromCtx(ctx context.Context) string {
	if ctx == nil {
		return "default_user"
	}
	if v := ctx.Value(userIDKey{}); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return "default_user"
}
