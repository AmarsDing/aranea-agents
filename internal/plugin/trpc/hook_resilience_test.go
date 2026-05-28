package plugintrpc

import (
	"context"
	"testing"

	"aranea-agents/internal/agent/callbacks"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestRecoverHookPanic_NilRecovery(t *testing.T) {
	err := recoverHookPanic("before_model", nil, nil)
	if err != nil {
		t.Fatalf("nil recovery should return nil, got %v", err)
	}
}

func TestRecoverHookPanic_PreservesPriorError(t *testing.T) {
	priorErr := error(newTestError("original"))
	recovered := recoverHookPanic("before_model", "panic value", priorErr)
	if recovered != priorErr {
		t.Fatalf("prior error should be preserved when panic also occurs, got %v", recovered)
	}
}

func TestRecoverHookPanic_SwallowsPanicWhenNoPrior(t *testing.T) {
	recovered := recoverHookPanic("before_model", "something went wrong", nil)
	if recovered != nil {
		t.Fatalf("panic without prior error should be swallowed (return nil), got %v", recovered)
	}
}

func TestResilientHookErr_SwallowsNonBlock(t *testing.T) {
	err := resilientHookErr("after_model", newTestError("transient"))
	if err != nil {
		t.Fatalf("non-block error should be swallowed, got %v", err)
	}
}

func TestResilientHookErr_PropagatesBlockError(t *testing.T) {
	blockErr := newTestError("cost_guard: daily_budget_exceeded HOOK_BLOCKED")
	err := resilientHookErr("before_model", blockErr)
	if err != blockErr {
		t.Fatalf("blocked error should propagate, got %v", err)
	}
}

func TestWrapResilient_BeforeToolPanic(t *testing.T) {
	panicking := callbacks.NewBeforeToolHook(10, func(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
		panic("hook panic!")
	})
	wrapped := wrapResilient(panicking)
	if wrapped == nil {
		t.Fatal("expected non-nil wrapped hook")
	}
	bt, ok := wrapped.(callbacks.BeforeToolHook)
	if !ok {
		t.Fatal("expected BeforeToolHook")
	}
	res, err := bt.HandleBeforeTool(context.Background(), &trpctool.BeforeToolArgs{})
	if err != nil {
		t.Fatalf("panic should be recovered and swallowed, got error: %v", err)
	}
	_ = res
}

func TestWrapResilient_BeforeModelPanic(t *testing.T) {
	panicking := callbacks.NewBeforeModelHook(10, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		panic("model hook panic!")
	})
	wrapped := wrapResilient(panicking)
	if wrapped == nil {
		t.Fatal("expected non-nil wrapped hook")
	}
	bm, ok := wrapped.(callbacks.BeforeModelHook)
	if !ok {
		t.Fatal("expected BeforeModelHook")
	}
	res, err := bm.HandleBeforeModel(context.Background(), &trpcmodel.BeforeModelArgs{})
	if err != nil {
		t.Fatalf("panic should be recovered and swallowed, got error: %v", err)
	}
	_ = res
}

type testError struct {
	msg string
}

func (e *testError) Error() string { return e.msg }

func newTestError(msg string) *testError { return &testError{msg: msg} }
