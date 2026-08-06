package tools

import (
	"context"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// 2026-08-06 P0-3（20:45 会话计划失败根因之一）：plan_and_execute 内部串行
// 包含 Plan（≤60s LLM）+ Allocate（≤60s LLM）两个子阶段，外层工具装饰器却
// 套用全局 60s 默认超时 —— 子阶段预算之和 > 外层预算，外层必然中途掐断
// （plan_and_execute budget exhaustion）。修复：per-tool 超时覆盖，外层预算
// 必须 ≥ 子阶段预算之和 + 编排委派余量。

// TestTimeoutOverrideForTool_PlanAndExecute 验证 plan_and_execute 获得
// 扩展预算，且后缀匹配兼容 ToolSet 前缀（如 spirit_plan_and_execute）。
func TestTimeoutOverrideForTool_PlanAndExecute(t *testing.T) {
	t.Parallel()
	if got := timeoutOverrideForTool("plan_and_execute"); got != PlanAndExecuteTimeout {
		t.Fatalf("plan_and_execute override = %v, want %v", got, PlanAndExecuteTimeout)
	}
	if got := timeoutOverrideForTool("spirit_plan_and_execute"); got != PlanAndExecuteTimeout {
		t.Fatalf("prefixed plan_and_execute override = %v, want %v", got, PlanAndExecuteTimeout)
	}
	if got := timeoutOverrideForTool("read_file"); got != 0 {
		t.Fatalf("read_file override = %v, want 0 (no override)", got)
	}
	if got := timeoutOverrideForTool(""); got != 0 {
		t.Fatalf("empty name override = %v, want 0", got)
	}
}

// TestPlanAndExecuteTimeout_CoversSubPhaseBudgets 锁定预算不变量：
// 外层工具预算必须 ≥ 分解子超时 + 分配子超时 + 编排委派余量。
func TestPlanAndExecuteTimeout_CoversSubPhaseBudgets(t *testing.T) {
	t.Parallel()
	subPhaseSum := DecomposeLLMTimeout + AllocateLLMTimeout
	if PlanAndExecuteTimeout < subPhaseSum+30*time.Second {
		t.Fatalf("PlanAndExecuteTimeout=%v must cover decompose(%v)+allocate(%v)+30s margin",
			PlanAndExecuteTimeout, DecomposeLLMTimeout, AllocateLLMTimeout)
	}
}

// TestToolDecorator_PerToolTimeoutOverride 端到端验证：名为 plan_and_execute
// 的工具装饰后获得扩展截止预算；普通工具仍为默认 60s。
func TestToolDecorator_PerToolTimeoutOverride(t *testing.T) {
	t.Parallel()
	mkTool := func(name string) trpctool.CallableTool {
		return &deadlineProbingTool{name: name}
	}
	pe := NewToolDecorator(mkTool("plan_and_execute"), ToolDecoratorConfig{Logger: loggateway.NewNoop()})
	got, err := pe.Call(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	peBudget := got.(time.Duration)
	if peBudget < PlanAndExecuteTimeout-time.Second || peBudget > PlanAndExecuteTimeout {
		t.Fatalf("plan_and_execute budget = %v, want ~%v", peBudget, PlanAndExecuteTimeout)
	}

	plain := NewToolDecorator(mkTool("read_file"), ToolDecoratorConfig{Logger: loggateway.NewNoop()})
	got2, err := plain.Call(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	plainBudget := got2.(time.Duration)
	if plainBudget < DefaultToolTimeout-time.Second || plainBudget > DefaultToolTimeout {
		t.Fatalf("read_file budget = %v, want ~%v", plainBudget, DefaultToolTimeout)
	}
}

// deadlineProbingTool 在 Call 中报告 ctx 剩余预算，用于断言装饰器施加的超时。
type deadlineProbingTool struct {
	name string
}

func (t *deadlineProbingTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{Name: t.name}
}

func (t *deadlineProbingTool) Call(ctx context.Context, _ []byte) (any, error) {
	dl, ok := ctx.Deadline()
	if !ok {
		return time.Duration(0), nil
	}
	return time.Until(dl), nil
}
