package agent

import (
	"context"
	"strings"
	"testing"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// newTestInvocationContext 构造带指定 InvocationID 的调用上下文。
func newTestInvocationContext(invocationID string) context.Context {
	inv := &trpcagent.Invocation{InvocationID: invocationID}
	return trpcagent.NewInvocationContext(context.Background(), inv)
}

// runLoopGuardTurn 模拟一次「Before → After」工具调用往返。
func runLoopGuardTurn(t *testing.T, g *toolLoopGuard, ctx context.Context, tool, args string, result any, callErr error) *trpctool.BeforeToolResult {
	t.Helper()
	before := g.beforeHook()
	after := g.afterHook()
	bRes, err := before.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: tool, Arguments: []byte(args)})
	if err != nil {
		t.Fatalf("before hook error: %v", err)
	}
	// 被拦截时框架跳过真实执行，AfterTool 收到的是纠偏文本。
	if bRes.CustomResult != nil {
		if _, err := after.HandleAfterTool(ctx, &trpctool.AfterToolArgs{ToolName: tool, Arguments: []byte(args), Result: bRes.CustomResult}); err != nil {
			t.Fatalf("after hook error: %v", err)
		}
		return bRes
	}
	if _, err := after.HandleAfterTool(ctx, &trpctool.AfterToolArgs{ToolName: tool, Arguments: []byte(args), Result: result, Error: callErr}); err != nil {
		t.Fatalf("after hook error: %v", err)
	}
	return bRes
}

func TestLoopGuardBlocksThirdIdenticalCall(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-loop-1")

	// 第 1、2 次相同调用放行（取证 + 确认属合理模式）。
	for i := 1; i <= 2; i++ {
		res := runLoopGuardTurn(t, g, ctx, "gns3_exec", `{"device":"sw1","cmd":"ip link show"}`, "eth1 state DOWN", nil)
		if res.CustomResult != nil {
			t.Fatalf("call %d should pass, got blocked: %v", i, res.CustomResult)
		}
	}
	// 第 3 次起拦截并给出纠偏消息。
	for i := 3; i <= 5; i++ {
		res := runLoopGuardTurn(t, g, ctx, "gns3_exec", `{"device":"sw1","cmd":"ip link show"}`, "eth1 state DOWN", nil)
		if res.CustomResult == nil {
			t.Fatalf("call %d should be blocked", i)
		}
		msg, _ := res.CustomResult.(string)
		if !strings.HasPrefix(msg, loopGuardMarker) {
			t.Fatalf("blocked result should carry loop guard marker, got %q", msg)
		}
	}
}

func TestLoopGuardAllowsDifferentArgs(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-loop-2")

	// 同工具不同参数 = 递进式干活，持续放行。
	cmds := []string{
		`{"device":"sw1","cmd":"ip link show"}`,
		`{"device":"sw1","cmd":"ip addr show"}`,
		`{"device":"sw1","cmd":"ip route"}`,
		`{"device":"sw1","cmd":"ip neigh"}`,
	}
	for i, c := range cmds {
		res := runLoopGuardTurn(t, g, ctx, "gns3_exec", c, "ok", nil)
		if res.CustomResult != nil {
			t.Fatalf("different-args call %d should pass, got blocked", i+1)
		}
	}
}

func TestLoopGuardResultChangeResets(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-loop-3")

	// 轮询场景：同参数但结果有变化 → 不算循环。
	outputs := []string{"alarms: 0", "alarms: 1", "alarms: 2", "alarms: 3"}
	for i, out := range outputs {
		res := runLoopGuardTurn(t, g, ctx, "twin_alarm_list", `{"status":"firing"}`, out, nil)
		if res.CustomResult != nil {
			t.Fatalf("polling call %d with fresh result should pass", i+1)
		}
	}
}

func TestLoopGuardFailureRetryAllowed(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-loop-4")

	// 失败重试不累计、不拦截（归熔断器治理）。
	for i := 1; i <= 4; i++ {
		res := runLoopGuardTurn(t, g, ctx, "gns3_exec", `{"device":"sw1","cmd":"ip link show"}`, nil, context.DeadlineExceeded)
		if res.CustomResult != nil {
			t.Fatalf("failure retry %d should pass", i)
		}
	}
}

