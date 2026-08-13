package skillruntime

import (
	"context"
	"testing"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

func TestAgentVisibilityFilter_Allow_NoPolicyAllowsAll(t *testing.T) {
	f := NewAgentVisibilityFilter(&mockRuntime{json: "{}"})
	if !f(context.Background(), trpcskill.Summary{Name: "skill-a"}) {
		t.Error("expected skill-a to be allowed with empty policy")
	}
}

func TestAgentVisibilityFilter_Allow_EmptyRuntime(t *testing.T) {
	f := NewAgentVisibilityFilter(&mockRuntime{json: ""})
	if !f(context.Background(), trpcskill.Summary{Name: "any"}) {
		t.Error("expected any skill to be allowed with empty runtime json")
	}
}

func TestAgentVisibilityFilter_Allow_SlugInAllowList(t *testing.T) {
	f := NewAgentVisibilityFilter(&mockRuntime{json: `{"allowed_slugs":["skill-a"]}`})
	if !f(context.Background(), trpcskill.Summary{Name: "skill-a"}) {
		t.Error("expected skill-a to be allowed")
	}
}

func TestAgentVisibilityFilter_Allow_SlugNotInAllowList(t *testing.T) {
	f := NewAgentVisibilityFilter(&mockRuntime{json: `{"allowed_slugs":["skill-a"]}`})
	if f(context.Background(), trpcskill.Summary{Name: "skill-b"}) {
		t.Error("expected skill-b to be hidden when not in allow-list")
	}
}

func TestAgentVisibilityFilter_Allow_DenyListBlocks(t *testing.T) {
	f := NewAgentVisibilityFilter(&mockRuntime{json: `{"denied_slugs":["skill-b"]}`})
	if f(context.Background(), trpcskill.Summary{Name: "skill-b"}) {
		t.Error("expected skill-b to be denied")
	}
	if !f(context.Background(), trpcskill.Summary{Name: "skill-a"}) {
		t.Error("expected skill-a to remain allowed")
	}
}

func TestAgentVisibilityFilter_Allow_DenyWinsOverAllow(t *testing.T) {
	f := NewAgentVisibilityFilter(&mockRuntime{
		json: `{"allowed_slugs":["skill-a","skill-b"],"denied_slugs":["skill-b"]}`,
	})
	if f(context.Background(), trpcskill.Summary{Name: "skill-b"}) {
		t.Error("expected deny to win over allow for skill-b")
	}
	if !f(context.Background(), trpcskill.Summary{Name: "skill-a"}) {
		t.Error("expected skill-a to be allowed")
	}
}

func TestAgentVisibilityFilter_Allow_PolicySlugsNormalized(t *testing.T) {
	// Policy parser normalizes slugs (lowercase/trim); summary names are
	// canonicalized defensively. Mixed case on both sides must still match.
	f := NewAgentVisibilityFilter(&mockRuntime{json: `{"allowed_slugs":[" Skill-A "]}`})
	if !f(context.Background(), trpcskill.Summary{Name: "skill-a"}) {
		t.Error("expected normalized policy slug to match lowercase summary")
	}
}

func TestAgentVisibilityFilter_Allow_CaseInsensitiveSummary(t *testing.T) {
	f := NewAgentVisibilityFilter(&mockRuntime{json: `{"denied_slugs":["skill-a"]}`})
	if f(context.Background(), trpcskill.Summary{Name: "SKILL-A"}) {
		t.Error("expected case-insensitive deny for SKILL-A")
	}
}

func TestAgentVisibilityFilter_Allow_TrimmedName(t *testing.T) {
	f := NewAgentVisibilityFilter(&mockRuntime{json: `{"denied_slugs":["skill-a"]}`})
	if f(context.Background(), trpcskill.Summary{Name: "  skill-a  "}) {
		t.Error("expected trimmed name to match deny list")
	}
}

func TestAgentVisibilityFilter_Allow_EmptyName(t *testing.T) {
	f := NewAgentVisibilityFilter(&mockRuntime{json: "{}"})
	if f(context.Background(), trpcskill.Summary{Name: "   "}) {
		t.Error("expected empty summary name to be hidden")
	}
}

func TestAgentVisibilityFilter_Allow_NilFilter(t *testing.T) {
	var f *AgentVisibilityFilter
	if !f.allow(context.Background(), trpcskill.Summary{Name: "any"}) {
		t.Error("nil filter should allow all")
	}
}

// TestAgentVisibilityFilter_StableAcrossTurnQueries is the prompt-cache
// regression guard: the filter must NOT depend on invocation state (turn
// query), so the skill overview injected into the system prompt stays
// byte-stable across turns.
func TestAgentVisibilityFilter_StableAcrossTurnQueries(t *testing.T) {
	f := NewAgentVisibilityFilter(&mockRuntime{json: `{"denied_slugs":["skill-b"]}`})

	ctxTurn1 := trpcagent.NewInvocationContext(context.Background(), trpcagent.NewInvocation(
		trpcagent.WithInvocationRunOptions(trpcagent.RunOptions{
			RuntimeState: map[string]any{RuntimeStateTurnQueryKey: "read xlsx"},
		}),
	))
	ctxTurn2 := trpcagent.NewInvocationContext(context.Background(), trpcagent.NewInvocation(
		trpcagent.WithInvocationRunOptions(trpcagent.RunOptions{
			RuntimeState: map[string]any{RuntimeStateTurnQueryKey: "send an email"},
		}),
	))

	for _, name := range []string{"skill-a", "skill-b", "skill-c"} {
		s := trpcskill.Summary{Name: name}
		got1 := f(ctxTurn1, s)
		got2 := f(ctxTurn2, s)
		if got1 != got2 {
			t.Errorf("filter result for %q changed across turn queries: %v -> %v", name, got1, got2)
		}
	}
	if !f(ctxTurn1, trpcskill.Summary{Name: "skill-a"}) {
		t.Error("expected skill-a visible regardless of turn query")
	}
	if f(ctxTurn1, trpcskill.Summary{Name: "skill-b"}) {
		t.Error("expected skill-b denied regardless of turn query")
	}
}
