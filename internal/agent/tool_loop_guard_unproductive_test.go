package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// errTestToolFailed 模拟工具执行失败（空转轮记账的失败路径）。
var errTestToolFailed = errors.New("tool execution failed")

// runUnprodModelHook 执行一次空转早停 BeforeModel hook（模拟一轮 LLM 调用前），
// 返回请求中是否注入了空转引导 cue。
func runUnprodModelHook(t *testing.T, g *toolLoopGuard, ctx context.Context) bool {
	t.Helper()
	hook := g.modelHook()
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: []trpcmodel.Message{
		trpcmodel.NewSystemMessage("sys"),
		trpcmodel.NewUserMessage("hi"),
	}}}
	if _, err := hook.HandleBeforeModel(ctx, args); err != nil {
		t.Fatalf("model hook error: %v", err)
	}
	for _, m := range args.Request.Messages {
		if strings.HasPrefix(m.Content, unproductiveRoundCueMarker) {
			return true
		}
	}
	return false
}

// runUnprodToolCall 执行一次「Before → After」工具调用（带守卫记账），
// 返回 before 阶段拦截 error（nil=放行）。
func runUnprodToolCall(t *testing.T, g *toolLoopGuard, ctx context.Context, tool, args string, result any, callErr error) error {
	t.Helper()
	before := g.beforeHook()
	after := g.afterHook()
	if _, err := before.HandleBeforeTool(ctx, &trpctool.BeforeToolArgs{ToolName: tool, Arguments: []byte(args)}); err != nil {
		return err
	}
	if _, err := after.HandleAfterTool(ctx, &trpctool.AfterToolArgs{ToolName: tool, Arguments: []byte(args), Result: result, Error: callErr}); err != nil {
		t.Fatalf("after hook error: %v", err)
	}
	return nil
}

// 连续失败轮累计：满 unproductiveRoundGuideThreshold（3）轮零产出后，
// BeforeModel 注入降级引导 cue；此前不注。
func TestUnproductiveRoundsGuideInjection(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-unprod-guide")

	// 首轮 BeforeModel：尚无工具活动，不结算、不注入。
	if runUnprodModelHook(t, g, ctx) {
		t.Fatal("first model round must not inject guide")
	}
	for round := 1; round <= unproductiveRoundGuideThreshold; round++ {
		if err := runUnprodToolCall(t, g, ctx, "memberfs_list_inbox", `{"page":1}`, nil, errTestToolFailed); err != nil {
			t.Fatalf("round %d tool call should pass, got %v", round, err)
		}
		got := runUnprodModelHook(t, g, ctx)
		if round < unproductiveRoundGuideThreshold && got {
			t.Fatalf("round %d settled unprod=%d < threshold, must not inject", round, round)
		}
		if round == unproductiveRoundGuideThreshold && !got {
			t.Fatalf("round %d settled unprod=%d, guide cue must be injected", round, round)
		}
	}
}

// 任一有产出的调用即清零：失败 2 轮 → 成功 1 轮（计数归零）→ 再失败 2 轮
// 仍不注入（unprod=2 < 3）。
func TestUnproductiveRoundsProductiveResets(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-unprod-reset")

	runUnprodModelHook(t, g, ctx)
	for round := 1; round <= 2; round++ {
		if err := runUnprodToolCall(t, g, ctx, "memberfs_list_inbox", `{"page":1}`, nil, errTestToolFailed); err != nil {
			t.Fatalf("round %d should pass, got %v", round, err)
		}
		runUnprodModelHook(t, g, ctx) // unprod=1, 2（未达阈值）
	}
	// 产出轮：成功且非检索空结果 → 清零。
	if err := runUnprodToolCall(t, g, ctx, "memberfs_list_inbox", `{"page":1}`, "3 unread mails", nil); err != nil {
		t.Fatalf("productive call should pass, got %v", err)
	}
	if runUnprodModelHook(t, g, ctx) {
		t.Fatal("productive round must reset counter, no guide")
	}
	// 再失败 2 轮：unprod=1, 2，仍不达阈值。
	for round := 1; round <= 2; round++ {
		if err := runUnprodToolCall(t, g, ctx, "memberfs_list_inbox", `{"page":2}`, nil, errTestToolFailed); err != nil {
			t.Fatalf("round %d should pass, got %v", round, err)
		}
		if runUnprodModelHook(t, g, ctx) {
			t.Fatalf("unprod=%d after reset, must not inject", round)
		}
	}
}

// 检索空结果轮算空转；空结果熔断的拦截（不进 AfterTool）同样记空转。
// 3 轮「换词空检索」（第 3 轮被空结果熔断拦截）→ unprod=3 注入引导。
func TestUnproductiveRoundsEmptySearchCounts(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-unprod-empty")

	runUnprodModelHook(t, g, ctx)
	emptyResult := map[string]any{"results": []any{}}
	for round := 1; round <= 3; round++ {
		args := `{"query":"kw` + string(rune('a'+round)) + `"}`
		err := runUnprodToolCall(t, g, ctx, "memory_search", args, emptyResult, nil)
		if round <= 2 && err != nil {
			t.Fatalf("round %d empty search should pass, got %v", round, err)
		}
		if round == 3 && err == nil {
			t.Fatal("round 3 should be blocked by empty-result circuit breaker")
		}
		if round == 3 && !strings.Contains(err.Error(), "空结果") {
			t.Fatalf("round 3 block should be empty-result kind, got %q", err.Error())
		}
		got := runUnprodModelHook(t, g, ctx)
		if round < 3 && got {
			t.Fatalf("round %d must not inject yet", round)
		}
		if round == 3 && !got {
			t.Fatal("3 unproductive rounds (incl. blocked call) must inject guide")
		}
	}
}

