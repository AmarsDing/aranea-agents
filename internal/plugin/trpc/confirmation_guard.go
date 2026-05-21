package plugintrpc

import (
	"context"

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

// beforeTool is telemetry-only; blocking is unified in agent.Callback Chain ConfirmGate.
func (c *ConfirmationGuardPlugin) beforeTool(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
	if args == nil {
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	}
	needs := MatchConfirmationGuard(c.cfg, args.ToolName, args.Arguments)
	c.logger.Info("plugin.confirmation_guard.before_tool",
		"status", "success",
		"tool", args.ToolName,
		"needs_confirm", needs,
		"default_action", c.cfg.DefaultAction,
		"enforced_by", "chain_confirm_gate",
	)
	c.record(ctx, "before_tool", "success")
	return &trpctool.BeforeToolResult{Context: ctx}, nil
}

func (c *ConfirmationGuardPlugin) record(ctx context.Context, point, status string) {
	if c.stats != nil {
		c.stats.Record(ctx, c.name, point, status)
	}
}
