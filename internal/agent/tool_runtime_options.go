package agent

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/deferred"
	"aranea-agents/pkg/loggateway"

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

// buildToolExecutionTimeout returns the per-tool execution timeout.
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
