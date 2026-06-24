package biz

import "context"

// plannerSessionModelKey is the context key for the Spirit session's effective
// provider/model. Used by plan_and_execute to pass the session model to the
// planner/allocator for "inherit" mode LLM resolution.
type plannerSessionModelKey struct{}

// WithPlannerSessionModel injects the session's effective provider/model into
// the context. The plan_and_execute tool calls this before invoking the planner
// and allocator so they can resolve their LLM via "inherit" mode.
func WithPlannerSessionModel(ctx context.Context, provider, model string) context.Context {
	return context.WithValue(ctx, plannerSessionModelKey{}, [2]string{provider, model})
}

// PlannerSessionModelFromCtx extracts the session's effective provider/model
// from the context. Returns empty strings when not set.
func PlannerSessionModelFromCtx(ctx context.Context) (provider, model string) {
	v, ok := ctx.Value(plannerSessionModelKey{}).([2]string)
	if !ok {
		return "", ""
	}
	return v[0], v[1]
}
