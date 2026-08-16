package agent

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"aranea-agents/internal/tools/inverse"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// compensationTestCtx 构造携带 invocation 的 ctx；sessionID 为空时退化为
// invocation 作用域（验证 scope 降级路径）。
func compensationTestCtx(sessionID, invID, agentName string) context.Context {
	inv := trpcagent.NewInvocation()
	inv.AgentName = agentName
	inv.InvocationID = invID
	if sessionID != "" {
		inv.Session = &trpcsession.Session{ID: sessionID}
	}
	return trpcagent.NewInvocationContext(context.Background(), inv)
}

func compensationPendingCount(t *testing.T, tracker *compensationTracker, scope string) int {
	t.Helper()
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	return len(tracker.journals[scope])
}

func TestCompensationTracker_ForwardThenInverseSettles(t *testing.T) {
	inverse.Register("test_comp_inject", inverse.Spec{InverseTool: "test_comp_clear"})
	tracker := newCompensationTracker(loggateway.NewNoop())
	ctx := compensationTestCtx("sess-1", "inv-1", "ops")
	args := []byte(`{"target":"n1"}`)

	tracker.afterTool(ctx, &trpctool.AfterToolArgs{ToolName: "test_comp_inject", Arguments: args})
	if got := compensationPendingCount(t, tracker, "sess|sess-1"); got != 1 {
		t.Fatalf("pending after forward = %d, want 1", got)
	}

	// 参数不匹配逆工具调用不核销。
	tracker.afterTool(ctx, &trpctool.AfterToolArgs{ToolName: "test_comp_clear", Arguments: []byte(`{"target":"n2"}`)})
	if got := compensationPendingCount(t, tracker, "sess|sess-1"); got != 1 {
		t.Fatalf("pending after mismatched inverse = %d, want 1", got)
	}

	// 参数匹配（恒等映射：逆参数=正参数）核销成功。
	tracker.afterTool(ctx, &trpctool.AfterToolArgs{ToolName: "test_comp_clear", Arguments: args})
	if got := compensationPendingCount(t, tracker, "sess|sess-1"); got != 0 {
		t.Fatalf("pending after matched inverse = %d, want 0", got)
	}
}

func TestCompensationTracker_MapArgsApplied(t *testing.T) {
	inverse.Register("test_comp_map_fwd", inverse.Spec{
		InverseTool: "test_comp_map_inv",
		MapArgs: func(args []byte) ([]byte, error) {
			return []byte(`{"mapped":true}`), nil
		},
	})
	tracker := newCompensationTracker(loggateway.NewNoop())
	ctx := compensationTestCtx("sess-2", "inv-2", "ops")

	tracker.afterTool(ctx, &trpctool.AfterToolArgs{ToolName: "test_comp_map_fwd", Arguments: []byte(`{"raw":1}`)})
	if got := compensationPendingCount(t, tracker, "sess|sess-2"); got != 1 {
		t.Fatalf("pending = %d, want 1", got)
	}
	// 核销按映射后的参数匹配。
	tracker.afterTool(ctx, &trpctool.AfterToolArgs{ToolName: "test_comp_map_inv", Arguments: []byte(`{"raw":1}`)})
	if got := compensationPendingCount(t, tracker, "sess|sess-2"); got != 1 {
		t.Fatalf("pending after unmapped inverse = %d, want 1", got)
	}
	tracker.afterTool(ctx, &trpctool.AfterToolArgs{ToolName: "test_comp_map_inv", Arguments: []byte(`{"mapped":true}`)})
	if got := compensationPendingCount(t, tracker, "sess|sess-2"); got != 0 {
		t.Fatalf("pending after mapped inverse = %d, want 0", got)
	}
}

func TestCompensationTracker_MapArgsErrorSkips(t *testing.T) {
	inverse.Register("test_comp_err_fwd", inverse.Spec{
		InverseTool: "test_comp_err_inv",
		MapArgs: func(args []byte) ([]byte, error) {
			return nil, errors.New("bad args")
		},
	})
	tracker := newCompensationTracker(loggateway.NewNoop())
	ctx := compensationTestCtx("sess-3", "inv-3", "ops")

	tracker.afterTool(ctx, &trpctool.AfterToolArgs{ToolName: "test_comp_err_fwd", Arguments: []byte(`{}`)})
	if got := compensationPendingCount(t, tracker, "sess|sess-3"); got != 0 {
		t.Fatalf("pending = %d, want 0 (MapArgs error must skip tracking)", got)
	}
}

func TestCompensationTracker_ErrorResultIgnored(t *testing.T) {
	inverse.Register("test_comp_fail_inject", inverse.Spec{InverseTool: "test_comp_fail_clear"})
	tracker := newCompensationTracker(loggateway.NewNoop())
	ctx := compensationTestCtx("sess-4", "inv-4", "ops")

	tracker.afterTool(ctx, &trpctool.AfterToolArgs{
		ToolName:  "test_comp_fail_inject",
		Arguments: []byte(`{}`),
		Error:     errors.New("tool failed"),
	})
	if got := compensationPendingCount(t, tracker, "sess|sess-4"); got != 0 {
		t.Fatalf("pending = %d, want 0 (failed call out of scope)", got)
	}
}

func TestCompensationTracker_NoInvocationIgnored(t *testing.T) {
	inverse.Register("test_comp_noctx_inject", inverse.Spec{InverseTool: "test_comp_noctx_clear"})
	tracker := newCompensationTracker(loggateway.NewNoop())

	tracker.afterTool(context.Background(), &trpctool.AfterToolArgs{
		ToolName:  "test_comp_noctx_inject",
		Arguments: []byte(`{}`),
	})
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if len(tracker.journals) != 0 {
		t.Fatalf("journals = %d, want 0 (no invocation → fail-open)", len(tracker.journals))
	}
}