func TestLoopGuardIsolatedAcrossInvocations(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctxA := newTestInvocationContext("inv-loop-5a")
	ctxB := newTestInvocationContext("inv-loop-5b")

	// invocation A 两次相同调用；invocation B 相同工具相同参数不受影响。
	for i := 1; i <= 2; i++ {
		runLoopGuardTurn(t, g, ctxA, "gns3_exec", `{"x":1}`, "same", nil)
	}
	res := runLoopGuardTurn(t, g, ctxB, "gns3_exec", `{"x":1}`, "same", nil)
	if res.CustomResult != nil {
		t.Fatal("different invocation should not inherit loop state")
	}
}

func TestLoopGuardArgsCanonicalization(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-loop-6")

	// 键序/空白差异应归一化为同一签名。
	runLoopGuardTurn(t, g, ctx, "gns3_exec", `{"a":1,"b":2}`, "r", nil)
	runLoopGuardTurn(t, g, ctx, "gns3_exec", `{ "b":2, "a":1 }`, "r", nil)
	res := runLoopGuardTurn(t, g, ctx, "gns3_exec", `{"a":1,"b":2}`, "r", nil)
	if res.CustomResult == nil {
		t.Fatal("whitespace/key-order variant should be treated as identical call and blocked on 3rd")
	}
}

func TestLoopGuardBlockMessageCarriesResultDigest(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-loop-7")

	// 真实结果含多余空白/换行；拦截消息应回放压平后的摘要，
	// 并明示「结果仍然有效」，防止模型误读为取证失败而重试（2026-08-16 复验根修）。
	real := "3: eth1: <BROADCAST,MULTICAST>  mtu 1500\n  state DOWN qlen 1000"
	for i := 1; i <= 2; i++ {
		runLoopGuardTurn(t, g, ctx, "gns3_exec", `{"device":"sw1","cmd":"ip link show"}`, real, nil)
	}
	res := runLoopGuardTurn(t, g, ctx, "gns3_exec", `{"device":"sw1","cmd":"ip link show"}`, real, nil)
	if res.CustomResult == nil {
		t.Fatal("3rd identical call should be blocked")
	}
	msg, _ := res.CustomResult.(string)
	if !strings.Contains(msg, "state DOWN qlen 1000") {
		t.Fatalf("blocked message should replay last real result digest, got %q", msg)
	}
	if !strings.Contains(msg, "禁止重发") || !strings.Contains(msg, "取证已完成") {
		t.Fatalf("blocked message should affirm evidence validity and direct next action, got %q", msg)
	}
	if strings.Contains(msg, "\n") {
		t.Fatalf("digest should be whitespace-flattened, got %q", msg)
	}
}

// 轮换循环（2026-08-16 复验实证缺口）：三工具交替 + 结果内嵌时间戳，
// p=1 判定无从触发；周期检测应在满 3 轮的收尾调用（第 9 次）起拦截。
func TestLoopGuardRotationCycleBlocked(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-loop-8")

	cycle := []struct{ tool, args string }{
		{"gns3_health_check", `{"device":"sw1"}`},
		{"twin_alarm_get", `{"alarm_id":"ALM1"}`},
		{"gns3_exec", `{"device":"sw1","cmd":"ip link show eth1"}`},
	}
	// 前 8 次（两轮 + 第三轮前两步）放行——结果每次都带新 ts，模拟轮询。
	for i := 0; i < 8; i++ {
		c := cycle[i%3]
		res := runLoopGuardTurn(t, g, ctx, c.tool, c.args, map[string]any{"ts": i, "status": "ok"}, nil)
		if res.CustomResult != nil {
			t.Fatalf("call %d (%s) should pass, got blocked: %v", i+1, c.tool, res.CustomResult)
		}
	}
	// 第 9 次（第三轮收尾）起拦截，消息应列出轮换模式。
	res := runLoopGuardTurn(t, g, ctx, cycle[2].tool, cycle[2].args, map[string]any{"ts": 8, "status": "ok"}, nil)
	if res.CustomResult == nil {
		t.Fatal("9th call completing 3rd rotation should be blocked")
	}
	msg, _ := res.CustomResult.(string)
	if !strings.HasPrefix(msg, loopGuardMarker) {
		t.Fatalf("blocked result should carry loop guard marker, got %q", msg)
	}
	for _, want := range []string{"gns3_health_check", "twin_alarm_get", "gns3_exec", "输出结论"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("cycle block message should contain %q, got %q", want, msg)
		}
	}
	// 被拦调用不进窗口：模型固执重发同一步，持续被拦。
	res = runLoopGuardTurn(t, g, ctx, cycle[2].tool, cycle[2].args, map[string]any{"ts": 9, "status": "ok"}, nil)
	if res.CustomResult == nil {
		t.Fatal("retrying the blocked call should stay blocked")
	}
	// 换参数破循环：递进式干活放行。
	res = runLoopGuardTurn(t, g, ctx, "gns3_exec", `{"device":"sw1","cmd":"ip link show eth2"}`, "eth2 UP", nil)
	if res.CustomResult != nil {
		t.Fatalf("call with different args should pass, got blocked: %v", res.CustomResult)
	}
}

