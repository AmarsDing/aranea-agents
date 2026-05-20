package plugintrpc

import (
	"context"
	"sync"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type retryReflectConfig struct {
	MaxRetries               int      `json:"max_retries"`
	ExcludedTools            []string `json:"excluded_tools"`
	HighRiskToolsNeedConfirm bool     `json:"high_risk_tools_need_confirm"`
	ErrorIfRetryExceeded     bool     `json:"error_if_retry_exceeded"`
}

type RetryAndReflectPlugin struct {
	name   string
	cfg    retryReflectConfig
	stats  StatsRecorder
	logger *PluginSafeLogger

	mu      sync.Mutex
	retries map[string]int
}

var _ trpcplugin.Plugin = (*RetryAndReflectPlugin)(nil)

func NewRetryAndReflectPlugin(p biz.Plugin, stats StatsRecorder, bus event.Bus) *RetryAndReflectPlugin {
	var cfg retryReflectConfig
	cfg.MaxRetries = 3
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
		retries: make(map[string]int),
	}
}

func (r *RetryAndReflectPlugin) Name() string { return r.name }

func (r *RetryAndReflectPlugin) Register(reg *trpcplugin.Registry) {
	reg.AfterTool(r.afterTool)
}

func (r *RetryAndReflectPlugin) afterTool(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
	if args == nil || args.Error == nil {
		r.record(ctx, "after_tool", "ok")
		return &trpctool.AfterToolResult{}, nil
	}
	if toolInList(args.ToolName, r.cfg.ExcludedTools) {
		r.logger.Info("plugin.retry_reflect.after_tool", "status", "skip", "tool", args.ToolName, "reason", "excluded")
		r.record(ctx, "after_tool", "ok")
		return &trpctool.AfterToolResult{}, nil
	}
	key := retryKey(ctx, args.ToolName)
	n := r.bump(key)
	if n > r.cfg.MaxRetries {
		status := "ok"
		if r.cfg.ErrorIfRetryExceeded {
			status = "error"
			r.logger.Warn("plugin.retry_reflect.after_tool", "status", "max_retries_exceeded", "tool", args.ToolName, "retries", n)
		} else {
			r.logger.Info("plugin.retry_reflect.after_tool", "status", "max_retries_exceeded", "tool", args.ToolName, "retries", n)
		}
		r.record(ctx, "after_tool", status)
		return &trpctool.AfterToolResult{}, nil
	}
	r.logger.Info("plugin.retry_reflect.after_tool", "status", "retry_hint", "tool", args.ToolName, "attempt", n, "max_retries", r.cfg.MaxRetries, "error", args.Error.Error())
	r.record(ctx, "after_tool", "ok")
	return &trpctool.AfterToolResult{}, nil
}

func retryKey(ctx context.Context, tool string) string {
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
