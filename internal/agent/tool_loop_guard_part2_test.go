package agent

import (
	"aranea-agents/internal/tools/inverse"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

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

// 空结果熔断（2026-08-18 域 B 评测 sh-04/ab-04 超时事故根修）：
// knowledge_search 换词重试使每次签名全新，签名/轮换判定均被绕过；
// 连续空结果 ≥2 次后按工具维度（无视参数差异）熔断，引导模型直接作答。
func TestLoopGuardBlocksEmptyResultRetryAcrossDifferentQueries(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-empty-1")
	emptyResult := map[string]any{"chunks": []any{}}

	for i, q := range []string{"边界防火墙管理IP", "FW-Edge-02 IP", "firewall mgmt address"} {
		args := `{"query":"` + q + `"}`
		err := runLoopGuardTurn(t, g, ctx, "knowledge_search", args, emptyResult, nil)
		if i < 2 {
			if err != nil {
				t.Fatalf("empty call %d should pass, got blocked: %v", i+1, err)
			}
			continue
		}

		if err == nil {
			t.Fatal("3rd consecutive empty knowledge_search should be blocked")
		}
		if !strings.HasPrefix(err.Error(), loopGuardMarker) {
			t.Fatalf("empty block should carry loop guard marker, got %q", err.Error())
		}
		if !strings.Contains(err.Error(), "空结果") || !strings.Contains(err.Error(), "知识库") {
			t.Fatalf("empty block should explain knowledge base has no data, got %q", err.Error())
		}
		if !strings.Contains(err.Error(), "L2+L3 memory") || !strings.Contains(err.Error(), "编造") {
			t.Fatalf("empty block should fall back to memory and forbid invention, got %q", err.Error())
		}
		if _, ok := trpcagent.AsStopError(err); ok {
			t.Fatal("empty block must NOT be a StopError before saturation")
		}
	}
}

// 非空结果立即清零连续空计数：先空后命中再空，streak 重新起算，不误伤。
func TestLoopGuardEmptyStreakResetsOnNonEmptyResult(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-empty-2")
	emptyResult := map[string]any{"chunks": []any{}}
	hitResult := map[string]any{"chunks": []any{map[string]any{"id": "c1", "content": "命中"}}}

	if err := runLoopGuardTurn(t, g, ctx, "knowledge_search", `{"query":"a"}`, emptyResult, nil); err != nil {
		t.Fatalf("1st empty should pass: %v", err)
	}

	if err := runLoopGuardTurn(t, g, ctx, "knowledge_search", `{"query":"b"}`, hitResult, nil); err != nil {
		t.Fatalf("non-empty should pass: %v", err)
	}

	for i, q := range []string{"c", "d"} {
		if err := runLoopGuardTurn(t, g, ctx, "knowledge_search", `{"query":"`+q+`"}`, emptyResult, nil); err != nil {
			t.Fatalf("empty after reset call %d should pass: %v", i+1, err)
		}
	}
	if err := runLoopGuardTurn(t, g, ctx, "knowledge_search", `{"query":"e"}`, emptyResult, nil); err == nil {
		t.Fatal("3rd consecutive empty after reset should be blocked")
	}
}

// 失败调用不累计空 streak（失败治理归熔断器），也不触发空熔断。
func TestLoopGuardEmptyStreakIgnoresFailures(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-empty-3")
	emptyResult := map[string]any{"chunks": []any{}}

	if err := runLoopGuardTurn(t, g, ctx, "knowledge_search", `{"query":"a"}`, emptyResult, nil); err != nil {
		t.Fatalf("1st empty should pass: %v", err)
	}

	if err := runLoopGuardTurn(t, g, ctx, "knowledge_search", `{"query":"b"}`, nil, errors.New("timeout")); err != nil {
		t.Fatalf("failed call should pass: %v", err)
	}
	if err := runLoopGuardTurn(t, g, ctx, "knowledge_search", `{"query":"c"}`, emptyResult, nil); err != nil {
		t.Fatalf("2nd empty should pass: %v", err)
	}
	if err := runLoopGuardTurn(t, g, ctx, "knowledge_search", `{"query":"d"}`, emptyResult, nil); err == nil {
		t.Fatal("3rd consecutive empty (failures excluded) should be blocked")
	}
}

// 未登记工具不受空熔断约束：gns3_exec 返回空串属合法输出，不熔断。
func TestLoopGuardEmptyFuseOnlyAppliesToRegisteredTools(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-empty-4")

	for i := 1; i <= 4; i++ {
		args := `{"device":"sw1","cmd":"cmd-` + strings.Repeat("x", i) + `"}`
		if err := runLoopGuardTurn(t, g, ctx, "gns3_exec", args, "", nil); err != nil {
			t.Fatalf("unregistered tool empty-ish call %d should pass: %v", i, err)
		}
	}
}

// 空熔断拦截计入 blockedCount：顽固重发满阈值后同样升级 StopError 止损。
func TestLoopGuardEmptyBlockSaturatesToStopError(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-empty-5")
	emptyResult := map[string]any{"chunks": []any{}}

	for i, q := range []string{"a", "b"} {
		if err := runLoopGuardTurn(t, g, ctx, "knowledge_search", `{"query":"`+q+`"}`, emptyResult, nil); err != nil {
			t.Fatalf("empty call %d should pass: %v", i+1, err)
		}
	}

	for i := 1; i <= loopGuardSaturatedStopThreshold-1; i++ {
		err := runLoopGuardTurn(t, g, ctx, "knowledge_search", `{"query":"retry"}`, emptyResult, nil)
		if err == nil {
			t.Fatalf("blocked empty call %d should return error", i)
		}
		if _, ok := trpcagent.AsStopError(err); ok {
			t.Fatalf("blocked empty call %d should be plain error before saturation", i)
		}
	}
	err := runLoopGuardTurn(t, g, ctx, "knowledge_search", `{"query":"retry"}`, emptyResult, nil)
	if err == nil {
		t.Fatal("saturated empty block should return StopError")
	}
	if _, ok := trpcagent.AsStopError(err); !ok {
		t.Fatalf("saturated empty block should be StopError, got %v", err)
	}
}

func TestLoopGuardBlocksDatetimeWithChangingResult(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-datetime-volatile")
	for i := 1; i <= 2; i++ {
		if err := runLoopGuardTurn(t, g, ctx, "datetime", `{}`, fmt.Sprintf("2026-08-28T0%d:00:00Z", i), nil); err != nil {
			t.Fatalf("datetime call %d should pass, got %v", i, err)
		}
	}
	if err := runLoopGuardTurn(t, g, ctx, "datetime", `{}`, "2026-08-28T03:00:00Z", nil); err == nil {
		t.Fatal("third same-param datetime call must block even when the clock result changed")
	}
}

func TestLoopGuardBlocksForbiddenRetry(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-forbidden")
	ferr := errors.New("[RESOURCE_ACCESS/FORBIDDEN] target agent not found: ppc_strategist")
	if err := runLoopGuardTurn(t, g, ctx, "list_member_files", `{"agent_key":"ppc_strategist"}`, nil, ferr); err != nil {
		t.Fatalf("first forbidden call executes: %v", err)
	}
	if err := runLoopGuardTurn(t, g, ctx, "list_member_files", `{"agent_key":"ppc_strategist"}`, nil, ferr); err != nil {
		t.Fatalf("second forbidden call executes: %v", err)
	}
	if err := runLoopGuardTurn(t, g, ctx, "list_member_files", `{"agent_key":"ppc_strategist"}`, nil, ferr); err == nil {
		t.Fatal("third forbidden retry must be blocked")
	}
}

// P2-③（R4-Q6 根修）：HITL 拒绝后同参重发首次即拦。
// 链路序：守卫放行（beginInflight）→ 确认门禁用户拒绝（noteConfirmationOutcome
// 归还槽位+登记 denied）→ 模型同参重发 → 守卫首拦。beforeHook 单独驱动以模拟
// 「工具未执行、AfterTool 不运行」的真实短路路径。
func TestLoopGuardHITLDeniedResendBlocked(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-hitl-deny")
	args := `{"to":"a@b.c","subject":"s"}`
	before := g.beforeHook()

	if _, err := before.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: "send_email", Arguments: []byte(args)}); err != nil {
		t.Fatalf("first call should pass guard, got %v", err)
	}
	g.noteConfirmationOutcome(ctx, "send_email", []byte(args), true)

	_, err := before.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: "send_email", Arguments: []byte(args)})
	if err == nil {
		t.Fatal("resend of HITL-denied call must be blocked on first retry")
	}
	if !strings.HasPrefix(err.Error(), loopGuardMarker) {
		t.Fatalf("blocked error should carry loop guard marker, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "否决") {
		t.Fatalf("blocked error should explain the denial, got %q", err.Error())
	}
	if _, ok := trpcagent.AsStopError(err); ok {
		t.Fatal("first denied-resend block must NOT be a StopError before saturation")
	}

	if _, err := before.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: "send_email", Arguments: []byte(`{"to":"x@y.z"}`)}); err != nil {
		t.Fatalf("different args should pass, got %v", err)
	}

	ctx2 := newTestInvocationContext("inv-hitl-deny-2")
	if _, err := before.HandleBeforeTool(ctx2, &trpctool.BeforeToolArgs{ToolName: "send_email", Arguments: []byte(args)}); err != nil {
		t.Fatalf("new invocation should not inherit denial, got %v", err)
	}
}

