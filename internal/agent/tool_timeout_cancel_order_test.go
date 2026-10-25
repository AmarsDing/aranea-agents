package agent

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// E2E-H2：timeout 取消钩必须晚于链上一切消费 ctx 的下游 after-hook（输出限
// 流器 spill=60 / 补偿=60 / 提醒=60）。链 (Layer,Priority) 稳定升序；旧
// priority 0 首执行即 cancel()，priority60 的输出限流器收到已取消 ctx →
// spill 支路 PutWithRetry 见 ctx.Err() 弃重试 → spill 灰闸 ON 下超大结果
// 全量透传模型（spill_failed/preserved 无定位符无截断）。
func TestTimeoutCancelHookRunsAfterCtxConsumers(t *testing.T) {
	hooks := toolExecutionTimeoutHooks(func() time.Duration { return time.Minute }, loggateway.NewNoop())
	if len(hooks) != 2 {
		t.Fatalf("expect before+after hook pair, got %d", len(hooks))
	}
	before, ok := hooks[0].(callbacks.BeforeToolHook)
	if !ok {
		t.Fatalf("hooks[0] must be BeforeToolHook, got %T", hooks[0])
	}

	// probe 复刻 priority-60 输出限流器的 ctx 消费位。
	var probeCtxErr error
	probe := callbacks.NewAfterToolHook(60, func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
		probeCtxErr = ctx.Err()
		return &trpctool.AfterToolResult{}, nil
	})
	all := append([]callbacks.Callback{}, hooks...)
	all = append(all, probe)
	chain := callbacks.NewChain(all...)

	beforeRes, err := before.HandleBeforeTool(context.Background(), &trpctool.BeforeToolArgs{ToolCallID: "c1"})
	if err != nil {
		t.Fatalf("before hook: %v", err)
	}
	if beforeRes == nil || beforeRes.Context == nil {
		t.Fatal("before hook must inject a timeout ctx")
	}
	toolCtx := beforeRes.Context

	afterArgs := &trpctool.AfterToolArgs{ToolCallID: "c1"}
	ranAfter := 0
	for _, cb := range chain.Entries() {
		h, ok := cb.(callbacks.AfterToolHook)
		if !ok {
			continue
		}
		ranAfter++
		if _, err := h.HandleAfterTool(toolCtx, afterArgs); err != nil {
			t.Fatalf("after hook (%d): %v", ranAfter, err)
		}
	}
	if ranAfter < 2 {
		t.Fatalf("expect timeout-cancel + probe after hooks to run, ran %d", ranAfter)
	}

	if probeCtxErr != nil {
		t.Fatalf("priority-60 consumer must see a LIVE ctx (E2E-H2 regression: cancel fired first), got %v", probeCtxErr)
	}
	// 链尾 cancel 仍同步执行——timeoutCtx 生命周期不泄漏。
	if err := toolCtx.Err(); err != context.Canceled {
		t.Fatalf("timeout ctx must be canceled at chain tail, got %v", err)
	}
}

// E2E-H2 拨号钉：timeout 取消钩的 priority 不得低于 61（> 输出限流器 60）。
// 未来新增 >60 的 ctx 消费型 after-hook 时，此钉要求同步抬升取消档。
func TestTimeoutCancelHookPriorityAboveCtxConsumers(t *testing.T) {
	hooks := toolExecutionTimeoutHooks(func() time.Duration { return time.Second }, loggateway.NewNoop())
	chain := callbacks.NewChain(hooks...)
	for _, cb := range chain.Entries() {
		if h, ok := cb.(callbacks.AfterToolHook); ok {
			if h.Priority() <= 60 {
				t.Fatalf("timeout-cancel after hook priority = %d, must exceed the output limiter's 60", h.Priority())
			}
		}
	}
}
