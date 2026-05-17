// Package workspace provides the workspace-scoping context key and helpers
// used by the middleware, Ent hooks, and background jobs.
package workspace

import "context"

const (
	// SystemWorkspaceID is the sentinel value used by cron / admin tasks
	// that need to bypass per-tenant filtering.
	SystemWorkspaceID = "__system__"

	// DefaultWorkspaceID is the fallback when no workspace header is supplied
	// and the installation has not yet configured multi-tenancy.
	DefaultWorkspaceID = "default"
)

type ctxKey struct{}

// WithContext returns a new context carrying the given workspace ID.
func WithContext(ctx context.Context, workspaceID string) context.Context {
	return context.WithValue(ctx, ctxKey{}, workspaceID)
}

// FromContext extracts the workspace ID from ctx.
// Returns ("", false) if the context carries no workspace ID.
func FromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(ctxKey{}).(string)
	return v, ok && v != ""
}

// IDFromContext returns the workspace ID or the default if not set.
func IDFromContext(ctx context.Context) string {
	if id, ok := FromContext(ctx); ok {
		return id
	}
	return DefaultWorkspaceID
}

// WithSystemWorkspace returns a context that bypasses workspace filtering.
// Use for cron jobs, admin background tasks, or system-level operations.
func WithSystemWorkspace(ctx context.Context) context.Context {
	return WithContext(ctx, SystemWorkspaceID)
}

// IsSystem reports whether ctx is running under the system workspace bypass.
func IsSystem(ctx context.Context) bool {
	id, ok := FromContext(ctx)
	return ok && id == SystemWorkspaceID
}
