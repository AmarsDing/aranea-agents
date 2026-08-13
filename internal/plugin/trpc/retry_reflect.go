package plugintrpc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type retryReflectConfig struct {
	MaxRetries               int      `json:"max_retries"`
	TrackingScope            string   `json:"tracking_scope"`
	ExcludedTools            []string `json:"excluded_tools"`
	HighRiskToolsNeedConfirm bool     `json:"high_risk_tools_need_confirm"`
	ErrorIfRetryExceeded     bool     `json:"error_if_retry_exceeded"`
}

type RetryAndReflectPlugin struct {
	base       basePlugin
	cfg        retryReflectConfig
	monitorBus contract.MonitorBus
	rt         *Runtime

	mu        sync.Mutex
	retries   map[string]int
	lastPurge time.Time
}

const globalRetryPurgeInterval = 1 * time.Hour

var _ trpcplugin.Plugin = (*RetryAndReflectPlugin)(nil)
var _ trpcplugin.Closer = (*RetryAndReflectPlugin)(nil)

func NewRetryAndReflectPlugin(p biz.Plugin, stats StatsRecorder, monitorBus contract.MonitorBus, rt *Runtime, lg loggateway.Logger) *RetryAndReflectPlugin {
	var cfg retryReflectConfig
	cfg.MaxRetries = 3
	cfg.TrackingScope = "invocation"
	cfg.HighRiskToolsNeedConfirm = true
	parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg, lg)
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	return &RetryAndReflectPlugin{
		base:       newBasePlugin(p.Key, stats, monitorBus, lg),
		cfg:        cfg,
		monitorBus: monitorBus,
		rt:         rt,
		retries:    make(map[string]int),
	}
}

func (r *RetryAndReflectPlugin) Name() string { return r.base.Name() }

func (r *RetryAndReflectPlugin) Register(reg *trpcplugin.Registry) {
	reg.AfterAgent(r.afterAgent)
	reg.AfterTool(r.afterTool)
}

func (r *RetryAndReflectPlugin) afterAgent(ctx context.Context, args *trpcagent.AfterAgentArgs) (*trpcagent.AfterAgentResult, error) {
	if args != nil && args.Invocation != nil && args.Invocation.InvocationID != "" {
		prefix := args.Invocation.InvocationID + ":"
		r.mu.Lock()
		for key := range r.retries {
			if strings.HasPrefix(key, prefix) {
				delete(r.retries, key)
			}
		}
		r.mu.Unlock()
	}
	return &trpcagent.AfterAgentResult{Context: ctx}, nil
}

func (r *RetryAndReflectPlugin) Close(_ context.Context) error {
	r.mu.Lock()
	r.retries = make(map[string]int)
	r.mu.Unlock()
	return nil
}

func (r *RetryAndReflectPlugin) afterTool(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
	if args == nil || args.Error == nil {
		r.base.record(ctx, "after_tool", "success")
		return &trpctool.AfterToolResult{}, nil
	}
	if toolInList(args.ToolName, r.cfg.ExcludedTools) {
		r.base.logger.Info("plugin.retry_reflect.after_tool", "status", "skip", "tool", args.ToolName, "reason", "excluded")
		r.base.record(ctx, "after_tool", "success")
		return &trpctool.AfterToolResult{}, nil
	}
	if r.cfg.HighRiskToolsNeedConfirm && r.rt != nil && r.rt.ToolRequiresConfirmation(ctx, args.ToolName, args.Arguments) {
		r.base.logger.Info("plugin.retry_reflect.after_tool", "status", "skip", "tool", args.ToolName, "reason", "high_risk_needs_confirm")
		r.base.record(ctx, "after_tool", "success")
		return &trpctool.AfterToolResult{}, nil
	}
	// P2 (2026-08-06): deterministic system errors (node not registered, unknown
	// tool, permission/config) cannot be fixed by the LLM adjusting arguments —
	// reflecting on them caused 3 wasteful retries of "orchestration.skip not
	// registered" in the 20:45 session. Propagate the raw error untouched and
	// do NOT consume retry budget.
	if isDeterministicToolError(args.Error) {
		r.base.logger.Info("plugin.retry_reflect.after_tool", "status", "skip", "tool", args.ToolName, "reason", "deterministic_error")
		r.base.record(ctx, "after_tool", "success")
		return &trpctool.AfterToolResult{}, nil
	}
	key := retryKey(ctx, args.ToolName, r.cfg.TrackingScope)
	n := r.bump(key)
	if n > r.cfg.MaxRetries {
		status := "success"
		if r.cfg.ErrorIfRetryExceeded {
			status = "error"
			r.base.logger.Warn("plugin.retry_reflect.after_tool", "status", "max_retries_exceeded", "tool", args.ToolName, "retries", n)
		} else {
			r.base.logger.Info("plugin.retry_reflect.after_tool", "status", "max_retries_exceeded", "tool", args.ToolName, "retries", n)
		}
		r.base.recordEvent(ctx, "after_tool", status,
			fmt.Sprintf("tool %s 超过最大反思重试次数（%d/%d），最近错误: %s", args.ToolName, n, r.cfg.MaxRetries, args.Error.Error()))
		return &trpctool.AfterToolResult{}, nil
	}

	hint := buildReflectHint(args.ToolName, args.Error.Error(), n, r.cfg.MaxRetries)
	r.base.logger.Info("plugin.retry_reflect.after_tool",
		"status", "reflection",
		"tool", args.ToolName,
		"attempt", n,
		"max_retries", r.cfg.MaxRetries,
		"hint", hint,
	)
	r.publishReflectEvent(ctx, args, hint, n)
	r.base.record(ctx, "after_tool", "success")

	return &trpctool.AfterToolResult{
		CustomResult: map[string]any{
			"status":          "tool_failed",
			"tool":            args.ToolName,
			"error":           args.Error.Error(),
			"reflection_hint": hint,
			"retry_attempt":   n,
			"max_retries":     r.cfg.MaxRetries,
		},
	}, nil
}

