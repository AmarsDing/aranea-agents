package deferred

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

func createTestInnerTool() trpctool.Tool {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, _ struct{}) (string, error) {
			return "hello", nil
		},
		trpcfunction.WithName("test_tool"),
		trpcfunction.WithDescription("A test tool"),
	)
}

func TestDeferredCallableTool_Declaration(t *testing.T) {
	inner := createTestInnerTool()
	dt := NewDeferredCallableTool(inner, loggateway.NewNoop())

	got := dt.Declaration()
	if got == nil {
		t.Fatal("expected non-nil declaration")
	}
	if got.Name != "test_tool" {
		t.Fatalf("expected test_tool, got %s", got.Name)
	}
	if got.Description != "A test tool" {
		t.Fatalf("expected 'A test tool', got %s", got.Description)
	}
}

func TestDeferredCallableTool_Call_BlockedBeforeActivation(t *testing.T) {
	inner := createTestInnerTool()
	dt := NewDeferredCallableTool(inner, loggateway.NewNoop())

	_, err := dt.Call(context.Background(), []byte(`{}`))
	if err == nil {
		t.Fatal("expected error before activation")
	}
	if !strings.Contains(err.Error(), "not activated") {
		t.Fatalf("expected 'not activated' error, got: %v", err)
	}
}

func TestDeferredCallableTool_Call_AfterActivation(t *testing.T) {
	inner := createTestInnerTool()
	dt := NewDeferredCallableTool(inner, loggateway.NewNoop())

	ctx := withTestInvocation(context.Background())
	writeActivatedSet(ctx, "test_tool")

	result, err := dt.Call(ctx, []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error after activation: %v", err)
	}
	if result != "hello" {
		t.Fatalf("expected hello, got %v", result)
	}
}

func TestDeferredCallableTool_InnerTool(t *testing.T) {
	inner := createTestInnerTool()
	dt := NewDeferredCallableTool(inner, loggateway.NewNoop())

	got := dt.InnerTool()
	if got != inner {
		t.Fatal("InnerTool should return the original inner tool")
	}
}

func TestDeferredCallableTool_ShouldDefer(t *testing.T) {
	inner := createTestInnerTool()
	dt := NewDeferredCallableTool(inner, loggateway.NewNoop())

	if !dt.ShouldDefer(context.Background()) {
		t.Fatal("ShouldDefer must return true for DeferredCallableTool")
	}
}

func TestDeferredCallableTool_ImplementsDeferredTool(t *testing.T) {
	inner := createTestInnerTool()
	dt := NewDeferredCallableTool(inner, loggateway.NewNoop())

	var _ trpctool.DeferredTool = dt
}

// withTestInvocation 返回一个携带最小 invocation + session 的 context，
// 供需要 session state 的测试使用。
func withTestInvocation(ctx context.Context) context.Context {
	sess := &trpcsession.Session{}
	inv := trpcagent.NewInvocation()
	inv.Session = sess
	return trpcagent.NewInvocationContext(ctx, inv)
}
