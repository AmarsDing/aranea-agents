package callbacks_test

import (
	"context"
	"testing"

	"aranea-agents/internal/agent/callbacks"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestBeforeAfterModelHook(t *testing.T) {
	beforeCalled := false
	afterCalled := false
	chain := callbacks.NewChain(
		callbacks.NewBeforeModelHook(0, callbacks.LayerDynamic, func(_ context.Context, _ *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
			beforeCalled = true
			return &trpcmodel.BeforeModelResult{}, nil
		}),
		callbacks.NewAfterModelHook(0, func(_ context.Context, _ *trpcmodel.AfterModelArgs) (*trpcmodel.AfterModelResult, error) {
			afterCalled = true
			return &trpcmodel.AfterModelResult{}, nil
		}),
	)
	mc := chain.AdaptModelCallbacks()
	if _, err := mc.RunBeforeModel(context.Background(), &trpcmodel.BeforeModelArgs{}); err != nil {
		t.Fatal(err)
	}
	if _, err := mc.RunAfterModel(context.Background(), &trpcmodel.AfterModelArgs{}); err != nil {
		t.Fatal(err)
	}
	if !beforeCalled || !afterCalled {
		t.Fatalf("before=%v after=%v", beforeCalled, afterCalled)
	}
}