func buildReflectHint(tool, errMsg string, attempt, max int) string {
	return fmt.Sprintf(
		"Tool %q failed (attempt %d/%d): %s. Analyze the error, adjust arguments or strategy, then retry if appropriate.",
		tool, attempt, max, strings.TrimSpace(errMsg),
	)
}

// deterministicErrorKeywords match system-level failures that reflect-and-retry
// cannot fix. Deliberately excludes argument-shape errors ("invalid argument",
// "validation failed", …) because the reflection hint helps the LLM repair
// arguments on retry — classifying those as deterministic would lose that.
// Kept as a package-level var so it can be extended without touching afterTool.
var deterministicErrorKeywords = []string{
	"not registered", // graph node resolution (e.g. orchestration.skip)
	"unknown node",   // graph node resolution
	"unknown tool",   // tool resolution
	"tool not found", // tool resolution
	"no such tool",   // tool resolution
	"permission denied",
	"forbidden",
	"unauthorized",
	"not allowed",
	"access refused",
	"not implemented",
}

// isDeterministicToolError reports whether retrying the failed tool call with
// LLM-adjusted arguments cannot succeed. Case-insensitive substring match on
// the error chain's message; context.Canceled is deterministic (the invocation
// is being torn down, retrying is meaningless).
//
// Structured platform rate limits (apierror CodeRateLimit — subagent
// concurrency caps, federation/invoke quotas) are deterministic too: no
// argument adjustment lifts a concurrency cap or quota window, and the raw
// error already carries the actionable guidance. Matched by error CODE (not
// the "rate limit" substring) so plain third-party string errors like
// "rate limit exceeded" stay retriable.
func isDeterministicToolError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return true
	}
	if ae, ok := apierror.From(err); ok && ae.Code == apierror.CodeRateLimit {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, kw := range deterministicErrorKeywords {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}

func (r *RetryAndReflectPlugin) publishReflectEvent(ctx context.Context, args *trpctool.AfterToolArgs, hint string, attempt int) {
	if r.monitorBus == nil || args == nil {
		return
	}
	sessionID, agentKey := sessionAgentKey(ctx, nil)
	ev := contract.NewMonitorEvent(contract.MonitorEventTypeLog, r.base.name)
	ev.SessionID = sessionID
	ev.Message = "plugin retry reflect"
	ev.Metadata = map[string]any{
		"tool":            args.ToolName,
		"agent_key":       agentKey,
		"reflection_hint": hint,
		"retry_attempt":   attempt,
		"max_retries":     r.cfg.MaxRetries,
		"error":           args.Error.Error(),
	}
	r.monitorBus.Publish(ctx, ev)
}

func retryKey(ctx context.Context, tool, scope string) string {
	tool = strings.TrimSpace(tool)
	if strings.EqualFold(strings.TrimSpace(scope), "global") {
		return "global:" + tool
	}
	if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil && inv.InvocationID != "" {
		return inv.InvocationID + ":" + tool
	}
	return "global:" + tool
}

func (r *RetryAndReflectPlugin) bump(key string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if time.Since(r.lastPurge) > globalRetryPurgeInterval {
		r.retries = make(map[string]int)
		r.lastPurge = time.Now()
	}
	r.retries[key]++
	return r.retries[key]
}
