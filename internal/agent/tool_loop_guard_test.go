package agent

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// newTestInvocationContext 构造带指定 InvocationID 的调用上下文。
func newTestInvocationContext(invocationID string) context.Context {
	inv := &trpcagent.Invocation{InvocationID: invocationID}
	return trpcagent.NewInvocationContext(context.Background(), inv)
}

// runLoopGuardTurn 模拟一次「Before → After」工具调用往返。
// 返回 nil 表示放行；返回非 nil error 表示被守卫拦截（B1：拦截以 error 形态
// 返回，框架转为 RoleTool error 消息回灌模型，且不执行真实工具与 AfterTool）。
func runLoopGuardTurn(t *testing.T, g *toolLoopGuard, ctx context.Context, tool, args string, result any, callErr error) error {
	t.Helper()
	before := g.beforeHook()
	after := g.afterHook()
	if _, err := before.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: tool, Arguments: []byte(args)}); err != nil {
		return err // 被拦截：框架跳过真实执行与 AfterTool，守卫状态不变
	}
	if _, err := after.HandleAfterTool(ctx, &trpctool.AfterToolArgs{ToolName: tool, Arguments: []byte(args), Result: result, Error: callErr}); err != nil {
		t.Fatalf("after hook error: %v", err)
	}
	return nil
}

func TestLoopGuardBlocksThirdIdenticalCall(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-loop-1")

	// 第 1、2 次相同调用放行（取证 + 确认属合理模式）。
	for i := 1; i <= 2; i++ {
		if err := runLoopGuardTurn(t, g, ctx, "gns3_exec", `{"device":"sw1","cmd":"ip link show"}`, "eth1 state DOWN", nil); err != nil {
			t.Fatalf("call %d should pass, got blocked: %v", i, err)
		}
	}
	// 第 3 次起拦截并给出纠偏消息。
	for i := 3; i <= 5; i++ {
		err := runLoopGuardTurn(t, g, ctx, "gns3_exec", `{"device":"sw1","cmd":"ip link show"}`, "eth1 state DOWN", nil)
		if err == nil {
			t.Fatalf("call %d should be blocked", i)
		}
		if !strings.HasPrefix(err.Error(), loopGuardMarker) {
			t.Fatalf("blocked error should carry loop guard marker, got %q", err.Error())
		}
		// 防回归：未饱和的拦截 error 严禁是 StopError——StopError 会被框架上抛、
		// 中断整个节点运行（B1/B4 语义：前 N-1 次普通 error 纠偏，满阈值才升级）。
		if _, ok := trpcagent.AsStopError(err); ok {
			t.Fatal("block error must NOT be a StopError before saturation")
		}
	}
}

