package agent

import "context"

// rootTaskActivityIDKey is the context key for the root task activity ID.
// The ActivityProjector sets this in OnTurnStart so that downstream
// business orchestrators (spirit_team, team runner) can use it as the
// ParentActivityID for direct-publish events (team_stage, graph_stage,
// session), ensuring the frontend activity tree is correctly nested.
type rootTaskActivityIDKey struct{}

// ContextWithRootTaskActivityID returns a new context with the root task
// activity ID stored.
func ContextWithRootTaskActivityID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, rootTaskActivityIDKey{}, id)
}

// RootTaskActivityIDFromCtx extracts the root task activity ID from the
// context. Returns empty string if not set.
func RootTaskActivityIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(rootTaskActivityIDKey{}).(string); ok {
		return v
	}
	return ""
}