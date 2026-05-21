package plugintrpc

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"

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
	name   string
	cfg    retryReflectConfig
	stats  StatsRecorder
	logger *PluginSafeLogger
	bus    event.Bus
	rt     *Runtime

	mu      sync.Mutex
	retries map[string]int
}

var _ trpcplugin.Plugin = (*RetryAndReflectPlugin)(nil)

func NewRetryAndReflectPlugin(p biz.Plugin, stats StatsRecorder, bus event.Bus, rt *Runtime) *RetryAndReflectPlugin {
	var cfg retryReflectConfig
	cfg.MaxRetries = 3
	cfg.TrackingScope = "invocation"
	cfg.HighRiskToolsNeedConfirm = true
	parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg)
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	name := p.Key
	return &RetryAndReflectPlugin{
		name:    name,
		cfg:     cfg,
		stats:   stats,
		logger:  NewPluginSafeLogger(name, bus),
		bus:     bus,
		rt:      rt,
		retries: make(map[string]int),
	}
}

func (r *RetryAndReflectPlugin) Name() string { return r.name }

func (r *RetryAndReflectPlugin) Register(reg *trpcplugin.Registry) {
	reg.AfterTool(r.afterTool)
}

func (r *RetryAndReflectPlugin) afterTool(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
	if args == nil || args.Error == nil {
		r.record(ctx, "after_tool", "success")
		return &trpctool.AfterToolResult{}, nil
	}
	if toolInList(args.ToolName, r.cfg.ExcludedTools) {
		r.logger.Info("plugin.retry_reflect.after_tool", "status", "skip", "tool", args.ToolName, "reason", "excluded")
		r.record(ctx, "after_tool", "success")
		return &trpctool.AfterToolResult{}, nil
	}
	if r.cfg.HighRiskToolsNeedConfirm && r.rt != nil && r.rt.ToolRequiresConfirmation(ctx, args.ToolName, args.Arguments) {
		r.logger.Info("plugin.retry_reflect.after_tool", "status", "skip", "tool", args.ToolName, "reason", "high_risk_needs_confirm")
		r.record(ctx, "after_tool", "success")
		return &trpctool.AfterToolResult{}, nil
	}
	key := retryKey(ctx, args.ToolName, r.cfg.TrackingScope)
	n := r.bump(key)
	if n > r.cfg.MaxRetries {
		status := "success"
		if r.cfg.ErrorIfRetryExceeded {
			status = "error"
			r.logger.Warn("plugin.retry_reflect.after_tool", "status", "max_retries_exceeded", "tool", args.ToolName, "retries", n)
		} else {
			r.logger.Info("plugin.retry_reflect.after_tool", "status", "max_retries_exceeded", "tool", args.ToolName, "retries", n)
		}
		r.record(ctx, "after_tool", status)
		return &trpctool.AfterToolResult{}, nil
	}

	hint := buildReflectHint(args.ToolName, args.Error.Error(), n, r.cfg.MaxRetries)
	r.logger.Info("plugin.retry_reflect.after_tool",
		"status", "reflection",
		"tool", args.ToolName,
		"attempt", n,
		"max_retries", r.cfg.MaxRetries,
		"hint", hint,
	)
	r.publishReflectEvent(ctx, args, hint, n)
	r.record(ctx, "after_tool", "success")

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

func (r *RetryAndReflectPlugin) publishReflectEvent(ctx context.Context, args *trpctool.AfterToolArgs, hint string, attempt int) {
	if r.bus == nil || args == nil {
		return
	}
	sessionID, agentKey := sessionAgentKey(ctx, nil)
	env := event.NewEnvelope("plugin.retry_reflect", r.name, sessionID)
	env.Channel = event.RouteChannel(env)
	env.Metadata = map[string]any{
		"tool":            args.ToolName,
		"agent_key":       agentKey,
		"reflection_hint": hint,
		"retry_attempt":   attempt,
		"max_retries":     r.cfg.MaxRetries,
		"error":           args.Error.Error(),
	}
	r.bus.Publish(ctx, env)
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
	r.retries[key]++
	return r.retries[key]
}

func (r *RetryAndReflectPlugin) record(ctx context.Context, point, status string) {
	if r.stats != nil {
		r.stats.Record(ctx, r.name, point, status)
	}
}