// P2-③：确认超时只归还 inflight、不登记否决——用户授意的重发须能再走确认。
// 同时验证不归还时本用例会命中「并行重复」误拦（回归锚点）。
func TestLoopGuardHITLTimeoutReleasesInflightNoDenied(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-hitl-timeout")
	args := `{"to":"a@b.c"}`
	before := g.beforeHook()
	after := g.afterHook()

	if _, err := before.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: "send_email", Arguments: []byte(args)}); err != nil {
		t.Fatalf("first call should pass guard, got %v", err)
	}
	g.noteConfirmationOutcome(ctx, "send_email", []byte(args), false)

	if _, err := before.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: "send_email", Arguments: []byte(args)}); err != nil {
		t.Fatalf("resend after timeout should pass guard, got %v", err)
	}

	if _, err := after.HandleAfterTool(ctx, &trpctool.AfterToolArgs{ToolName: "send_email", Arguments: []byte(args), Result: "sent"}); err != nil {
		t.Fatalf("after hook error: %v", err)
	}
}

// P2-③：noteConfirmationOutcome 的边界形态——nil 守卫不 panic（装配缺失
// fail-open）；denied 登记不依赖先验 inflight（生产链序保证确认门禁只在其
// 守卫放行后运行，但合成路径下登记行为保持一致、可预期）。
func TestLoopGuardNoteConfirmationOutcomeEdge(t *testing.T) {
	var nilGuard *toolLoopGuard
	nilGuard.noteConfirmationOutcome(newTestInvocationContext("inv-hitl-nil"), "send_email", []byte(`{}`), true)

	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-hitl-noop")
	g.noteConfirmationOutcome(ctx, "send_email", []byte(`{"to":"a@b.c"}`), true)
	if _, err := g.beforeHook().HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: "send_email", Arguments: []byte(`{"to":"a@b.c"}`)}); err == nil {
		t.Fatal("denied signature must be blocked even without prior inflight")
	}

	g.noteConfirmationOutcome(context.Background(), "send_email", []byte(`{}`), true)
}

