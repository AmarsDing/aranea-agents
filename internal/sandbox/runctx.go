package sandbox

import (
	"context"
	"strings"
)

// runIDKey is the context key carrying the team run ID (P2-2). The team
// Runner tags the run context at start; consumers (SessionLeases) read it to
// attribute sandbox creations to the run budget. Mirrors the
// event.WithSessionRunID pattern (chat session_runs live in a different ID
// space, so a sandbox-local key is used deliberately).
type runIDKey struct{}

// WithRunID tags ctx with the team run ID for sandbox budget attribution.
func WithRunID(ctx context.Context, runID string) context.Context {
	runID = strings.TrimSpace(runID)
	if runID == "" || ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, runIDKey{}, runID)
}

// RunIDFromContext reads the team run ID tagged by WithRunID ("" when absent).
func RunIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(runIDKey{}).(string)
	return strings.TrimSpace(v)
}
