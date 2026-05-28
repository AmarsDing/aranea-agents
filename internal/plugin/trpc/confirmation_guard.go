package plugintrpc

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"

	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type confirmationGuardConfig = ConfirmationGuardConfig

type ConfirmationGuardPlugin struct {
	name   string
	cfg    confirmationGuardConfig
	stats  StatsRecorder
	logger *PluginSafeLogger
}

var _ trpcplugin.Plugin = (*ConfirmationGuardPlugin)(nil)

func NewConfirmationGuardPlugin(p biz.Plugin, stats StatsRecorder, bus event.Bus) *ConfirmationGuardPlugin {
	var cfg confirmationGuardConfig
	cfg.DefaultAction = "reject"
	parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg)
	return &ConfirmationGuardPlugin{name: p.Key, cfg: cfg, stats: stats, logger: NewPluginSafeLogger(p.Key, bus)}
}

func (c *ConfirmationGuardPlugin) Name() string { return c.name }

func (c *ConfirmationGuardPlugin) Register(r *trpcplugin.Registry) {
	r.BeforeTool(c.beforeTool)
}

// beforeTool blocks tool execution when confirmation is required.
func (c *ConfirmationGuardPlugin) beforeTool(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
	if args == nil {
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	}
	if MatchConfirmationGuard(c.cfg, args.ToolName, args.Arguments) {
		c.logger.Info("plugin.confirmation_guard.before_tool",
			"status", "blocked",
			"tool", args.ToolName,
			"default_action", c.cfg.DefaultAction,
		)
		c.record(ctx, "before_tool", "blocked")
		msg := fmt.Sprintf("confirmation_guard: tool %q requires confirmation", args.ToolName)
		return &trpctool.BeforeToolResult{
			Context:      ctx,
			CustomResult: map[string]any{"error": msg, "blocked": true},
		}, nil
	}
	c.logger.Info("plugin.confirmation_guard.before_tool",
		"status", "success",
		"tool", args.ToolName,
		"needs_confirm", false,
	)
	c.record(ctx, "before_tool", "success")
	return &trpctool.BeforeToolResult{Context: ctx}, nil
}

func (c *ConfirmationGuardPlugin) record(ctx context.Context, point, status string) {
	if c.stats != nil {
		c.stats.Record(ctx, c.name, point, status)
	}
}
