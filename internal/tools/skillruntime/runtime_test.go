package skillruntime

import (
	"context"
	"testing"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

func TestRunOptionWithTurnQuery_empty(t *testing.T) {
	opt := RunOptionWithTurnQuery("  ")
	ro := &trpcagent.RunOptions{}
	opt(ro)
	if ro.RuntimeState != nil {
		t.Fatalf("expected nil RuntimeState for empty query, got %#v", ro.RuntimeState)
	}
}

func TestTurnQueryFromContext(t *testing.T) {
	inv := trpcagent.NewInvocation(
		trpcagent.WithInvocationRunOptions(trpcagent.RunOptions{
			RuntimeState: map[string]any{RuntimeStateTurnQueryKey: "read xlsx"},
		}),
	)
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)
	if got := TurnQueryFromContext(ctx); got != "read xlsx" {
		t.Fatalf("TurnQueryFromContext() = %q, want read xlsx", got)
	}
}

func TestAgentVisibilityFilter_nilRuntime(t *testing.T) {
	// nil runtime → default "{}" policy → no allow/deny lists → allow all.
	f := NewAgentVisibilityFilter(nil)
	if !f(context.Background(), trpcskill.Summary{Name: "any"}) {
		t.Fatal("nil runtime filter should allow all")
	}
}