func TestCompensationTracker_InvocationScopeFallback(t *testing.T) {
	inverse.Register("test_comp_inv_inject", inverse.Spec{InverseTool: "test_comp_inv_clear"})
	tracker := newCompensationTracker(loggateway.NewNoop())
	ctx := compensationTestCtx("", "inv-9", "ops") // 无 session

	tracker.afterTool(ctx, &trpctool.AfterToolArgs{ToolName: "test_comp_inv_inject", Arguments: []byte(`{}`)})
	if got := compensationPendingCount(t, tracker, "inv|inv-9"); got != 1 {
		t.Fatalf("pending under invocation scope = %d, want 1", got)
	}
}

func TestCompensationTracker_CrossInvocationSameSessionSettles(t *testing.T) {
	inverse.Register("test_comp_xinv_inject", inverse.Spec{InverseTool: "test_comp_xinv_clear"})
	tracker := newCompensationTracker(loggateway.NewNoop())
	args := []byte(`{"link":"a-b"}`)

	// inject 在 invocation A，clear 在同会话的 invocation B（graph 跨节点补偿）。
	tracker.afterTool(compensationTestCtx("sess-x", "inv-a", "ops"), &trpctool.AfterToolArgs{ToolName: "test_comp_xinv_inject", Arguments: args})
	tracker.afterTool(compensationTestCtx("sess-x", "inv-b", "verify"), &trpctool.AfterToolArgs{ToolName: "test_comp_xinv_clear", Arguments: args})
	if got := compensationPendingCount(t, tracker, "sess|sess-x"); got != 0 {
		t.Fatalf("pending = %d, want 0 (session scope spans invocations)", got)
	}
}

func TestCompensationTracker_SweepAlertsOnceOnTimeout(t *testing.T) {
	inverse.Register("test_comp_sweep_inject", inverse.Spec{InverseTool: "test_comp_sweep_clear"})
	tracker := newCompensationTracker(loggateway.NewNoop())
	tracker.alertAfter = -time.Minute // 立即可判超时
	ctx := compensationTestCtx("sess-5", "inv-5", "ops")

	tracker.afterTool(ctx, &trpctool.AfterToolArgs{ToolName: "test_comp_sweep_inject", Arguments: []byte(`{"p":1}`)})

	tracker.sweep()
	tracker.mu.Lock()
	p := tracker.journals["sess|sess-5"][loopGuardSignature("test_comp_sweep_clear", []byte(`{"p":1}`))]
	tracker.mu.Unlock()
	if p == nil || !p.alerted {
		t.Fatal("timed-out pending must be marked alerted after sweep")
	}

	// 二次 sweep 不重复告警（alerted 已置位，无 panic/无新增即语义成立）。
	tracker.sweep()
	tracker.mu.Lock()
	stillThere := tracker.journals["sess|sess-5"][loopGuardSignature("test_comp_sweep_clear", []byte(`{"p":1}`))]
	tracker.mu.Unlock()
	if stillThere == nil {
		t.Fatal("alerted pending must remain until settled")
	}

	// 告警后核销仍生效。
	tracker.afterTool(ctx, &trpctool.AfterToolArgs{ToolName: "test_comp_sweep_clear", Arguments: []byte(`{"p":1}`)})
	if got := compensationPendingCount(t, tracker, "sess|sess-5"); got != 0 {
		t.Fatalf("pending = %d, want 0", got)
	}
}

func TestCompensationTracker_OverflowDropsOldest(t *testing.T) {
	inverse.Register("test_comp_ovf_inject", inverse.Spec{InverseTool: "test_comp_ovf_clear"})
	tracker := newCompensationTracker(loggateway.NewNoop())
	ctx := compensationTestCtx("sess-6", "inv-6", "ops")

	first := []byte(`{"i":0}`)
	tracker.afterTool(ctx, &trpctool.AfterToolArgs{ToolName: "test_comp_ovf_inject", Arguments: first})
	// 拉开时间戳，保证 first 严格最旧（Windows 时钟粒度粗，快速连插会并列，
	// 最旧选取在等值时间戳下不确定）。
	time.Sleep(20 * time.Millisecond)
	// 填满 journal（首条 at 最早，超限时应被丢弃）。
	for i := 1; i <= compensateJournalMax; i++ {
		args := []byte(fmt.Sprintf(`{"i":%d}`, i))
		tracker.afterTool(ctx, &trpctool.AfterToolArgs{ToolName: "test_comp_ovf_inject", Arguments: args})
	}
	if got := compensationPendingCount(t, tracker, "sess|sess-6"); got != compensateJournalMax {
		t.Fatalf("pending = %d, want cap %d", got, compensateJournalMax)
	}
	// 最旧一条已被丢弃：用首条参数核销是 no-op。
	tracker.afterTool(ctx, &trpctool.AfterToolArgs{ToolName: "test_comp_ovf_clear", Arguments: first})
	if got := compensationPendingCount(t, tracker, "sess|sess-6"); got != compensateJournalMax {
		t.Fatalf("pending = %d, want %d (oldest already evicted)", got, compensateJournalMax)
	}
}

func TestCompensationTrackerHook_ReturnsContext(t *testing.T) {
	hook := compensationTrackerAfterHook(nil)
	ctx := compensationTestCtx("sess-7", "inv-7", "ops")
	res, err := hook.HandleAfterTool(ctx, &trpctool.AfterToolArgs{ToolName: "unrelated_tool", Arguments: []byte(`{}`)})
	if err != nil {
		t.Fatalf("hook error = %v", err)
	}
	if res == nil || res.Context != ctx {
		t.Fatal("hook must pass through ctx")
	}
}
