package callbacks_test

import (
	"context"
	"testing"

	"aranea-agents/internal/agent/callbacks"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
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

func TestNewChain_LayerOrdering(t *testing.T) {
	staticHook := callbacks.NewBeforeModelHook(4, callbacks.LayerStatic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
	semiStaticHook := callbacks.NewBeforeModelHook(4, callbacks.LayerSemiStatic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
	dynamicHook := callbacks.NewBeforeModelHook(4, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})

	// Register in reverse order
	chain := callbacks.NewChain(dynamicHook, semiStaticHook, staticHook)

	// Verify layer ordering
	layers := []callbacks.SystemLayer{}
	for _, cb := range chain.Entries() {
		if lc, ok := cb.(callbacks.LayeredCallback); ok {
			layers = append(layers, lc.Layer())
		} else {
			layers = append(layers, callbacks.LayerDynamic)
		}
	}

	if len(layers) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(layers))
	}
	if layers[0] != callbacks.LayerStatic {
		t.Errorf("entry 0: expected LayerStatic, got %v", layers[0])
	}
	if layers[1] != callbacks.LayerSemiStatic {
		t.Errorf("entry 1: expected LayerSemiStatic, got %v", layers[1])
	}
	if layers[2] != callbacks.LayerDynamic {
		t.Errorf("entry 2: expected LayerDynamic, got %v", layers[2])
	}
}

func TestNewChain_LayerAndPriorityOrdering(t *testing.T) {
	// Static priority 4, SemiStatic priority 3, Dynamic priority 1
	// Expected order: Static(4), SemiStatic(3), Dynamic(1)
	// Layer takes precedence over Priority
	hook1 := callbacks.NewBeforeModelHook(4, callbacks.LayerStatic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
	hook2 := callbacks.NewBeforeModelHook(3, callbacks.LayerSemiStatic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
	hook3 := callbacks.NewBeforeModelHook(1, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})

	chain := callbacks.NewChain(hook3, hook2, hook1)

	layers := []callbacks.SystemLayer{}
	for _, cb := range chain.Entries() {
		if lc, ok := cb.(callbacks.LayeredCallback); ok {
			layers = append(layers, lc.Layer())
		} else {
			layers = append(layers, callbacks.LayerDynamic)
		}
	}

	if layers[0] != callbacks.LayerStatic || layers[1] != callbacks.LayerSemiStatic || layers[2] != callbacks.LayerDynamic {
		t.Errorf("expected [Static, SemiStatic, Dynamic], got %v", layers)
	}
}