// B4：同一节点隔离键下拦截满 loopGuardSaturatedStopThreshold 次后，
// 守卫升级返回 StopError，由框架上抛强制终止节点运行止损（2026-08-16 复验
// 实证：顽固模型连 error 纠偏也无视，反复重发烧光 16 次预算仍不推进）。
func TestLoopGuardSaturatedEscalatesToStopError(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-loop-saturated")
	args := `{"device":"sw1","cmd":"ip link show"}`

	// 2 次真实放行（打满取证额度）。
	for i := 1; i <= 2; i++ {
		if err := runLoopGuardTurn(t, g, ctx, "gns3_exec", args, "eth1 state DOWN", nil); err != nil {
			t.Fatalf("call %d should pass, got blocked: %v", i, err)
		}
	}
	// 第 3 次起拦截：前 threshold-1 次为普通 error 纠偏。
	for i := 1; i <= loopGuardSaturatedStopThreshold-1; i++ {
		err := runLoopGuardTurn(t, g, ctx, "gns3_exec", args, "eth1 state DOWN", nil)
		if err == nil {
			t.Fatalf("blocked call %d should return error", i)
		}
		if _, ok := trpcagent.AsStopError(err); ok {
			t.Fatalf("blocked call %d should be plain error before saturation, got StopError", i)
		}
		if !strings.Contains(err.Error(), "强制终止") {
			t.Fatalf("plain block error should warn about upcoming forced termination, got %q", err.Error())
		}
	}
	// 第 threshold 次拦截：升级 StopError。
	err := runLoopGuardTurn(t, g, ctx, "gns3_exec", args, "eth1 state DOWN", nil)
	if err == nil {
		t.Fatal("saturated block should return StopError")
	}
	if _, ok := trpcagent.AsStopError(err); !ok {
		t.Fatalf("saturated block should be StopError, got %v", err)
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
		if err := runLoopGuardTurn(t, g, ctx, "gns3_exec", c, "ok", nil); err != nil {
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
		if err := runLoopGuardTurn(t, g, ctx, "twin_alarm_list", `{"status":"firing"}`, out, nil); err != nil {
			t.Fatalf("polling call %d with fresh result should pass", i+1)
		}
	}
}

func TestLoopGuardFailureRetryAllowed(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-loop-4")

	// 失败重试不累计、不拦截（归熔断器治理）。
	for i := 1; i <= 4; i++ {
		if err := runLoopGuardTurn(t, g, ctx, "gns3_exec", `{"device":"sw1","cmd":"ip link show"}`, nil, context.DeadlineExceeded); err != nil {
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
	if err := runLoopGuardTurn(t, g, ctxB, "gns3_exec", `{"x":1}`, "same", nil); err != nil {
		t.Fatal("different invocation should not inherit loop state")
	}
}

func TestLoopGuardArgsCanonicalization(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-loop-6")

	// 键序/空白差异应归一化为同一签名。
	runLoopGuardTurn(t, g, ctx, "gns3_exec", `{"a":1,"b":2}`, "r", nil)
	runLoopGuardTurn(t, g, ctx, "gns3_exec", `{ "b":2, "a":1 }`, "r", nil)
	if err := runLoopGuardTurn(t, g, ctx, "gns3_exec", `{"a":1,"b":2}`, "r", nil); err == nil {
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
	err := runLoopGuardTurn(t, g, ctx, "gns3_exec", `{"device":"sw1","cmd":"ip link show"}`, real, nil)
	if err == nil {
		t.Fatal("3rd identical call should be blocked")
	}
	msg := err.Error()
	if !strings.Contains(msg, "state DOWN qlen 1000") {
		t.Fatalf("blocked message should replay last real result digest, got %q", msg)
	}
	if !strings.Contains(msg, "禁止重发") || !strings.Contains(msg, "取证已完成") {
		t.Fatalf("blocked message should affirm evidence validity and direct next action, got %q", msg)
	}
	// B1：error 形态必须明示「非工具执行失败」，防模型把拦截误读为失败而重试。
	if !strings.Contains(msg, "非执行失败") {
		t.Fatalf("blocked message should state it is not a tool failure, got %q", msg)
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
		if err := runLoopGuardTurn(t, g, ctx, c.tool, c.args, map[string]any{"ts": i, "status": "ok"}, nil); err != nil {
			t.Fatalf("call %d (%s) should pass, got blocked: %v", i+1, c.tool, err)
		}
	}
	// 第 9 次（第三轮收尾）起拦截，消息应列出轮换模式。
	err := runLoopGuardTurn(t, g, ctx, cycle[2].tool, cycle[2].args, map[string]any{"ts": 8, "status": "ok"}, nil)
	if err == nil {
		t.Fatal("9th call completing 3rd rotation should be blocked")
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, loopGuardMarker) {
		t.Fatalf("blocked error should carry loop guard marker, got %q", msg)
	}
	for _, want := range []string{"gns3_health_check", "twin_alarm_get", "gns3_exec", "输出结论"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("cycle block message should contain %q, got %q", want, msg)
		}
	}
	// 被拦调用不进窗口：模型固执重发同一步，持续被拦。
	if err := runLoopGuardTurn(t, g, ctx, cycle[2].tool, cycle[2].args, map[string]any{"ts": 9, "status": "ok"}, nil); err == nil {
		t.Fatal("retrying the blocked call should stay blocked")
	}
	// 换参数破循环：递进式干活放行。
	if err := runLoopGuardTurn(t, g, ctx, "gns3_exec", `{"device":"sw1","cmd":"ip link show eth2"}`, "eth2 UP", nil); err != nil {
		t.Fatalf("call with different args should pass, got blocked: %v", err)
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
		if err := runLoopGuardTurn(t, g, ctx, tool, args, map[string]any{"ts": i}, nil); err != nil {
			t.Fatalf("call %d should pass, got blocked", i+1)
		}
	}
	err := runLoopGuardTurn(t, g, ctx, "tool_b", `{"y":2}`, map[string]any{"ts": 5}, nil)
	if err == nil {
		t.Fatal("6th call completing 3rd AB cycle should be blocked")
	}
	if msg := err.Error(); !strings.Contains(msg, "tool_a → tool_b") {
		t.Fatalf("cycle block message should list the rotation, got %q", msg)
	}
}

// 同质连调（同签名）但结果每次变化 = 合法轮询，周期检测不得误伤
// （p=1 域归结果感知判定，循环检测只认异质基块）。
func TestLoopGuardHomogeneousPollingNotBlockedByCycle(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-loop-10")

	for i := 0; i < 8; i++ {
		if err := runLoopGuardTurn(t, g, ctx, "twin_alarm_list", `{"status":"firing"}`, map[string]any{"alarms": i}, nil); err != nil {
			t.Fatalf("polling call %d with fresh result should pass, got blocked: %v", i+1, err)
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
		if err := runLoopGuardTurn(t, g, diagnose, "gns3_exec", args, "eth1 state DOWN", nil); err != nil {
			t.Fatalf("diagnose call %d should pass, got blocked", i)
		}
	}
	// remediate 不受 diagnose 连坐：自身 2 次额度独立，前 2 次放行、第 3 次起拦截。
	for i := 1; i <= 2; i++ {
		if err := runLoopGuardTurn(t, g, remediate, "gns3_exec", args, "eth1 state DOWN", nil); err != nil {
			t.Fatalf("remediate call %d should pass (isolated from diagnose), got blocked", i)
		}
	}
	if err := runLoopGuardTurn(t, g, remediate, "gns3_exec", args, "eth1 state DOWN", nil); err == nil {
		t.Fatal("remediate 3rd identical call should be blocked")
	}
	// diagnose 侧第 3 次同样被拦（自身计数未被 remediate 干扰）。
	if err := runLoopGuardTurn(t, g, diagnose, "gns3_exec", args, "eth1 state DOWN", nil); err == nil {
		t.Fatal("diagnose 3rd identical call should be blocked")
	}
}

// 同一轮 LLM 响应并行发射多个相同调用时，BeforeTool 尚未见到 AfterTool 计数。
// in-flight 窗口只放行第一次，其余立即拦截；不计入饱和止损（避免首轮扇出直接 StopError）。
func TestLoopGuardBlocksParallelDuplicateInSameBatch(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-loop-parallel")
	before := g.beforeHook()
	after := g.afterHook()
	argsJSON := `{"device":"sw1","cmd":"ip link show"}`
	beforeArgs := &trpctool.BeforeToolArgs{ToolName: "gns3_exec", Arguments: []byte(argsJSON)}

	const n = 4
	errs := make([]error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = before.HandleBeforeTool(ctx, beforeArgs)
		}(i)
	}
	close(start)
	wg.Wait()

	allowed, blocked := 0, 0
	for _, err := range errs {
		if err == nil {
			allowed++
			continue
		}
		blocked++
		if !strings.HasPrefix(err.Error(), loopGuardMarker) {
			t.Fatalf("parallel block should carry loop guard marker, got %q", err.Error())
		}
		if !strings.Contains(err.Error(), "并行") {
			t.Fatalf("parallel block should mention parallel duplicate, got %q", err.Error())
		}
		if _, ok := trpcagent.AsStopError(err); ok {
			t.Fatal("first-batch parallel duplicates must not escalate to StopError")
		}
	}
	if allowed != 1 {
		t.Fatalf("exactly one parallel duplicate should run, got allowed=%d blocked=%d", allowed, blocked)
	}
	if blocked != n-1 {
		t.Fatalf("remaining parallel duplicates should be blocked, got allowed=%d blocked=%d", allowed, blocked)
	}

	if _, err := after.HandleAfterTool(ctx, &trpctool.AfterToolArgs{
		ToolName: "gns3_exec", Arguments: []byte(argsJSON), Result: "eth1 state DOWN",
	}); err != nil {
		t.Fatalf("after hook error: %v", err)
	}
	// in-flight 释放后，串行第二次成功调用仍放行（取证+确认额度）。
	if err := runLoopGuardTurn(t, g, ctx, "gns3_exec", argsJSON, "eth1 state DOWN", nil); err != nil {
		t.Fatalf("serial 2nd success after parallel winner should pass, got %v", err)
	}
	if err := runLoopGuardTurn(t, g, ctx, "gns3_exec", argsJSON, "eth1 state DOWN", nil); err == nil {
		t.Fatal("serial 3rd identical call should be blocked")
	}
}

func TestLoopGuardAllowsParallelDifferentArgs(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-loop-parallel-diff")
	before := g.beforeHook()
	calls := []string{`{"cmd":"a"}`, `{"cmd":"b"}`, `{"cmd":"c"}`}
	start := make(chan struct{})
	errs := make([]error, len(calls))
	var wg sync.WaitGroup
	wg.Add(len(calls))
	for i, args := range calls {
		go func(i int, args string) {
			defer wg.Done()
			<-start
			_, errs[i] = before.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{
				ToolName: "gns3_exec", Arguments: []byte(args),
			})
		}(i, args)
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("different-args parallel call %d should pass, got %v", i+1, err)
		}
	}
}

func TestLoopGuardReleasesStaleInflight(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-stale-inflight")
	before := g.beforeHook()
	args := &trpctool.BeforeToolArgs{ToolName: "web_fetch", Arguments: []byte(`{"urls":["https://x"]}`)}

	if _, err := before.HandleBeforeTool(ctx, args); err != nil {
		t.Fatalf("first call should run: %v", err)
	}
	if _, err := before.HandleBeforeTool(ctx, args); err == nil {
		t.Fatal("second call should block as in-flight parallel duplicate")
	}

	g.mu.Lock()
	e := g.entries[loopGuardInvocationKey(ctx)]
	sig := loopGuardSignature(args.ToolName, args.Arguments)
	slot := e.inflight[sig]
	slot.since = time.Now().Add(-loopGuardInflightStale - time.Second)
	e.inflight[sig] = slot
	g.mu.Unlock()

	if _, err := before.HandleBeforeTool(ctx, args); err != nil {
		t.Fatalf("stale inflight should be treated as released, got %v", err)
	}
}
