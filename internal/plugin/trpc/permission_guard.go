package plugintrpc

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type permissionGuardConfig struct {
	DenyTools       []string `json:"deny_tools"`
	ConfirmTools    []string `json:"confirm_tools"`
	AgentAllowlist  []string `json:"agent_allowlist"`
}

type PermissionGuardPlugin struct {
	name   string
	cfg    permissionGuardConfig
	stats  StatsRecorder
	logger *PluginSafeLogger
}

var _ trpcplugin.Plugin = (*PermissionGuardPlugin)(nil)

func NewPermissionGuardPlugin(p biz.Plugin, stats StatsRecorder, bus event.Bus) *PermissionGuardPlugin {
	var cfg permissionGuardConfig
	parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg)
	return &PermissionGuardPlugin{name: p.Key, cfg: cfg, stats: stats, logger: NewPluginSafeLogger(p.Key, bus)}
}

func (p *PermissionGuardPlugin) Name() string { return p.name }

func (p *PermissionGuardPlugin) Register(r *trpcplugin.Registry) {
	r.BeforeTool(p.beforeTool)
}

func (p *PermissionGuardPlugin) beforeTool(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
	if args == nil {
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	}
	agentKey := agentKeyFromCtx(ctx, nil)
	if len(p.cfg.AgentAllowlist) > 0 && !toolInList(agentKey, p.cfg.AgentAllowlist) {
		p.logger.Info("plugin.permission_guard.before_tool", "status", "skip", "tool", args.ToolName, "reason", "agent_not_in_allowlist")
		p.record(ctx, "before_tool", "ok")
		return &trpctool.BeforeToolResult{Context: ctx}, nil
	}
	if toolInList(args.ToolName, p.cfg.DenyTools) || toolInList(args.ToolName, p.cfg.ConfirmTools) {
		p.logger.Info("plugin.permission_guard.before_tool", "status", "blocked", "tool", args.ToolName)
		p.record(ctx, "before_tool", "blocked")
		msg := fmt.Sprintf("permission_guard: tool %q is not permitted", args.ToolName)
		return &trpctool.BeforeToolResult{
			Context:      ctx,
			CustomResult: map[string]any{"error": msg, "blocked": true},
		}, nil
	}
	p.logger.Info("plugin.permission_guard.before_tool", "status", "ok", "tool", args.ToolName)
	p.record(ctx, "before_tool", "ok")
	return &trpctool.BeforeToolResult{Context: ctx}, nil
}

func agentKeyFromCtx(ctx context.Context, inv *trpcagent.Invocation) string {
	_, key := sessionAgentKey(ctx, inv)
	return key
}

func (p *PermissionGuardPlugin) record(ctx context.Context, point, status string) {
	if p.stats != nil {
		p.stats.Record(ctx, p.name, point, status)
	}
}
