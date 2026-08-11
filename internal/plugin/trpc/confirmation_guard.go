package plugintrpc

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"

	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type ConfirmationGuardPlugin struct {
	base basePlugin
	cfg  ConfirmationGuardConfig
}

var _ trpcplugin.Plugin = (*ConfirmationGuardPlugin)(nil)

func NewConfirmationGuardPlugin(p biz.Plugin, stats StatsRecorder, monitorBus contract.MonitorBus, lg loggateway.Logger) *ConfirmationGuardPlugin {
	var cfg ConfirmationGuardConfig
	cfg.DefaultAction = "reject"
	parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg, lg)
	return &ConfirmationGuardPlugin{base: newBasePlugin(p.Key, stats, monitorBus, lg), cfg: cfg}
}

func (c *ConfirmationGuardPlugin) Name() string { return c.base.Name() }

func (c *ConfirmationGuardPlugin) Register(r *trpcplugin.Registry) {
	r.BeforeTool(c.beforeTool)
}

// beforeTool blocks tool execution when confirmation is required.
//
// E2E-P1-10 unify: when the product confirmation gate already ran (approved,
// allow-without-channel, or explicitly handled), this plugin is a no-op.
// Hard-blocking here only applies when the product gate is NOT installed /
// did not match — so there is a single confirmation state machine in practice
// whenever tool_confirmation BeforeTool is wired.
func (c *ConfirmationGuardPlugin) beforeTool(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
	if args == nil {
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	}
	// P1-10: skip when the product callback already handled confirmation.
	if ToolConfirmHandled(ctx) {
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	}
	if MatchConfirmationGuard(c.cfg, args.ToolName, args.Arguments) {
		c.base.logger.Info("plugin.confirmation_guard.before_tool",
			"status", "blocked",
			"tool", args.ToolName,
			"default_action", c.cfg.DefaultAction,
		)
		c.base.recordEvent(ctx, "before_tool", "blocked",
			fmt.Sprintf("tool %s 需要确认（default_action=%s，未接入交互确认门）", args.ToolName, c.cfg.DefaultAction))
		msg := fmt.Sprintf("confirmation_guard: tool %q requires confirmation (enable product confirm gate for interactive approval)", args.ToolName)
		return &trpctool.BeforeToolResult{
			Context:      ctx,
			CustomResult: map[string]any{"error": msg, "blocked": true, "needs_confirm": true},
		}, nil
	}
	c.base.logger.Info("plugin.confirmation_guard.before_tool",
		"status", "success",
		"tool", args.ToolName,
		"needs_confirm", false,
	)
	c.base.record(ctx, "before_tool", "success")
	return &trpctool.BeforeToolResult{Context: ctx}, nil
}
