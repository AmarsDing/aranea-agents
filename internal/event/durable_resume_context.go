package event

import (
	"context"
	"strings"
)

type durableResumeKey struct{}

// DurableResumeSpec tags a turn as trpc checkpoint resume (CC-R-03 / CC-F-02).
// The orchestrator skips biz user-row persist and reuses the original turn_id.
type DurableResumeSpec struct {
	SessionRunID     string
	TurnID           string
	UserContent      string
	AgentID          string
	RuntimeRunID     string
	TrpcInvocationID string
	SessionRevision  int64
	DialogMode       string
	Provider         string
	Model            string
}

// WithDurableResume attaches durable resume metadata to ctx.
func WithDurableResume(ctx context.Context, spec DurableResumeSpec) context.Context {
	if ctx == nil {
		return ctx
	}
	spec.SessionRunID = strings.TrimSpace(spec.SessionRunID)
	spec.TurnID = strings.TrimSpace(spec.TurnID)
	if spec.SessionRunID == "" && spec.TurnID == "" {
		return ctx
	}
	return context.WithValue(ctx, durableResumeKey{}, spec)
}

// DurableResumeFromContext reads durable resume metadata from ctx.
func DurableResumeFromContext(ctx context.Context) (DurableResumeSpec, bool) {
	if ctx == nil {
		return DurableResumeSpec{}, false
	}
	v, ok := ctx.Value(durableResumeKey{}).(DurableResumeSpec)
	if !ok {
		return DurableResumeSpec{}, false
	}
	v.SessionRunID = strings.TrimSpace(v.SessionRunID)
	v.TurnID = strings.TrimSpace(v.TurnID)
	if v.SessionRunID == "" && v.TurnID == "" {
		return DurableResumeSpec{}, false
	}
	return v, true
}
