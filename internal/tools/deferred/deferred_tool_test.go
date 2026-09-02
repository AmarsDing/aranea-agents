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

// TestActivation_ParallelWorkerSessionClone 复现并行 worker 激活态丢失：
// 框架并行执行同一批 tool call 时，worker 持有克隆 session（ID 相同、
// state 隔离）。修复前 tool_load 在 worker 中的激活写入随 worker 退出丢失，
// 父 invocation（同 session）的 ToolFilter/门禁永远看不到激活态；
// 修复后进程级注册表按 session ID 共享，父 ctx 立即可见。
func TestActivation_ParallelWorkerSessionClone(t *testing.T) {
	sessionID := "sess-parallel-worker-clone"
	t.Cleanup(func() { activatedRegistry.Delete(sessionID) })

	parentSess := &trpcsession.Session{ID: sessionID}
	parentInv := trpcagent.NewInvocation()
	parentInv.Session = parentSess
	parentCtx := trpcagent.NewInvocationContext(context.Background(), parentInv)

	// worker：克隆 session（与框架 newParallelInvocationView 一致）
	workerInv := trpcagent.NewInvocation()
	workerInv.Session = parentSess.Clone()
	workerCtx := trpcagent.NewInvocationContext(context.Background(), workerInv)

	if !writeActivatedSet(workerCtx, "twin_device_search") {
		t.Fatal("writeActivatedSet on worker ctx must succeed")
	}

	// 父 ctx（真实 session）必须看到激活态
	if !isActivatedForSession(parentCtx, "twin_device_search") {
		t.Fatal("parent invocation must see activation written by parallel worker")
	}

	// ToolFilter 在父 ctx 上放行已激活的延迟工具
	mgr := NewDeferredToolManager([]DeferredToolEntry{
		{Name: "twin_device_search", BaseName: "twin_device_search", Description: "search devices", Category: "twinops"},
	})
	raw := trpcfunction.NewFunctionTool(
		func(_ context.Context, _ struct{}) (string, error) { return "ok", nil },
		trpcfunction.WithName("twin_device_search"),
		trpcfunction.WithDescription("search devices"),
	)
	dt := NewDeferredCallableTool(raw, loggateway.NewNoop())
	filter := mgr.ToolFilter()
	if !filter(parentCtx, dt) {
		t.Fatal("ToolFilter must pass activated deferred tool on parent ctx")
	}

	// 门禁在父 ctx 上放行执行
	if _, err := dt.Call(parentCtx, []byte(`{}`)); err != nil {
		t.Fatalf("activated tool must execute on parent ctx, got: %v", err)
	}

	// 未激活工具仍被过滤/拒绝（注册表不越权）
	mgr2 := NewDeferredToolManager([]DeferredToolEntry{
		{Name: "twin_alarm_query", BaseName: "twin_alarm_query", Description: "query alarms", Category: "twinops"},
	})
	raw2 := trpcfunction.NewFunctionTool(
		func(_ context.Context, _ struct{}) (string, error) { return "ok", nil },
		trpcfunction.WithName("twin_alarm_query"),
		trpcfunction.WithDescription("query alarms"),
	)
	dt2 := NewDeferredCallableTool(raw2, loggateway.NewNoop())
	if mgr2.ToolFilter()(parentCtx, dt2) {
		t.Fatal("ToolFilter must hide non-activated deferred tool")
	}
}
