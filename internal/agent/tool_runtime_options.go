package agent

import (
	"context"
	"sync"
	"time"

	"aranea-agents/internal/biz"
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
		RetryOn:         trpctool.DefaultRetryOn,
	}
}

// buildToolExecutionTimeout returns the per-tool execution timeout duration.
// When AgentRuntimeSettings does not specify a value (zero or negative),
// the default safety-net timeout is used. Returns 0 only when tools are disabled.
func buildToolExecutionTimeout(s *biz.AgentRuntimeSettings) time.Duration {
	if !s.ToolsEnabled {
		return 0
	}
	if s.ToolsExecutionTimeoutSec > 0 {
		return time.Duration(s.ToolsExecutionTimeoutSec) * time.Second
	}
	return defaultToolExecutionTimeout
}

// toolExecutionTimeoutHooks creates a BeforeTool + AfterTool callback pair that
// enforces a per-tool execution timeout. The BeforeTool hook injects a
// context.WithTimeout into the framework's callback pipeline; the AfterTool
// hook cleans up the cancel function to prevent goroutine leaks.
//
// This is the product-layer implementation of tool execution timeout since the
// framework does not provide a built-in timeout option.
func toolExecutionTimeoutHooks(timeout time.Duration, lg loggateway.Logger) []trpccallbacks.Callback {
	if timeout <= 0 {
		return nil
	}
	// pendingCancels stores cancel functions keyed by a unique per-invocation
	// tool call ID. The BeforeTool hook writes, the AfterTool hook reads and deletes.
	var pendingCancels sync.Map

	before := trpccallbacks.NewBeforeToolHook(0, func(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
		timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
		// Use tool call ID as key; fall back to tool name if ID is empty.
		key := toolCallCancelKey(args)
		pendingCancels.Store(key, cancel)
		return &trpctool.BeforeToolResult{Context: timeoutCtx}, nil
	})

	after := trpccallbacks.NewAfterToolHook(0, func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
		key := toolCallCancelKeyFromAfter(args)
		if v, ok := pendingCancels.LoadAndDelete(key); ok {
			cancel := v.(context.CancelFunc)
			cancel()
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
				loggateway.Str("timeout", timeout.String()),
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