// p=2 两工具轮换：ABABAB 在第 6 次（满 3 轮）起拦截。
func TestLoopGuardTwoToolCycleBlocked(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-loop-9")

	for i := 0; i < 5; i++ {
		tool, args := "tool_a", `{"x":1}`
		if i%2 == 1 {
			tool, args = "tool_b", `{"y":2}`
		}
		res := runLoopGuardTurn(t, g, ctx, tool, args, map[string]any{"ts": i}, nil)
		if res.CustomResult != nil {
			t.Fatalf("call %d should pass, got blocked", i+1)
		}
	}
	res := runLoopGuardTurn(t, g, ctx, "tool_b", `{"y":2}`, map[string]any{"ts": 5}, nil)
	if res.CustomResult == nil {
		t.Fatal("6th call completing 3rd AB cycle should be blocked")
	}
	msg, _ := res.CustomResult.(string)
	if !strings.Contains(msg, "tool_a → tool_b") {
		t.Fatalf("cycle block message should list the rotation, got %q", msg)
	}
}

// 同质连调（同签名）但结果每次变化 = 合法轮询，周期检测不得误伤
// （p=1 域归结果感知判定，循环检测只认异质基块）。
func TestLoopGuardHomogeneousPollingNotBlockedByCycle(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-loop-10")

	for i := 0; i < 8; i++ {
		res := runLoopGuardTurn(t, g, ctx, "twin_alarm_list", `{"status":"firing"}`, map[string]any{"alarms": i}, nil)
		if res.CustomResult != nil {
			t.Fatalf("polling call %d with fresh result should pass, got blocked: %v", i+1, res.CustomResult)
		}
	}
}

// 跨节点（AgentName）隔离（2026-08-16 复验实证「连坐」根修）：图谱全链路共享
// InvocationID，diagnose 的取证调用不得累计进 remediate 的重复计数——
// 各节点独立享有 loopGuardBlockThreshold 次真实取证额度。
func TestLoopGuardCountsIsolatedAcrossNodes(t *testing.T) {
	g := newToolLoopGuard(nil)
	mkCtx := func(agentName string) context.Context {
		inv := &trpcagent.Invocation{InvocationID: "inv-loop-11", AgentName: agentName}
		return trpcagent.NewInvocationContext(context.Background(), inv)
	}
	diagnose := mkCtx("ops_fault_diagnosis")
	remediate := mkCtx("ops_change_execution")
	args := `{"device":"sw1","cmd":"ip link show"}`

	// diagnose 取证 2 次（打满自身额度）。
	for i := 1; i <= 2; i++ {
		res := runLoopGuardTurn(t, g, diagnose, "gns3_exec", args, "eth1 state DOWN", nil)
		if res.CustomResult != nil {
			t.Fatalf("diagnose call %d should pass, got blocked", i)
		}
	}
	// remediate 不受 diagnose 连坐：自身 2 次额度独立，前 2 次放行、第 3 次起拦截。
	for i := 1; i <= 2; i++ {
		res := runLoopGuardTurn(t, g, remediate, "gns3_exec", args, "eth1 state DOWN", nil)
		if res.CustomResult != nil {
			t.Fatalf("remediate call %d should pass (isolated from diagnose), got blocked", i)
		}
	}
	res := runLoopGuardTurn(t, g, remediate, "gns3_exec", args, "eth1 state DOWN", nil)
	if res.CustomResult == nil {
		t.Fatal("remediate 3rd identical call should be blocked")
	}
	// diagnose 侧第 3 次同样被拦（自身计数未被 remediate 干扰）。
	res = runLoopGuardTurn(t, g, diagnose, "gns3_exec", args, "eth1 state DOWN", nil)
	if res.CustomResult == nil {
		t.Fatal("diagnose 3rd identical call should be blocked")
	}
}
