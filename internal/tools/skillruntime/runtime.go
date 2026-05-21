package skillruntime

import (
	"context"
	"strings"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

// RuntimeStateTurnQueryKey is stored in invocation RuntimeState so VisibilityFilter
// can apply Layer B intent routing against the current user message.
const RuntimeStateTurnQueryKey = "skill_turn_query"

// TurnQueryFromContext reads the per-turn user query from invocation RuntimeState.
func TurnQueryFromContext(ctx context.Context) string {
	q, _ := trpcagent.GetRuntimeStateValueFromContext[string](ctx, RuntimeStateTurnQueryKey)
	return strings.TrimSpace(q)
}

// RunOptionWithTurnQuery injects the user message used for skill intent routing.
func RunOptionWithTurnQuery(query string) trpcagent.RunOption {
	q := strings.TrimSpace(query)
	if q == "" {
		return func(*trpcagent.RunOptions) {}
	}
	return trpcagent.MergeRuntimeState(map[string]any{RuntimeStateTurnQueryKey: q})
}