// 纯文本轮（无工具调用）不结算：失败 2 轮（unprod=2）→ 一次无工具活动的
// BeforeModel（计数保持 2）→ 再失败 1 轮 → unprod=3 注入。
func TestUnproductiveRoundsPureTextRoundIgnored(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-unprod-text")

	runUnprodModelHook(t, g, ctx)
	for round := 1; round <= 2; round++ {
		if err := runUnprodToolCall(t, g, ctx, "memberfs_list_inbox", `{"page":1}`, nil, errTestToolFailed); err != nil {
			t.Fatalf("round %d should pass, got %v", round, err)
		}
		runUnprodModelHook(t, g, ctx)
	}
	// 纯文本续轮：roundSawTool=false，不结算（若误结算 unprod 保持 2 相同，
	// 关键是不得清零/不得注入）。
	if runUnprodModelHook(t, g, ctx) {
		t.Fatal("pure-text round must not inject (unprod=2)")
	}
	// 再失败 1 轮 → unprod=3 注入。
	if err := runUnprodToolCall(t, g, ctx, "memberfs_list_inbox", `{"page":1}`, nil, errTestToolFailed); err != nil {
		t.Fatalf("round 3 should pass, got %v", err)
	}
	if !runUnprodModelHook(t, g, ctx) {
		t.Fatal("unprod=3 must inject guide")
	}
}

// 满 unproductiveRoundBlockThreshold（5）轮零产出后，beforeHook 封锁一切
// 新工具调用（普通 error 含封锁文案）；todo_declare_blocker 豁免且其成功
// 调用视为产出（投降通道收敛后解除计数）。
func TestUnproductiveRoundsBlockThreshold(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-unprod-block")

	runUnprodModelHook(t, g, ctx)
	for round := 1; round <= unproductiveRoundBlockThreshold; round++ {
		if err := runUnprodToolCall(t, g, ctx, "memberfs_list_inbox", `{"page":1}`, nil, errTestToolFailed); err != nil {
			t.Fatalf("round %d should pass, got %v", round, err)
		}
		runUnprodModelHook(t, g, ctx)
	}
	// 第 6 轮起：任何新调用被封锁（declare_blocker 除外）。
	err := runUnprodToolCall(t, g, ctx, "memberfs_list_inbox", `{"page":2}`, nil, nil)
	if err == nil {
		t.Fatal("call after 5 unproductive rounds must be blocked")
	}
	if !strings.Contains(err.Error(), "已封锁新的工具调用") {
		t.Fatalf("block error should explain tool lockdown, got %q", err.Error())
	}
	if _, ok := trpcagent.AsStopError(err); ok {
		t.Fatal("first lockdown block must NOT be StopError before saturation")
	}
	// 豁免：todo_declare_blocker 放行且成功 → 产出轮，计数清零。
	if err := runUnprodToolCall(t, g, ctx, todoDeclareBlockerToolName, `{"reason":"inbox API down"}`, "blocker declared", nil); err != nil {
		t.Fatalf("declare_blocker must be exempt from lockdown, got %v", err)
	}
	runUnprodModelHook(t, g, ctx) // 结算产出轮 → unprod=0
	// 解除后：新调用重新放行。
	if err := runUnprodToolCall(t, g, ctx, "memberfs_list_inbox", `{"page":2}`, "1 unread mail", nil); err != nil {
		t.Fatalf("after declare_blocker the lockdown must lift, got %v", err)
	}
}

// 封锁状态下顽固重发计入 blockedCount：满 loopGuardSaturatedStopThreshold
// 次后升级 StopError 强制终止节点（共享 B4 饱和止损通道）。
func TestUnproductiveRoundsBlockSaturation(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-unprod-saturated")

	runUnprodModelHook(t, g, ctx)
	for round := 1; round <= unproductiveRoundBlockThreshold; round++ {
		if err := runUnprodToolCall(t, g, ctx, "memberfs_list_inbox", `{"page":1}`, nil, errTestToolFailed); err != nil {
			t.Fatalf("round %d should pass, got %v", round, err)
		}
		runUnprodModelHook(t, g, ctx)
	}
	// 封锁后重发：前 threshold-1 次普通 error，第 threshold 次 StopError。
	for i := 1; i <= loopGuardSaturatedStopThreshold; i++ {
		err := runUnprodToolCall(t, g, ctx, "memberfs_list_inbox", `{"page":9}`, nil, nil)
		if err == nil {
			t.Fatalf("locked-down call %d must be blocked", i)
		}
		_, isStop := trpcagent.AsStopError(err)
		if i < loopGuardSaturatedStopThreshold && isStop {
			t.Fatalf("call %d must be plain error before saturation", i)
		}
		if i == loopGuardSaturatedStopThreshold && !isStop {
			t.Fatal("call at saturation threshold must escalate to StopError")
		}
	}
}
