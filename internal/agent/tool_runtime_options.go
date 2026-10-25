package agent

import (
	"context"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools"
	"aranea-agents/internal/tools/deferred"
	"aranea-agents/pkg/loggateway"

	trpccallbacks "aranea-agents/internal/agent/callbacks"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

const (
	defaultRetryMaxAttempts       = 2
	defaultRetryInitialIntervalMs = 500
	defaultRetryBackoffFactor     = 2.0
	defaultRetryMaxIntervalMs     = 5000

	// defaultToolExecutionTimeout is the safety-net timeout for a single tool execution
	// (including BeforeTool callbacks, actual execution, and AfterTool callbacks).
	// This prevents indefinite blocking when a tool or its callbacks hang.
	// Set to 0 to disable (not recommended in production).
	defaultToolExecutionTimeout = 10 * time.Minute
)

func buildToolFilter(s *biz.AgentRuntimeSettings, dm *deferred.DeferredToolManager, lg loggateway.Logger) trpctool.FilterFunc {
	var filters []trpctool.FilterFunc
	if denyList, err := biz.JSONStringList(s.ToolsDenyJSON); err != nil {
		// Fail-closed: when deny list JSON is malformed, block ALL tools
		// rather than silently allowing denied tools to be used.
		lg.Error("tools deny list JSON parse failed; FAILING CLOSED — all tools blocked",
			loggateway.StepID("agent.tool_build"),
			loggateway.Err(err),
		)
		filters = append(filters, func(_ context.Context, _ trpctool.Tool) bool { return false })
	} else if len(denyList) > 0 {
		filters = append(filters, trpctool.NewExcludeToolNamesFilter(denyList...))
	}
	if dm != nil {
		filters = append(filters, dm.ToolFilter())
	}
	if len(filters) == 0 {
		return nil
	}
	return func(ctx context.Context, t trpctool.Tool) bool {
		for _, f := range filters {
			if !f(ctx, t) {
				return false
			}
		}
		return true
	}
}

func buildToolRetryPolicy(s *biz.AgentRuntimeSettings) *trpctool.RetryPolicy {
	if !s.ToolsEnabled || !s.ToolsRetryEnabled {
		return nil
	}
	maxAttempts := s.ToolsRetryMaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultRetryMaxAttempts
	}
	initialMs := s.ToolsRetryInitialIntervalMs
	if initialMs <= 0 {
		initialMs = defaultRetryInitialIntervalMs
	}
	backoff := s.ToolsRetryBackoffFactor
	if backoff <= 0 {
		backoff = defaultRetryBackoffFactor
	}
	maxMs := s.ToolsRetryMaxIntervalMs
	if maxMs <= 0 {
		maxMs = defaultRetryMaxIntervalMs
	}
	return &trpctool.RetryPolicy{
		MaxAttempts:     maxAttempts,
		InitialInterval: time.Duration(initialMs) * time.Millisecond,
		BackoffFactor:   backoff,
		MaxInterval:     time.Duration(maxMs) * time.Millisecond,
		Jitter:          s.ToolsRetryJitter,
		RetryOn:         tools.SelectiveRetryOn,
	}
}

// toolExecutionTimeoutHooks creates a BeforeTool + AfterTool callback pair that
// enforces a per-tool execution timeout. The BeforeTool hook injects a
// context.WithTimeout into the framework's callback pipeline; the AfterTool
// hook cleans up the cancel function to prevent goroutine leaks.
//
// This is the product-layer implementation of tool execution timeout since the
// framework does not provide a built-in timeout option.
//
// P1-2：timeout 由固定值改为每调用查询（timeoutFn，生产上即
// toolExecutionTimeoutFor 的 resolver 查询闭包）——策略变更经 resolver Reload/Set
// 后即对新调用生效，无需重建 agent。timeoutFn 出口已规范化（恒 >0）。
//
// E2E-H2：AfterTool 取消钩的 priority 必须高于链上一切消费 ctx 的下游
// after-hook（记录器 50 / 输出限流器·spill 60 / 补偿·提醒 60）。链是
// (Layer,Priority) 稳定升序，旧值 0 首执行即无条件 cancel()，priority60 的
// 输出限流器收到已取消 ctx → spill 支路 PutWithRetry 见 ctx.Err() 直接弃重
// 试 → spill 灰闸 ON 部署下超大结果全量透传给模型（spill_failed/preserved
// 信封无定位符无截断），ReportSpillFailed 对取消 ctx 降级 Warn = 告警面不
// 可见。移至 90（最大现有 after 优先级 60 之上）后：下游钩子全程活 ctx，
// cancel 仍在本次工具调用的链尾同步执行，防泄漏语义不变。
func toolExecutionTimeoutHooks(timeoutFn func() time.Duration, lg loggateway.Logger) []trpccallbacks.Callback {
	if timeoutFn == nil {
		return nil
	}
	// pendingCancels stores cancel functions keyed by a unique per-invocation
	// tool call ID. The BeforeTool hook writes, the AfterTool hook reads and deletes.
	type pendingEntry struct {
		cancel  context.CancelFunc
		timeout time.Duration // 本次调用实际生效值（AfterTool 超时日志如实反映）
	}
	var pendingCancels sync.Map

	before := trpccallbacks.NewBeforeToolHook(0, func(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
		timeout := timeoutFn()
		timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
		// Use tool call ID as key; fall back to tool name if ID is empty.
		key := toolCallCancelKey(args)
		pendingCancels.Store(key, pendingEntry{cancel: cancel, timeout: timeout})
		return &trpctool.BeforeToolResult{Context: timeoutCtx}, nil
	})

	// E2E-H2: 90 = 晚于输出限流器(60)/补偿(60)/提醒(60)，保证 cancel 不再
	// 抢先撕掉下游 after-hook 的 ctx（见函数注释）。
	after := trpccallbacks.NewAfterToolHook(90, func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
		key := toolCallCancelKeyFromAfter(args)
		var applied time.Duration
		if v, ok := pendingCancels.LoadAndDelete(key); ok {
			entry := v.(pendingEntry)
			entry.cancel()
			applied = entry.timeout
		}
		// Log timeout detection for observability. Only report when the tool
		// actually failed (or produced no result) AND the context expired.
		if ctx.Err() == context.DeadlineExceeded && (args.Error != nil || args.Result == nil) {
			toolName := ""
			if args != nil && args.Declaration != nil {
				toolName = args.Declaration.Name
			}
			lg.Warn("tool execution timed out",
				loggateway.StepID("agent.tool_execution_timeout"),
				loggateway.Str("tool_name", toolName),
				loggateway.Str("timeout", applied.String()),
			)
		}
		return &trpctool.AfterToolResult{}, nil
	})

	return []trpccallbacks.Callback{before, after}
}

// toolCallCancelKey derives a unique key for the pending cancel map from BeforeToolArgs.
func toolCallCancelKey(args *trpctool.BeforeToolArgs) string {
	if args == nil {
		return ""
	}
	if id := args.ToolCallID; id != "" {
		return id
	}
	if args.Declaration != nil {
		return args.Declaration.Name
	}
	if args.ToolName != "" {
		return args.ToolName
	}
	return ""
}

// toolCallCancelKeyFromAfter derives the same key from AfterToolArgs.
// Must use identical logic to toolCallCancelKey for consistent key matching.
func toolCallCancelKeyFromAfter(args *trpctool.AfterToolArgs) string {
	if args == nil {
		return ""
	}
	if id := args.ToolCallID; id != "" {
		return id
	}
	if args.Declaration != nil {
		return args.Declaration.Name
	}
	if args.ToolName != "" {
		return args.ToolName
	}
	return ""
}
