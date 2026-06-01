package deferred

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

func TestDeferredCallableTool_LazyResolution(t *testing.T) {
	resolved := false
	factory := func(ctx context.Context) (trpctool.Tool, error) {
		resolved = true
		return trpcfunction.NewFunctionTool(
			func(ctx context.Context, _ struct{}) (string, error) {
				return "hello", nil
			},
			trpcfunction.WithName("test_tool"),
		), nil
	}
	decl := &trpctool.Declaration{
		Name:        "test_tool",
		Description: "A test tool",
	}
	dt := NewDeferredCallableTool(decl, factory, loggateway.NewNoop())
	if resolved {
		t.Fatal("factory should not be called on construction")
	}
	result, err := dt.Call(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resolved {
		t.Fatal("factory should be called on first Call")
	}
	if result != "hello" {
		t.Fatalf("expected hello, got %v", result)
	}
}

func TestDeferredCallableTool_Declaration(t *testing.T) {
	decl := &trpctool.Declaration{
		Name:        "test_tool",
		Description: "A test tool",
	}
	dt := NewDeferredCallableTool(decl, nil, loggateway.NewNoop())
	got := dt.Declaration()
	if got.Name != "test_tool" {
		t.Fatalf("expected test_tool, got %s", got.Name)
	}
}

func TestDeferredCallableTool_ResolveOnce(t *testing.T) {
	callCount := 0
	factory := func(ctx context.Context) (trpctool.Tool, error) {
		callCount++
		return trpcfunction.NewFunctionTool(
			func(ctx context.Context, _ struct{}) (string, error) {
				return "hello", nil
			},
			trpcfunction.WithName("test_tool"),
		), nil
	}
	decl := &trpctool.Declaration{Name: "test_tool", Description: "test"}
	dt := NewDeferredCallableTool(decl, factory, loggateway.NewNoop())
	dt.Call(context.Background(), []byte(`{}`))
	dt.Call(context.Background(), []byte(`{}`))
	if callCount != 1 {
		t.Fatalf("factory should be called once, called %d times", callCount)
	}
}

func TestDeferredCallableTool_FactoryErrorPersisted(t *testing.T) {
	factoryErr := errors.New("factory exploded")
	callCount := 0
	factory := func(ctx context.Context) (trpctool.Tool, error) {
		callCount++
		return nil, factoryErr
	}
	decl := &trpctool.Declaration{Name: "broken_tool", Description: "test"}
	dt := NewDeferredCallableTool(decl, factory, loggateway.NewNoop())

	_, err1 := dt.Call(context.Background(), []byte(`{}`))
	if err1 == nil {
		t.Fatal("expected error on first call")
	}

	_, err2 := dt.Call(context.Background(), []byte(`{}`))
	if err2 == nil {
		t.Fatal("expected error on second call")
	}
	if callCount != 2 {
		t.Fatalf("factory should be retried on each call after failure, called %d times", callCount)
	}
}

func TestDeferredCallableTool_FactoryRetryOnSuccess(t *testing.T) {
	factoryErr := errors.New("factory exploded")
	callCount := 0
	factory := func(ctx context.Context) (trpctool.Tool, error) {
		callCount++
		if callCount == 1 {
			return nil, factoryErr
		}
		return trpcfunction.NewFunctionTool(
			func(ctx context.Context, _ struct{}) (string, error) {
				return "hello", nil
			},
			trpcfunction.WithName("test_tool"),
		), nil
	}
	decl := &trpctool.Declaration{Name: "retry_tool", Description: "test"}
	dt := NewDeferredCallableTool(decl, factory, loggateway.NewNoop())

	_, err1 := dt.Call(context.Background(), []byte(`{}`))
	if err1 == nil {
		t.Fatal("expected error on first call")
	}

	result, err2 := dt.Call(context.Background(), []byte(`{}`))
	if err2 != nil {
		t.Fatalf("expected success on retry, got error: %v", err2)
	}
	if result != "hello" {
		t.Fatalf("expected hello, got %v", result)
	}
	if callCount != 2 {
		t.Fatalf("factory should be called twice (fail then succeed), called %d times", callCount)
	}

	result3, err3 := dt.Call(context.Background(), []byte(`{}`))
	if err3 != nil {
		t.Fatalf("expected success on third call, got error: %v", err3)
	}
	if result3 != "hello" {
		t.Fatalf("expected hello, got %v", result3)
	}
	if callCount != 2 {
		t.Fatalf("factory should not be called again after success, called %d times", callCount)
	}
}
