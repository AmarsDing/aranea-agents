package callbacks_test

import (
	"context"
	"testing"

	"aranea-agents/internal/agent/callbacks"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// recordingBeforeAgent records the order in which it is called.
type recordingBeforeAgent struct {
	prio   int
	record *[]string
	name   string
}

func (r *recordingBeforeAgent) Point() callbacks.CallbackPoint { return callbacks.PointBeforeAgent }
func (r *recordingBeforeAgent) Priority() int                  { return r.prio }
func (r *recordingBeforeAgent) HandleBeforeAgent(_ context.Context, _ *trpcagent.BeforeAgentArgs) (*trpcagent.BeforeAgentResult, error) {
	*r.record = append(*r.record, r.name)
	return &trpcagent.BeforeAgentResult{}, nil
}

func TestChainPriorityOrdering(t *testing.T) {
	order := []string{}
	c := callbacks.NewChain(
		&recordingBeforeAgent{prio: 20, record: &order, name: "C"},
		&recordingBeforeAgent{prio: 10, record: &order, name: "A"},
		&recordingBeforeAgent{prio: 10, record: &order, name: "B"},
	)

	ac := c.AdaptAgentCallbacks()
	_, err := ac.RunBeforeAgent(context.Background(), &trpcagent.BeforeAgentArgs{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// A and B have equal priority 10; C has priority 20 — expect A, B, C.
	if len(order) != 3 {
		t.Fatalf("expected 3 callbacks, got %d", len(order))
	}
	if order[0] != "A" || order[1] != "B" || order[2] != "C" {
		t.Fatalf("unexpected order: %v", order)
	}
}

func TestChainAppend(t *testing.T) {
	order := []string{}
	base := callbacks.NewChain(
		&recordingBeforeAgent{prio: 10, record: &order, name: "A"},
	)
	extended := base.Append(
		&recordingBeforeAgent{prio: 5, record: &order, name: "Z"},
	)

	ac := extended.AdaptAgentCallbacks()
	_, _ = ac.RunBeforeAgent(context.Background(), &trpcagent.BeforeAgentArgs{})

	if len(order) != 2 || order[0] != "Z" || order[1] != "A" {
		t.Fatalf("unexpected order after Append: %v", order)
	}
}

func TestToolRecorderCallback(t *testing.T) {
	called := false
	recorder := callbacks.NewToolRecorderCallback(0, func(_ context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
		called = true
		return &trpctool.AfterToolResult{}, nil
	})

	chain := callbacks.NewChain(recorder)
	tc := chain.AdaptToolCallbacks()

	_, err := tc.RunAfterTool(context.Background(), &trpctool.AfterToolArgs{ToolName: "test_tool"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected AfterTool callback to have been called")
	}
}