// P2-③（R4-Q6 根修）：补偿对工具同参去重阈值收紧为 1——审批通过后同参重放
// 首次即拦，且拦截消息携带方案 C 引导（指向逆工具）。
func TestLoopGuardInversePairThresholdTightened(t *testing.T) {
	inverse.Register("test_lg_inject", inverse.Spec{InverseTool: "test_lg_clear"})
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-lg-tighten")
	args := `{"link":"r1-eth0"}`

	if err := runLoopGuardTurn(t, g, ctx, "test_lg_inject", args, `{"ok":true}`, nil); err != nil {
		t.Fatalf("first inject should pass, got %v", err)
	}
	globalCompensationTracker.afterTool(ctx, &trpctool.AfterToolArgs{ToolName: "test_lg_inject", Arguments: []byte(args)})

	err := runLoopGuardTurn(t, g, ctx, "test_lg_inject", args, `{"ok":true}`, nil)
	if err == nil {
		t.Fatal("same-param replay of side-effect tool must be blocked on first repeat")
	}
	if !strings.HasPrefix(err.Error(), loopGuardMarker) {
		t.Fatalf("blocked error should carry marker, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "test_lg_clear") {
		t.Fatalf("block message should carry plan-C cue pointing to inverse tool, got %q", err.Error())
	}
}

// P2-③：逆工具自身同参重发同样首次即拦（重复 clear 幂等空转）。
func TestLoopGuardInverseToolRepeatClearBlocked(t *testing.T) {
	inverse.Register("test_lg_inject", inverse.Spec{InverseTool: "test_lg_clear"})
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-lg-clear")
	args := `{"link":"r1-eth0"}`

	if err := runLoopGuardTurn(t, g, ctx, "test_lg_clear", args, "cleared", nil); err != nil {
		t.Fatalf("first clear should pass, got %v", err)
	}
	if err := runLoopGuardTurn(t, g, ctx, "test_lg_clear", args, "cleared", nil); err == nil {
		t.Fatal("second identical clear must be blocked (threshold tightened to 1)")
	}

	if err := runLoopGuardTurn(t, g, ctx, "test_lg_clear", `{"link":"r2-eth0"}`, "cleared", nil); err != nil {
		t.Fatalf("clear with different args should pass, got %v", err)
	}
}

func TestLoopGuardErrorClass_EmptyCommandWrapped(t *testing.T) {
	wrapped := map[string]any{"result": map[string]any{"ok": false, "httpStatus": 400.0, "error": "empty command"}}
	if got := loopGuardErrorClass("gns3_exec", wrapped, nil); got != "empty_command" {
		t.Fatalf("class=%q, want empty_command", got)
	}
	if got := loopGuardErrorClass("gns3_exec", nil, errors.New("cmd 不能为空")); got != "empty_command" {
		t.Fatalf("go error class=%q, want empty_command", got)
	}
	ok := map[string]any{"device": "sw1", "output": "UP"}
	if got := loopGuardErrorClass("gns3_exec", ok, nil); got != "" {
		t.Fatalf("success must have empty class, got %q", got)
	}
}

func TestLoopGuardErrorClassFuseIgnoresArgs(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-err-class-1")
	fail := map[string]any{"result": map[string]any{"ok": false, "httpStatus": 400.0, "error": "empty command"}}

	if err := runLoopGuardTurn(t, g, ctx, "gns3_exec", `{"device":"sw1","cmd":""}`, fail, nil); err != nil {
		t.Fatalf("1st empty command should pass: %v", err)
	}
	if err := runLoopGuardTurn(t, g, ctx, "gns3_exec", `{"device":"sw1","command":""}`, fail, nil); err != nil {
		t.Fatalf("2nd empty command (different args) should pass: %v", err)
	}
	if err := runLoopGuardTurn(t, g, ctx, "gns3_exec", `{"device":"sw1","cmd":"ping 1.1.1.1"}`, fail, nil); err == nil {
		t.Fatal("3rd same error class should be blocked")
	}
}

func TestLoopGuardErrorClassClearsOnSuccess(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-err-class-2")
	fail := map[string]any{"ok": false, "error": "empty command"}
	ok := map[string]any{"device": "sw1", "output": "eth1 UP"}

	if err := runLoopGuardTurn(t, g, ctx, "gns3_exec", `{"device":"sw1","cmd":""}`, fail, nil); err != nil {
		t.Fatalf("1st fail: %v", err)
	}
	if err := runLoopGuardTurn(t, g, ctx, "gns3_exec", `{"device":"sw1","cmd":"ip link show"}`, ok, nil); err != nil {
		t.Fatalf("success must clear error class: %v", err)
	}
	if err := runLoopGuardTurn(t, g, ctx, "gns3_exec", `{"device":"sw1","command":""}`, fail, nil); err != nil {
		t.Fatalf("fail after success is a new streak: %v", err)
	}
}

func TestLoopGuardErrorClassHTTP501(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-err-class-501")
	fail := map[string]any{"ok": false, "httpStatus": 501.0, "error": "not implemented"}
	for i := 1; i <= 2; i++ {
		if err := runLoopGuardTurn(t, g, ctx, "fault_inject", fmt.Sprintf(`{"n":%d}`, i), fail, nil); err != nil {
			t.Fatalf("call %d should pass: %v", i, err)
		}
	}
	if err := runLoopGuardTurn(t, g, ctx, "fault_inject", `{"n":3}`, fail, nil); err == nil {
		t.Fatal("3rd HTTP 501 should be blocked")
	}
}
